import { useCallback, useEffect, useRef, useState } from "react";
import {
  CaretRight,
  CaretUp,
  CaretUpDown,
  CaretDown,
  WarningCircle,
} from "@phosphor-icons/react";
import { isBinaryPath, pushErrorMessage } from "../api";
import { useAppServices } from "../app/appServices";
import { copyText } from "../clipboard";
import { useStore } from "../store";
import { loadGitPrefs } from "../theme";
import type { DiffResp, DiffScope } from "../types";
import { parseFileDiff, defaultOpenByPath, splitDiff, splitPath, splitRows, highlightLine, hunkGaps, trailingGapKey, langFromPath, type ContextGap, type DiffRow, type FileDiffSummary, type FileStatus, type ParsedFileDiff } from "../diffSummary";
import { DiffStateView, DiffToolbar } from "./DiffParts";
import { useWorktreeActions } from "./worktreeActions";
import { useBreakpoint } from "../hooks/useBreakpoint";

export { DiffSkeleton } from "./DiffParts";

// renderCode turns one diff line into syntax-highlighted spans (INC-41 D3).
// Tokens are dependency-free and byte-exact, so `white-space: pre` alignment is
// preserved; the .hl-* colors go inert when the user turns syntax off
// (`:root:not([data-syntax])`), leaving plain, still-tinted diff text.
function renderCode(text: string, lang: string) {
  return highlightLine(text || " ", lang).map((tok, i) => (
    <span key={i} className={tok.c ? "hl-" + tok.c : undefined}>
      {tok.t}
    </span>
  ));
}

// Compact Codex-style status glyph shown before the path in a file header.
const STATUS_GLYPH: Record<FileStatus, string> = {
  modified: "M",
  added: "A",
  deleted: "D",
  renamed: "R",
  copied: "C",
};

// INC-41 RV-5 · badges the leading glyph already states. "new file" next to a
// green A (and "deleted" next to a red D) said the same thing twice while
// squeezing the filename into `package-lock.js…`; only the badges the glyph
// cannot carry ("binary", "mode changed") still earn their width.
const GLYPH_BADGES = new Set(["new file", "deleted", "renamed", "copied"]);

// INC-41 RD-12 · what the file list needs to know about a file, read straight off
// its metadata lines. `parseFileDiff` answers the same question — but it also
// materializes every row of every file, a cost the *list* has no use for and would
// pay again on every keystroke in its filter (a review holding one 40k-line
// lockfile would rebuild 40k rows per character). Git writes a file's metadata
// before its first `@@`, so the scan stops there; the badges/status vocabulary is
// parseFileDiff's, line for line.
function headMeta(lines: string[]): { status: FileStatus; binary: boolean } {
  let status: FileStatus = "modified";
  let binary = false;
  for (const line of lines) {
    if (line.startsWith("@@")) break;
    if (line.startsWith("new file")) status = "added";
    else if (line.startsWith("deleted file")) status = "deleted";
    else if (line.startsWith("rename ")) status = "renamed";
    else if (line.startsWith("copy ")) status = "copied";
    else if (line.startsWith("Binary files") || line.startsWith("GIT binary patch")) binary = true;
  }
  return { status, binary };
}

// INC-41 RVW-ORDER · one file of the review, tracked or not.
//
// The panel used to hold two lists and render them back to back: every untracked
// file first, then the tracked stream. Which meant a review whose untracked set
// happens to be `bin/ar`, `bin/arwebui` opened on two headers that say `[binary]`
// and, when clicked, "Content isn't shown" — the first thing the reader saw was
// the two files nobody can read, and the actual code changes started below the
// fold. The golden's review opens on its first *readable* file header.
//
// So: one list, one order. `file` is what separates the two kinds — the tracked
// diff's own hunk lines, or null for an untracked file the card has to fetch.
interface ReviewFile {
  path: string;
  status: FileStatus;
  add: number | null; // null = not knowable yet (an untracked blob not yet read)
  del: number;
  binary: boolean; // no lines to count, no body to show → sinks, states no counts
  conflict: boolean;
  file: FileDiffSummary | null;
}

// What an untracked card learned about its file by *asking* (RVW-BINCOUNT): the
// blob came back (`binary: false`, a real `add`) or the endpoint refused it
// (`binary: true`). Only the card knows this — DiffView can otherwise guess from
// the extension alone, and `bin/ar` has none.
interface UntrackedFact {
  binary: boolean;
  add: number | null;
  reason?: UntrackedReason;
}

type UntrackedReason = "binary" | "large" | "unavailable";

// Sort: readable files first, in path order; the non-inline ones (binary,
// oversized/unavailable) sink to the end, where a header that opens on
// "Content isn't shown"
// costs the reader nothing. `<`/`>` on the path, not localeCompare: git orders
// its own diff by byte, and the two lists have to agree on where `a/` goes.
const cmpReviewFile = (a: ReviewFile, b: ReviewFile) =>
  a.binary !== b.binary ? (a.binary ? 1 : -1) : a.path < b.path ? -1 : a.path > b.path ? 1 : 0;

const rowSign = (r?: DiffRow) => (!r ? "" : r.kind === "add" ? "+" : r.kind === "del" ? "-" : " ");
const halfKind = (r: DiffRow | undefined, side: "left" | "right") =>
  !r ? "empty" : side === "left" && r.kind === "del" ? "del" : side === "right" && r.kind === "add" ? "add" : "";

// INC-41 DF-4 · the diff's line-wrap preference, shared by every file and every
// session (the conversation's code blocks keep theirs per-block because a page
// holds dozens of unrelated blocks; a review is one surface, so one switch).
// Kept in localStorage rather than the store: a display preference the user sets
// once, not session state — and the store is off this change's touch list.
// DIFF-WRAP-DEFAULT-ON · default *on* when the user hasn't set a preference: a
// review is one surface whose whole job is to show the changed characters, so
// soft-wrap by default means a long line is never clipped by the panel edge
// (Codex-parity: the diff stays readable inside its column). Only an explicit
// "0" — the user reaching for the toolbar / `…` switch to turn wrap off — turns
// it off; that choice still persists exactly as before. Absent key → wrap on.
const WRAP_KEY = "ar.diff.wrap";
const loadWrap = (storage: Storage): boolean => {
  try {
    const v = storage.getItem(WRAP_KEY);
    return v === null ? true : v === "1"; // unset → default on; explicit "0" → off
  } catch {
    return true; // private mode / storage disabled: keep the default "nothing clipped" stance
  }
};
const saveWrap = (storage: Storage, on: boolean) => {
  try {
    storage.setItem(WRAP_KEY, on ? "1" : "0");
  } catch {
    /* ignore */
  }
};

// INC-41 RVW-4 · which changes the review opens on. Codex's review defaults to
// the last turn, and so does the thread: its change card counts the files *this
// turn* touched, and its `Review` button is a link into this panel — so a panel
// that opened on the working tree answered a question nobody asked (`Edited 5
// files +2 −179` in the card, a different diff in the rail it linked to). The
// default is the turn now; the working tree is one click away, and whichever the
// user picks sticks (same reasoning as WRAP_KEY: a preference, not session
// state). Unparsable/absent value → the default, never a crash.
const SCOPE_KEY = "ar.diff.scope";
const isScope = (v: unknown): v is DiffScope => v === "working-tree" || v === "last-turn";
const loadScope = (storage: Storage): DiffScope => {
  try {
    const v = storage.getItem(SCOPE_KEY);
    return isScope(v) ? v : "last-turn";
  } catch {
    return "last-turn"; // private mode / storage disabled / test stub
  }
};
const saveScope = (storage: Storage, s: DiffScope) => {
  try {
    storage.setItem(SCOPE_KEY, s);
  } catch {
    /* ignore */
  }
};

// INC-41 DIFF-CP / RD-8 · the bar is "tight" below this width, and its secondary
// residents stand down. Measured on the *bar*, not the window: this panel is the
// session's main column, and its width moves with the sidebar and the right rail
// as well as the viewport. (At a 1024px window it is 339px wide — a media query
// cannot see that, which is how the bugs below survived.)
//
//  1 · Commit-or-push drops its label for its glyph. It never leaves: a main
//      action you have to go looking for is the gap DIFF-CP closes.
//  2 · The split/inline toggle steps aside and the view falls back to inline —
//      which is D4's own rule ("split needs room"), finally applied to the box
//      that needs the room. It was keyed to a `max-width: 900px` *window*, so at
//      a 1100px window it still offered split view for a 415px panel: two ~190px
//      columns of code.
//  3 · RD-8 · Copy and Wrap follow them — into the `…` menu, where they are
//      still one click away rather than gone. DIFF-CP stopped one control short:
//      with the resident Commit pill back on the bar (150px that never shrinks),
//      a 339px panel still needed 367px, so the bar overflowed by 28px and the
//      thing hanging off the end was the ✕ — measured at x=1051.9 against a panel
//      whose right edge is 1024 (qa/runs/2026-07-12-r33/after-rd89/before.json).
//      The user could read the diff and not close it. Every control on this bar
//      is `flex: 0 0 auto` (tw.css), so flexbox does not solve this for us:
//      the row has to be *short enough*, which means the low-frequency controls
//      have to leave it. The ✕ is last and unshrinkable, always.
//
// 640, not 600: the panel at a 1152px window is 467px and at 1280 it is 568px —
// both already tight in every practical sense, and the extra 40px of margin is
// what keeps a wider `+1,234 −5,678` summary or a longer branch chip from
// walking the bar back over its own edge.
const BAR_TIGHT_PX = 640;

// A phone review has no width to spend framing code inside another card. Codex
// lets file sections run edge-to-edge in the review rail; dropping our 12px
// card margins gives a 390px phone 24px more path/code width while the top and
// bottom borders still separate adjacent files. Use the viewport signal, not
// barTight: an ordinary desktop split rail is also narrower than 640px and must
// keep the card shape.
const fileCardClass = (edgeToEdge: boolean, untracked = false) =>
  "filediff" +
  (untracked ? " filediff-untracked" : "") +
  (edgeToEdge ? " !m-0 !rounded-none !border-x-0" : "");

// FileHead is the one file header in the review — Codex has exactly one kind of
// changed-file card, and after DF-3 so do we: tracked edits and untracked new
// files render through this same summary (caret + M/A/D glyph + path + `+x −y`
// + badges) and the same expandable body underneath.
export function FileHead({
  path,
  status,
  add,
  del,
  badges,
}: {
  path: string;
  status: FileStatus;
  add: number | null;
  del: number;
  badges: string[];
}) {
  const { dir, base } = splitPath(path);
  // INC-41 DF-D3 · a binary file has no lines, so it has no line counts. `A
  // bin/ar +0 −0 [binary]` stated a measurement nobody took: the zeros are not
  // "nothing changed", they're "not applicable" — and the badge right next to
  // them already says exactly that. Same principle a070dea applied to the tool
  // cards (which stopped printing a fabricated `+0 −0` of their own); here the
  // badge speaks alone.
  const unavailable = badges.some((badge) => badge === "binary" || badge === "large" || badge === "unavailable");
  return (
    <summary className="fd-head mono">
      <span className="fd-caret" aria-hidden="true">
        <CaretRight size={12} weight="bold" />
      </span>
      <span className={"fd-glyph fd-glyph-" + status} title={status} aria-hidden="true">
        {STATUS_GLYPH[status]}
      </span>
      <span className="fd-path" title={path}>
        {dir && <span className="fd-dir">{dir}</span>}
        <b style={{ fontWeight: 600, color: "var(--ink)" }}>{base}</b>
      </span>
      {/* RD-4: counts sit right after the filename (Codex: `docs/DESIGN.md +8 -4`),
          both numbers always rendered — a pure deletion reads "+0 −176", not a
          lone "−176". `add === null` is the one honest gap: an untracked file's
          line count is only known once its blob is in hand. */}
      {!unavailable && (
        <span className="fd-counts">
          <span className="add">+{add === null ? "…" : add}</span>
          <span className="del">-{del}</span>
        </span>
      )}
      {/* INC-41 DF-D6 · badges are a property of *this file*, so they travel with
          its name — right after the counts, the way "binary" reads as part of the
          line `A bin/ar [binary]`. Behind the elastic gap they ended up ~475px
          from the filename, hard against the panel's right edge, where they read
          as a column of their own belonging to nothing in particular. The gap
          (.fd-spacer, not .fd-path — tw.css keeps the path from stretching
          `.fd-path{flex:1}`) now sits last and simply absorbs the leftover. */}
      {badges
        .filter((b) => !GLYPH_BADGES.has(b))
        .map((b) => (
          <span className={"fd-badge" + (b === "conflict" ? " conflict" : "")} key={b}>{b}</span>
        ))}
      <span className="fd-spacer" aria-hidden="true" />
    </summary>
  );
}

// `initialScope` is the entry point's claim: the changes card names a scope in
// its title ("Edited N files" = last turn, "Changes in workspace" = working
// tree), and the panel it links to must open on that same scope — RVW-4's own
// rationale, which the card's workspace fallback otherwise inverts (card says
// "+1", panel says "No changes this turn"). It is a hint, not a preference:
// it never persists, and entries that make no claim pass nothing.
export function DiffView({ sid, onClose, initialScope }: { sid: string; onClose?: () => void; initialScope?: DiffScope | null }) {
  const { api, storage } = useAppServices();
  const { toast, openPrompt } = useStore();
  const bumpWorkspaceEpoch = useStore((s) => s.bumpWorkspaceEpoch);
  // INC-41 TH-5 · a file the thread's change card asked us to open. It is a
  // one-shot request: we take it into local state (so the file stays open once
  // the user reads it), clear it from the store, and let the file's own card key
  // off `focusEpoch` so a *second* click on the same row re-opens and re-scrolls
  // it even if it was manually collapsed in between.
  const pendingFocus = useStore((s) => s.diffFocusPath);
  const clearDiffFocus = useStore((s) => s.clearDiffFocus);
  const [focusPath, setFocusPath] = useState<string | null>(null);
  const [focusEpoch, setFocusEpoch] = useState(0);
  const [data, setData] = useState<DiffResp | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [scope, setScope] = useState<DiffScope>(() => initialScope ?? loadScope(storage.local));
  const scopeTriggerRef = useRef<HTMLButtonElement>(null);
  const scopeFocusPending = useRef(false);
  // The panel stays mounted while the timeline's changes card is still
  // clickable beside it — a later card click must still land on its scope.
  useEffect(() => {
    if (initialScope) setScope(initialScope);
  }, [initialScope]);
  // Did the *user* choose this scope, here, in this panel? Only then is "Last
  // turn unavailable" an answer worth showing: a scope we picked for them (the
  // RVW-4 default) failing on a session with no durable baseline is our problem,
  // not theirs, so it falls back to the working tree without saying a word — and
  // without persisting, because they never expressed a preference.
  const picked = useRef(false);
  const pickScope = (s: DiffScope) => {
    picked.current = true;
    scopeFocusPending.current = true;
    saveScope(storage.local, s);
    setScope(s);
  };
  const requestID = useRef(0);
  // Fold-all override: null = every file follows its own default (INC-41 RD-1 —
  // per-file disclosure, so one huge file no longer folds its small neighbours);
  // true/false = the user pressed Expand-all / Collapse-all. Bumping the epoch
  // remounts the <details> so a global toggle wins over any manual per-file
  // toggling since the last one.
  const [override, setOverride] = useState<boolean | null>(null);
  const [foldEpoch, setFoldEpoch] = useState(0);
  const setAll = (open: boolean) => {
    // A file-list jump pins its target open so the landing can never be a
    // folded header. A later fold-all click is the newer, explicit intent:
    // release that one-shot pin before applying the global state.
    setFocusPath(null);
    setOverride(open);
    setFoldEpoch((e) => e + 1);
  };
  // D2 file filter + D4 inline/split view. Split needs room; below ~900px it
  // falls back to inline so two columns never crush the diff column.
  const [fileQuery, setFileQuery] = useState("");
  const [view, setView] = useState<"inline" | "split">("inline");
  // DF-4 · soft-wrap long diff lines (see WRAP_KEY above). Off = Codex's default
  // (one horizontal scroll for the whole rail); on = nothing is clipped.
  const [wrap, setWrap] = useState(() => loadWrap(storage.local));
  const toggleWrap = () =>
    setWrap((w) => {
      saveWrap(storage.local, !w);
      return !w;
    });
  // INC-41 RVW-BINCOUNT · what the untracked cards found out, kept where the file
  // list can read it. `isBinaryPath` is an *extension* guess, and the two files
  // this review shows most (`bin/ar`, `bin/arwebui`) have no extension: the guess
  // said "text", so the list printed a `+…` that would never resolve and the card
  // spent a doomed request proving otherwise — while the header two pixels away
  // rendered `[binary]` and no counts. One screen, two answers. The card is the
  // one that *knows* (it either holds the blob's lines or watched the endpoint
  // refuse it), so it reports, and its truth overrides the guess everywhere: the
  // counts, the ordering, and whether the next mount asks again at all.
  const [facts, setFacts] = useState<Record<string, UntrackedFact>>({});
  useEffect(() => setFacts({}), [sid]); // a different workspace, different files
  const reportFact = useCallback((path: string, fact: UntrackedFact) => {
    setFacts((prev) => {
      const cur = prev[path];
      // Monotone: a card remounts on every fold-all/filter/focus, and a fresh
      // mount starts with `lines: null` again — that must not walk a known `+42`
      // back to `+…`, nor a known binary back to a guess.
      const next: UntrackedFact = {
        binary: fact.binary || !!cur?.binary,
        add: fact.add ?? cur?.add ?? null,
        reason: fact.reason ?? cur?.reason,
      };
      if (cur && cur.binary === next.binary && cur.add === next.add && cur.reason === next.reason) return prev;
      return { ...prev, [path]: next };
    });
  }, []);
  const bp = useBreakpoint();
  const narrow = bp.compact || bp.tablet;
  // DIFF-CP · what the bar actually measures (see BAR_TIGHT_PX). A stable
  // callback ref, because the bar only exists once the diff has landed — a `[]`
  // effect would run against the skeleton and find nothing to observe. The
  // ResizeObserver is guarded for jsdom, which has none: no observer → not tight
  // → the label stays, the honest default for a bar we cannot measure.
  const [barTight, setBarTight] = useState(false);
  const barObs = useRef<ResizeObserver | null>(null);
  const barRef = useCallback((el: HTMLDivElement | null) => {
    barObs.current?.disconnect();
    barObs.current = null;
    if (!el || typeof ResizeObserver === "undefined") return;
    const measure = () => setBarTight(el.clientWidth > 0 && el.clientWidth < BAR_TIGHT_PX);
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    barObs.current = ro;
    measure();
  }, []);
  useEffect(() => () => barObs.current?.disconnect(), []);
  // `narrow` is the window; `barTight` is the panel. Split view needs room in the
  // box that renders it, and only `narrow` (window ≤900px) may *refuse* it —
  // two columns would crush the diff column there. DIFF-SPLIT-TOGGLE-GONE ·
  // `barTight` (a mid-width panel <640px, e.g. 605px at a 1440 window) must not:
  // its toggle demotes into `…` (see the tight menu below) instead of vanishing,
  // so the panel honours the user's explicit `view`. `view` defaults to "inline",
  // so this only changes rendering once the user actively chooses split — it
  // never regresses on its own.
  const effView = narrow ? "inline" : view;
  // DF-1 · the review rail is ~56% of the window, so below ~1400px the worktree
  // chip's text is the first thing with nowhere to go. It shrinks (never its
  // neighbours — see .diffwrap .diffbar in tw.css), and here it stops being
  // a half-word clipped mid-glyph and becomes an honest icon-only chip; the full
  // "worktree of <repo> · <branch>" stays one hover away in its title.
  const chipCompact = bp.compact || bp.tablet || bp.desktop;

  const load = () => {
    const currentRequest = ++requestID.current;
    setData(null);
    setErr("");
    api.diff(sid, scope)
      .then((d) => {
        if (currentRequest !== requestID.current) return;
        // RVW-4 · the silent fallback. `data` stays null, so the skeleton simply
        // keeps running while the working-tree request the scope change fires
        // lands — the user sees one load, not a flash of an error card.
        if (scope === "last-turn" && d.available === false && !picked.current) {
          setScope("working-tree");
          return;
        }
        // Drop any Expand/Collapse-all override from the previous payload, so each
        // file of the new one opens on its own merits (defaultOpenByPath) — decided
        // during render, before the first paint, never by a post-paint effect.
        setOverride(null);
        setFoldEpoch((epoch) => epoch + 1);
        setData(d);
        setErr("");
      })
      .catch((e) => {
        if (currentRequest === requestID.current) setErr(e.message);
      });
  };
  useEffect(() => {
    setFileQuery("");
    load();
    return () => {
      requestID.current += 1;
    };
  }, [sid, scope]);
  // Scope changes intentionally replace the current diff with a loading
  // skeleton. That briefly unmounts both the selected menuitem and its trigger,
  // so Popover's ordinary next-frame focus return has no live target. Finish
  // the handoff once the new payload restores the toolbar.
  useEffect(() => {
    if (!data || !scopeFocusPending.current) return;
    scopeFocusPending.current = false;
    requestAnimationFrame(() => scopeTriggerRef.current?.focus());
  }, [data]);
  // Take the pending focus (TH-5). Runs on mount too — the panel is usually
  // mounted BY the click, so the request is waiting for us before the diff has
  // even loaded; the file list picks it up when the payload lands. Any active
  // file filter is dropped: the user just asked for a specific file, and a
  // stale query that excludes it would silently answer "no matching files".
  useEffect(() => {
    if (!pendingFocus) return;
    setFocusPath(pendingFocus);
    setFocusEpoch((e) => e + 1);
    setFileQuery("");
    clearDiffFocus();
  }, [pendingFocus, clearDiffFocus]);
  // Callback ref on the focused file's card: stable, so it fires exactly when
  // that card mounts (its key carries focusEpoch), i.e. once per focus request.
  const focusRef = useCallback((el: HTMLDetailsElement | null) => {
    el?.scrollIntoView?.({ block: "start", behavior: "smooth" });
  }, []);
  // INC-41 RD-12 · the file list's click target: the same focus request the thread's
  // change card makes (TH-5), raised from inside the panel. It goes straight into
  // the local focus state rather than through the store's `focusDiffFile`, because
  // the store round-trip exists to hand a path *across* components and its consumer
  // (the effect above) clears the file filter on arrival — which would wipe the very
  // query the user is filtering this list with. The file the request names is opened
  // (isOpen), remounted (fileKey carries focusEpoch) and scrolled to (focusRef), so
  // clicking the same row twice re-scrolls to it.
  const focusFile = (path: string) => {
    setFocusPath(path);
    setFocusEpoch((e) => e + 1);
  };
  // Codex review→commit(→push): stage & commit the workspace changes, optionally
  // pushing to the upstream branch. `thenPush` chains a push only when the commit
  // succeeded, so a failed commit never pushes a half-finished state.
  const commit = () => openCommitPrompt(false);
  const commitAndPush = () => openCommitPrompt(true);
  const openCommitPrompt = (thenPush: boolean) => {
    openPrompt({
      title: thenPush ? "Commit & push" : "Commit changes",
      label: "commit message",
      // Seed from the Settings › Git commit-message template (INC-41 H4).
      initial: loadGitPrefs(storage.local).commitTemplate,
      submitLabel: thenPush ? "Commit & push" : "Commit",
      onSubmit: (message) => void doCommit(message, thenPush),
    });
  };
  const doCommit = async (message: string, thenPush = false) => {
    setBusy(true);
    try {
      await api.commit(sid, message);
      if (thenPush) {
        const r = await api.push(sid);
        toast(r.branch ? `committed & pushed ${r.branch}` : "committed & pushed", "info");
      } else {
        toast("committed", "info");
      }
      load();
      bumpWorkspaceEpoch();
    } catch (e: any) {
      toast(pushErrorMessage(e), "error", e.details);
    } finally {
      setBusy(false);
    }
  };
  // Push already-made commits to the upstream/origin branch (no new commit). The
  // backend returns structured failures (no remote / no upstream / rejected /
  // detached / auth) which the ApiError message already spells out.
  const doPush = async () => {
    setBusy(true);
    try {
      const r = await api.push(sid);
      toast(r.branch ? `pushed ${r.branch}` : "pushed", "info");
      load();
      bumpWorkspaceEpoch();
    } catch (e: any) {
      toast(pushErrorMessage(e), "error", e.details);
    } finally {
      setBusy(false);
    }
  };

  // INC-41 RD-C · Apply-to-project / Remove-worktree (INC-49) used to be written
  // out here, which is why they existed only here: the Environment rail's
  // `Worktree` row could copy a path and nothing else. Same endpoints, same
  // confirmations, same toasts — now in `worktreeActions`, which that rail calls
  // too. This panel's behaviour is unchanged, down to the busy flag and the
  // reload on success.
  const { applyBack, removeWorktree } = useWorktreeActions({
    sid,
    onDone: () => {
      load();
      bumpWorkspaceEpoch();
    },
    setBusy,
  });

  // Turn the workspace into its own repo, then re-load — offered from the
  // non-repo / nested empty states so "no diff" is always actionable.
  const gitInit = async () => {
    setBusy(true);
    try {
      await api.gitInit(sid);
      toast("workspace is now a git repository — future changes will show here", "info");
      load();
      bumpWorkspaceEpoch();
    } catch (e: any) {
      toast(e.message, "error", e.details);
    } finally {
      setBusy(false);
    }
  };

  // RVW-3 · the unified diff, verbatim, on the clipboard — the exact text `git
  // diff` produced, so it pastes into an issue or a message as a diff (and back
  // into `git apply`). Feedback is the app's existing toast; a failure to write
  // the clipboard says so rather than passing silently for a copy that never
  // happened.
  const copyDiff = async () => {
    const text = data?.diff || "";
    if (!text) return;
    try {
      await copyText(text);
      toast("diff copied", "info");
    } catch {
      toast("couldn’t copy the diff");
    }
  };

  const stateBar = (
    <DiffToolbar
      variant="state"
      scope={scope}
      scopeTriggerRef={scopeTriggerRef}
      onScopeChange={pickScope}
      onRefresh={load}
      onClose={onClose}
    />
  );

  if (err)
    return (
      <div className="diffwrap">
        {stateBar}
        <DiffStateView state={{ kind: "error", message: err, onRetry: load }} />
      </div>
    );
  // INC-41 RVW-6 · the review loads the way the rest of the app does. This was a
  // single grey sentence ("Loading changes…") in a 658px panel — while the 40px
  // change card in the thread that *links here*, the sidebar, and the timeline
  // all draw skeleton bars. The summary card was loading more gracefully than the
  // panel it opens.
  if (!data)
    return (
      <div className="diffwrap">
        {stateBar}
        <DiffStateView state={{ kind: "loading" }} />
      </div>
    );

  if (scope === "last-turn" && data.available === false)
    return (
      <div className="diffwrap">
        {stateBar}
        <DiffStateView
          state={{ kind: "last-turn-unavailable", reason: data.reason }}
        />
      </div>
    );

  if (!data.known)
    return (
      <div className="diffwrap">
        {stateBar}
        <DiffStateView
          state={{ kind: "workspace-unavailable", onRetry: load }}
        />
      </div>
    );
  if (scope === "working-tree" && data.nested)
    return (
      <div className="diffwrap">
        {stateBar}
        <DiffStateView
          state={{ kind: "nested", busy, onTrack: gitInit }}
        />
      </div>
    );
  if (scope === "working-tree" && !data.isRepo)
    return (
      <div className="diffwrap">
        {stateBar}
        <DiffStateView
          state={{ kind: "non-repo", busy, onTrack: gitInit }}
        />
      </div>
    );

  const files = splitDiff(data.diff || "");
  const untracked = data.untracked || [];
  const hiddenUntracked = data.hiddenUntracked || 0;
  const conflicts = data.conflicts || [];
  const conflictSet = new Set(conflicts);
  const empty = files.length === 0 && untracked.length === 0 && hiddenUntracked === 0;

  // Per-file +/- counts (from the diff itself, so untracked-content blocks count
  // too) — Codex shows these next to each file and a total at the top.
  const stats = files.map((f) => ({ f, add: f.add, del: f.del }));
  const totalAdd = stats.reduce((s, x) => s + x.add, 0);
  const totalDel = stats.reduce((s, x) => s + x.del, 0);
  const q = fileQuery.trim().toLowerCase();
  // INC-41 RVW-ORDER · the review, as one ordered list of files — the *only* list.
  // Tracked and untracked go in together (DF-3 already made them the same card),
  // readable files first in path order, unreadable ones last (cmpReviewFile). The
  // stream below and the file-list popover both render *this*, so a click in the
  // list can no longer land on a different file than the one it named.
  //
  // RVW-BINCOUNT · `binary` is the card's reported truth where there is one, and
  // the extension guess only until then.
  const entries: ReviewFile[] = [
    ...untracked.map((path): ReviewFile => {
      const fact = facts[path];
      const knownReason = fact?.reason ?? data.untrackedReasons?.[path];
      return {
        path,
        status: "added",
        add: fact ? fact.add : null,
        del: 0,
        binary: fact ? fact.binary : !!knownReason || isBinaryPath(path),
        conflict: false,
        file: null,
      };
    }),
    ...stats.map(({ f, add, del }): ReviewFile => {
      const meta = headMeta(f.lines);
      return {
        path: f.path, status: meta.status, add, del, binary: meta.binary,
        conflict: conflictSet.has(f.path), file: f,
      };
    }),
  ].sort(cmpReviewFile);
  // DF-3 · untracked files are files: they go through the same filter, the same
  // count, the same Expand/Collapse-all as everything else in the review.
  const shown = q ? entries.filter((e) => e.path.toLowerCase().includes(q)) : entries;
  const fileCount = entries.length;
  const shownCount = shown.length;
  const shownTracked = shown.filter((e) => e.file);
  const shownUntrackedCount = shown.length - shownTracked.length;
  // Per-file default disclosure (RD-1) — computed over the whole review, not just
  // the filtered subset, so filtering never changes a file's default state.
  const defaults = defaultOpenByPath(files);
  // A file the change card sent us to is open, whatever its default or the
  // current fold-all state — you cannot "go to a file's diff" and land on a
  // folded header (TH-5).
  const isOpen = (path: string) => (path === focusPath ? true : override ?? defaults.get(path) ?? true);
  // Only the focused card remounts on a focus request (its neighbours keep any
  // manual fold state) — hence focusEpoch in the key of that one card.
  const fileKey = (path: string) => path + ":" + foldEpoch + (path === focusPath ? ":f" + focusEpoch : "");
  // An untracked entry that survives to this list is one git refused to inline
  // (binary, >256KB, or past the inline budget — webui/meta.go), so it folds by
  // default for the same reason DF-2 folds generated files: a review shouldn't
  // open on a wall of content nobody reads. Expand-all still opens it.
  const untrackedOpen = override ?? false;
  const allShownOpen =
    shownCount > 0 && shownTracked.every((e) => isOpen(e.path)) && (shownUntrackedCount === 0 || untrackedOpen);

  return (
    <div className={"diffwrap" + (wrap ? " diff-wrap" : "")}>
      <DiffToolbar
        variant="ready"
        barRef={barRef}
        scope={scope}
        scopeTriggerRef={scopeTriggerRef}
        onScopeChange={pickScope}
        onRefresh={load}
        onClose={onClose}
        barTight={barTight}
        empty={empty}
        totalAdd={totalAdd}
        totalDel={totalDel}
        worktree={!!data.worktree}
        mainRepo={data.mainRepo}
        branch={data.branch}
        chipCompact={chipCompact}
        files={shown}
        fileCount={fileCount}
        query={fileQuery}
        hiddenUntracked={hiddenUntracked}
        allShownOpen={allShownOpen}
        narrow={narrow}
        view={effView}
        wrap={wrap}
        busy={busy}
        isRepo={data.isRepo}
        conflictCount={conflicts.length}
        onQueryChange={setFileQuery}
        onFocusFile={focusFile}
        onToggleAll={() => setAll(!allShownOpen)}
        onToggleWrap={toggleWrap}
        onCopy={() => void copyDiff()}
        onToggleView={() =>
          setView(effView === "split" ? "inline" : "split")
        }
        onApplyProject={() =>
          data.mainRepo && applyBack(data.mainRepo)
        }
        onRemoveWorktree={removeWorktree}
        onCommit={commit}
        onCommitAndPush={commitAndPush}
        onPush={() => void doPush()}
      />
      {conflicts.length > 0 && (
        <div className="diff-conflict-note" role="status">
          <WarningCircle size={16} weight="fill" />
          <span>
            Resolve {conflicts.length} merge {conflicts.length === 1 ? "conflict" : "conflicts"} before committing.
            Push existing commits remains available.
          </span>
        </div>
      )}
      {/* INC-41 L4 · every "nothing to show" in this panel speaks the timeline's
          empty-state language (icon + title + one line of guidance) via the
          shared `.diff-empty` shape — a bare grey sentence was the odd one out. */}
      {empty && (
        <DiffStateView state={{ kind: "empty", scope }} />
      )}
      {!empty && q && shownCount === 0 && (
        <DiffStateView
          state={{
            kind: "no-matches",
            query: fileQuery,
            fileCount,
          }}
        />
      )}
      {/* INC-41 RD-12 · the review opens on the *first file*. What used to be here
          — `N generated files hidden`, full-bleed, above everything — meant the
          first sentence of every review was about the files it does not contain;
          the golden's first line is its first file header, and its "hidden" count
          is a footnote inside the file list (see the toolbar's file-tree popover,
          where the note now lives, tooltip and all). */}
      {/* INC-41 DF-3 · untracked files used to be a grey `new files (untracked) ·
          N` strip of bare paths: no glyph, no `+N −0`, no line numbers, nothing to
          open — a second visual language for the files a review most wants to
          read. They are ordinary file cards now…
          INC-41 RVW-ORDER · …and they are in the *stream* now, not in a block of
          their own bolted on top of it. Two untracked binaries (`bin/ar`,
          `bin/arwebui`) were enough to make every review of this repo open on two
          headers whose bodies read "Content isn't shown", with the code the reader
          came for pushed below the fold. One list, sorted, unreadable files last —
          and the file-list popover above renders the same array, so its rows and
          these cards can never disagree about what comes where. */}
      {shown.map((e) => {
        if (!e.file)
          return (
            <UntrackedFile
              key={fileKey(e.path)}
              sid={sid}
              path={e.path}
              effView={effView}
              detailsRef={e.path === focusPath ? focusRef : undefined}
              defaultOpen={e.path === focusPath ? true : untrackedOpen}
              // Same budget as FileBody's prefetch: on a small review, read the file
              // up front so its header can state a real `+N −0` instead of `+…`.
              prefetch={shownUntrackedCount <= 25}
              // RVW-BINCOUNT · what we already learned about this file, so a remount
              // (fold-all, filter, focus) never re-asks a question the endpoint has
              // already refused — and reports back what it learns itself.
              knownReason={facts[e.path]?.reason ?? data.untrackedReasons?.[e.path] ?? (isBinaryPath(e.path) ? "binary" : undefined)}
              onFact={reportFact}
              edgeToEdge={narrow}
            />
          );
        const f = e.file;
        const parsed = parseFileDiff(f.lines);
        const lang = langFromPath(f.path);
        // A hunk header with no @@ context text is pure noise: a lone "⋯" band.
        // Drop it entirely when the file has a single hunk (nothing to separate);
        // with several hunks it becomes a compact hairline separator instead.
        const hunkCount = parsed.rows.reduce((n, r) => n + (r.kind === "hunk" ? 1 : 0), 0);
        const open = isOpen(f.path);
        return (
          <details
            className={fileCardClass(narrow)}
            key={fileKey(f.path)}
            open={open}
            ref={f.path === focusPath ? focusRef : undefined}
          >
            {/* RV-3 · the disclosure caret: `list-style: none` killed the platform
                triangle, so a collapsed file was a lone header with no hint that a
                body was hiding under it. Header shape now lives in FileHead, which
                untracked files share (DF-3). */}
            <FileHead
              path={f.path}
              status={parsed.status}
              add={e.add}
              del={e.del}
              badges={e.conflict ? [...parsed.badges, "conflict"] : parsed.badges}
            />
            <FileBody
              sid={sid}
              path={f.path}
              parsed={parsed}
              lang={lang}
              effView={effView}
              hunkCount={hunkCount}
              // Only pay for a blob prefetch on bodies the user is actually looking
              // at, and only while the review is small enough that one request per
              // file is cheap (see FileBody's trailing-band note).
              prefetch={open && effView === "inline" && shownTracked.length <= 25}
            />
          </details>
        );
      })}
    </div>
  );
}

// INC-41 DF-3 · UntrackedFile — a new file that never reached `git diff`.
//
// The backend already inlines every small text file it finds as a synthetic
// new-file diff (webui/meta.go), so what lands in `untracked` is precisely the
// remainder: binary blobs, files over 256KB, and anything past the inline
// budget. Those used to render as bare paths in a text strip. Here they are the
// same `details.filediff` card as any other file — A glyph, path, `+N −0`, a
// disclosure — with their body read from the workspace on demand (api.blob, the
// endpoint the "N unmodified lines" bands already use) and rendered as what it
// is: a file made entirely of added lines.
export function UntrackedFile({
  sid,
  path,
  effView,
  defaultOpen,
  prefetch,
  detailsRef,
  knownReason,
  onFact,
  edgeToEdge,
}: {
  sid: string;
  path: string;
  effView: "inline" | "split";
  defaultOpen: boolean;
  prefetch: boolean;
  detailsRef?: (el: HTMLDetailsElement | null) => void;
  knownReason?: UntrackedReason;
  onFact?: (path: string, fact: UntrackedFact) => void;
  edgeToEdge: boolean;
}) {
  const { api } = useAppServices();
  const [open, setOpen] = useState(defaultOpen);
  const [lines, setLines] = useState<string[] | null>(null);
  // INC-41 DF-D7 · `untracked` is, by construction, the files git would not
  // inline: binaries, blobs over 256KB, and the tail past the inline budget
  // (webui/meta.go). So this card's blob fetch is the one most likely to hit the
  // endpoint's "file is not text" 400 — and for a `.bin`/`.png`/`.zip` it hits it
  // *every* time, on every mount, for a card whose body we already know reads
  // "Content isn't shown". The extension answers that question for free: the
  // failed state is entered without asking, so a binary file now costs zero
  // requests and leaves zero red lines in the console.
  //
  // INC-41 RVW-BINCOUNT · …for a file that *has* an extension. `bin/ar` has none,
  // so the guess said "text" and this card paid the doomed request anyway — once
  // per mount, i.e. once per fold-all, filter and focus. `knownBinary` is the
  // panel's memory of what a previous mount found out (it is seeded with the same
  // extension guess when there is nothing to remember yet), so the endpoint is
  // asked at most once about any file, ever.
  const [failedReason, setFailedReason] = useState<UntrackedReason | null>(
    () => knownReason ?? (isBinaryPath(path) ? "binary" : null),
  );
  const failed = failedReason !== null;

  useEffect(() => {
    if (failed || (!open && !prefetch) || lines) return;
    let alive = true;
    api.blob(sid, path)
      .then((r) => alive && setLines(r.lines))
      // Silent: an oversized file is an expected failure here, not an error the
      // user has to act on. The card says so in place of its rows.
      .catch((e) => {
        if (!alive) return;
        setFailedReason(e?.status === 413 ? "large" : "binary");
      });
    return () => {
      alive = false;
    };
  }, [sid, path, open, prefetch, lines, failed]);

  const rows: DiffRow[] = (lines || []).map((text, i) => ({ kind: "add", newNo: i + 1, text }));
  const parsed: ParsedFileDiff = { badges: failedReason ? [failedReason] : [], status: "added", rows };
  // A file the review intentionally cannot inline has no countable lines, so it
  // carries the backend's reason badge and FileHead prints no counts (DF-D3).
  const add = lines ? lines.length : failed ? 0 : null;
  // RVW-BINCOUNT · and now the file list agrees with them too. This card is the
  // only place in the panel that knows whether the blob came back or was refused;
  // until it said so, the list printed `+…` for a file whose own header three
  // pixels away said `[binary]`. It reports both facts — "is it readable" and, for
  // the ones that are, "how many lines" — and DiffView folds them into the list,
  // the sort order, and the next mount's decision not to ask again.
  useEffect(() => {
    onFact?.(path, { binary: failed, add, reason: failedReason ?? undefined });
  }, [onFact, path, failed, failedReason, add]);

  return (
    <details
      className={fileCardClass(edgeToEdge, true)}
      open={open}
      ref={detailsRef}
      onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}
    >
      <FileHead path={path} status="added" add={add} del={0} badges={parsed.badges} />
      {failed ? (
        <div className="fd-nobody">
          {failedReason === "large"
            ? "Content isn’t shown — this file is too large to display."
            : failedReason === "binary"
              ? "Content isn’t shown — this file is binary."
              : "Content isn’t shown — this file is unavailable."}
        </div>
      ) : lines ? (
        <FileBody
          sid={sid}
          path={path}
          parsed={parsed}
          lang={langFromPath(path)}
          effView={effView}
          hunkCount={0}
          // An added file's diff *is* the whole file: no gaps, nothing to fetch.
          prefetch={false}
        />
      ) : (
        <div className="fd-nobody">Loading…</div>
      )}
    </details>
  );
}

// FileBody renders one file's diff rows, and — in the inline view — the
// clickable "N unmodified lines" collapser bands Codex shows before the first
// hunk, between hunks, and (INC-41 RD-2) after the last hunk, so every file can
// be walked all the way to EOF. Clicking a band fetches the file's current text
// once (api.blob) and reveals the hidden unmodified region in place; clicking the
// revealed region's header collapses it again. The split view keeps the plain
// hunk-separator rendering (its paired-column model has no per-row anchor to
// hang a band on), so context expansion lives in the default inline layout.
export function FileBody({
  sid,
  path,
  parsed,
  lang,
  effView,
  hunkCount,
  prefetch,
}: {
  sid: string;
  path: string;
  parsed: ParsedFileDiff;
  lang: string;
  effView: "inline" | "split";
  hunkCount: number;
  prefetch: boolean;
}) {
  const { api } = useAppServices();
  const toast = useStore((s) => s.toast);
  // A trailing gap only exists where there's a new side that the diff might stop
  // short of: an added file's diff *is* the whole file, and a deleted one has no
  // new side at all.
  const gaps = hunkGaps(parsed.rows, { trailing: parsed.status !== "added" && parsed.status !== "deleted" });
  const trailKey = trailingGapKey(parsed.rows);
  const trailGap = gaps.get(trailKey);
  // Fetched file text (null until first reveal) and the set of gap keys whose
  // region is currently expanded.
  const [blob, setBlob] = useState<string[] | null>(null);
  // DF-D7 · a binary file has no lines to reveal — `git diff` already said so
  // with its "binary" badge, and the blob endpoint would answer 400 "file is not
  // text". Start from "the blob is unavailable" instead of proving it with a
  // request: the bands that would offer to reveal context stand down (band()
  // returns null for an unknowable length once blobFailed), and nothing is sent.
  const unreadable = parsed.badges.includes("binary") || isBinaryPath(path);
  const [blobFailed, setBlobFailed] = useState(unreadable);
  const [open, setOpen] = useState<Set<number>>(new Set());
  const [loadingIdx, setLoadingIdx] = useState<number | null>(null);

  // A unified diff never states a file's total line count, so the trailing gap's
  // length is unknowable from the payload alone (ContextGap.end === null). Rather
  // than invent a number, fetch the file once up front for the bodies on screen:
  // that turns the tail band into Codex's exact "N unmodified lines", and — just
  // as important — makes the band disappear when the last hunk already ran to EOF
  // (n <= 0), instead of offering an expander that reveals nothing. Bodies the
  // user isn't looking at, and reviews too large to spend a request per file on,
  // skip the prefetch and fall back to a count-less "to end of file" band that
  // resolves itself on the first click.
  const needsBlob = !!trailGap && trailGap.end === null;
  useEffect(() => {
    if (unreadable || !prefetch || !needsBlob || blob || blobFailed) return;
    let alive = true;
    api.blob(sid, path)
      .then((r) => alive && setBlob(r.lines))
      .catch(() => alive && setBlobFailed(true)); // silent: the diff itself still renders
    return () => {
      alive = false;
    };
  }, [sid, path, prefetch, needsBlob, blob, blobFailed, unreadable]);

  const toggleGap = async (idx: number) => {
    if (unreadable) return; // nothing to fetch, and nothing a fetch could add
    if (open.has(idx)) {
      setOpen((prev) => {
        const next = new Set(prev);
        next.delete(idx);
        return next;
      });
      return;
    }
    if (!blob) {
      setLoadingIdx(idx);
      try {
        const r = await api.blob(sid, path);
        setBlob(r.lines);
      } catch (e: any) {
        toast(e.message, "error", e.details);
        setBlobFailed(true);
        setLoadingIdx(null);
        return;
      }
      setLoadingIdx(null);
    }
    setOpen((prev) => new Set(prev).add(idx));
  };

  // How many lines a gap hides. null = not knowable yet (a to-EOF gap whose blob
  // hasn't been fetched); every other case is exact.
  const gapLen = (gap: ContextGap): number | null =>
    gap.end !== null ? gap.end - gap.start + 1 : blob ? blob.length - gap.start + 1 : null;

  // The revealed unmodified lines for a gap, sliced by 1-based new-file numbers.
  // A to-EOF gap runs to the end of the fetched blob.
  const revealedRows = (gap: ContextGap): DiffRow[] => {
    if (!blob) return [];
    const out: DiffRow[] = [];
    const end = gap.end ?? blob.length;
    for (let ln = gap.start; ln <= end && ln - 1 < blob.length; ln++) {
      out.push({ kind: "ctx", newNo: ln, oldNo: ln, text: blob[ln - 1] });
    }
    return out;
  };

  // Collapser band for a hidden run of unmodified lines. Expanded, it becomes a
  // thin "collapse" header above the revealed lines. The caret points at the
  // hidden content (RD-4a): up for the leading gap (file start → first hunk),
  // down for the trailing gap (last hunk → EOF), both ways for interior gaps.
  const band = (idx: number, gap: ContextGap, kind: "leading" | "interior" | "trailing", context?: string) => {
    const n = gapLen(gap);
    if (n !== null && n <= 0) return null; // nothing is actually hidden there
    if (n === null && blobFailed) return null; // can't read the file → can't reveal it
    const expanded = open.has(idx);
    const caret = expanded ? (
      <CaretUp size={12} />
    ) : kind === "leading" ? (
      <CaretUp size={12} />
    ) : kind === "trailing" ? (
      <CaretDown size={12} />
    ) : (
      <CaretUpDown size={12} />
    );
    const label =
      loadingIdx === idx
        ? "Loading…"
        : n === null
          ? "unmodified lines to end of file"
          : `${n.toLocaleString()} unmodified line${n === 1 ? "" : "s"}`;
    // DF-5 · the band sits *on* the code grid, not beside it: its first cell is
    // the line-number gutter (same `calc(5ch + 18px)` as `.dl`, holding the
    // caret the way a row holds its number), and its label starts exactly where
    // the code column does. It used to be a `px-[10px]` flex row aligned with
    // neither, which read as a button bolted onto the diff rather than a fold in
    // it — Codex's band is a line of the file. Geometry lives in tw.css
    // (`.fd-gap`), because `5ch` only agrees with `.dl`'s gutter if the two
    // inherit the same mono font and size from `.fd-body`.
    return (
      <button
        type="button"
        className="fd-gap"
        onClick={() => void toggleGap(idx)}
        disabled={loadingIdx === idx}
        title={expanded ? "Hide these unmodified lines" : "Show these unmodified lines"}
      >
        <span className="fd-gap-caret" aria-hidden="true">
          {caret}
        </span>
        <span className="fd-gap-label">
          {label}
          {context ? (
            // RVW-HUNKBAND · the @@ context (enclosing function/section) rides
            // *inside* the fold band as a secondary, dimmer tail — Codex shows one
            // grey band per gap, not a fold band stacked on a `.dl-hunk` heading.
            <span className="fd-gap-context ml-2 text-[11px] text-dim">{context}</span>
          ) : null}
        </span>
      </button>
    );
  };

  // DIFF-SPLIT-ADDED · a purely added or deleted file has no opposite side to
  // sit beside, so side-by-side split would render one real column next to a
  // half-width empty one — pushing the actual code past the viewport's right
  // edge (the added lines only start where the vanished old column ends). Codex
  // renders single-sided files as one column; we fall back to the inline
  // (single-column) path below so the content is visible from the left even when
  // the user has split selected. Modified files still take the split branch.
  if (effView === "split" && parsed.status !== "added" && parsed.status !== "deleted") {
    return (
      <div className="fd-body fd-split">
        {splitRows(parsed.rows).map((sr, i) =>
          sr.hunk !== undefined ? (
            sr.hunk ? (
              <div className="dl-hunk dl-hunk-span" key={i}>{sr.hunk}</div>
            ) : hunkCount > 1 ? (
              <div className="dl-hunk dl-hunk-span dl-hunk-blank" key={i} aria-hidden="true" />
            ) : null
          ) : (
            <div className="dls" key={i}>
              <span className={"dls-marker" + (sr.left?.kind === "add" || sr.right?.kind === "add" ? " add" : sr.left?.kind === "del" || sr.right?.kind === "del" ? " del" : "")} />
              <span className="dl-no">{sr.left?.oldNo ?? ""}</span>
              <span className={"dls-half " + halfKind(sr.left, "left")}>
                <span className="dl-sign">{rowSign(sr.left)}</span>
                {sr.left && renderCode(sr.left.text, lang)}
              </span>
              <span className="dl-no">{sr.right?.newNo ?? ""}</span>
              <span className={"dls-half " + halfKind(sr.right, "right")}>
                <span className="dl-sign">{rowSign(sr.right)}</span>
                {sr.right && renderCode(sr.right.text, lang)}
              </span>
            </div>
          ),
        )}
      </div>
    );
  }

  const ctxRow = (r: DiffRow, key: string) => (
    <div className="dl" key={key}>
      <span className="dl-marker" />
      <span className="dl-no">{r.newNo ?? ""}</span>
      <span className="dl-text">
        <span className="dl-sign"> </span>
        {renderCode(r.text, lang)}
      </span>
    </div>
  );

  return (
    <div className="fd-body">
      {parsed.rows.map((r, i) => {
        if (r.kind === "hunk") {
          const gap = gaps.get(i);
          // RVW-HUNKBAND · when this hunk has a fold band, the @@ context folds
          // *into* that band (as a dim tail on the label) — we must not also emit
          // a `.dl-hunk` heading, or the two `bg-panel-2` bands stack and read as
          // one duplicated grey band instead of Codex's single collapser.
          const bandEl = gap ? band(i, gap, gap.start === 1 ? "leading" : "interior", r.text) : null;
          const revealed = gap && open.has(i) ? revealedRows(gap).map((cr, k) => ctxRow(cr, i + ":rv:" + k)) : null;
          const header = bandEl ? null : r.text ? (
            <div className="dl-hunk" key={i + ":h"}>{r.text}</div>
          ) : hunkCount > 1 ? (
            <div className="dl-hunk dl-hunk-blank" key={i + ":h"} aria-hidden="true" />
          ) : null;
          if (!bandEl && !header) return null;
          return (
            <div key={i}>
              {bandEl}
              {revealed}
              {header}
            </div>
          );
        }
        return (
          <div className={"dl " + (r.kind === "ctx" ? "" : r.kind)} key={i}>
            <span className={"dl-marker" + (r.kind === "ctx" ? "" : " " + r.kind)} />
            <span className="dl-no">{(r.kind === "del" ? r.oldNo : r.newNo) ?? ""}</span>
            <span className="dl-text">
              <span className="dl-sign">{rowSign(r)}</span>
              {renderCode(r.text, lang)}
            </span>
          </div>
        );
      })}
      {/* RD-2 · the tail: last hunk → EOF. Same band, keyed past the last row. */}
      {trailGap && (
        <>
          {band(trailKey, trailGap, "trailing")}
          {open.has(trailKey) && revealedRows(trailGap).map((cr, k) => ctxRow(cr, trailKey + ":rv:" + k))}
        </>
      )}
    </div>
  );
}
