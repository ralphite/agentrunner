import { useEffect, useRef, useState } from "react";
import {
  Rows,
  Columns,
  MagnifyingGlass,
  GitBranch,
  GitCommit,
  CaretDown,
  CaretRight,
  CaretUp,
  CaretUpDown,
  ArrowClockwise,
  ArrowsHorizontal,
  ArrowsOutLineVertical,
  ArrowsInLineVertical,
  DotsThree,
  TextAlignLeft,
  X,
  FileDashed,
  FileMagnifyingGlass,
  FolderDashed,
  ClockCounterClockwise,
} from "@phosphor-icons/react";
import { AR } from "../api";
import { useStore } from "../store";
import { loadGitPrefs } from "../theme";
import type { DiffResp, DiffScope } from "../types";
import { parseFileDiff, defaultOpenByPath, splitDiff, splitPath, splitRows, highlightLine, hunkGaps, trailingGapKey, langFromPath, type ContextGap, type DiffRow, type FileStatus, type ParsedFileDiff } from "../diffSummary";
import { Popover, PopItem, PopSection } from "./Popover";
import "../styles.diff.css";

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

// INC-41 DF-D5 · the whole sentence the hidden-files note used to try (and fail)
// to fit on one ellipsized line. It lives in the row's tooltip now; the row
// itself states the two facts that fit.
const HIDDEN_NOTE_TITLE =
  "Untracked files that look generated — dependencies, build output — are omitted so the review stays responsive. Every source file remains visible.";

const rowSign = (r?: DiffRow) => (!r ? "" : r.kind === "add" ? "+" : r.kind === "del" ? "−" : " ");
const halfKind = (r: DiffRow | undefined, side: "left" | "right") =>
  !r ? "empty" : side === "left" && r.kind === "del" ? "del" : side === "right" && r.kind === "add" ? "add" : "";

// INC-41 DF-4 · the diff's line-wrap preference, shared by every file and every
// session (the conversation's code blocks keep theirs per-block because a page
// holds dozens of unrelated blocks; a review is one surface, so one switch).
// Kept in localStorage rather than the store: a display preference the user sets
// once, not session state — and the store is off this change's touch list.
const WRAP_KEY = "ar.diff.wrap";
const loadWrap = (): boolean => {
  try {
    return localStorage.getItem(WRAP_KEY) === "1";
  } catch {
    return false; // private mode / storage disabled: wrap simply doesn't persist
  }
};
const saveWrap = (on: boolean) => {
  try {
    localStorage.setItem(WRAP_KEY, on ? "1" : "0");
  } catch {
    /* ignore */
  }
};

// FileHead is the one file header in the review — Codex has exactly one kind of
// changed-file card, and after DF-3 so do we: tracked edits and untracked new
// files render through this same summary (caret + M/A/D glyph + path + `+x −y`
// + badges) and the same expandable body underneath.
function FileHead({
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
  const binary = badges.includes("binary");
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
        {base}
      </span>
      {/* RD-4: counts sit right after the filename (Codex: `docs/DESIGN.md +8 -4`),
          both numbers always rendered — a pure deletion reads "+0 −176", not a
          lone "−176". `add === null` is the one honest gap: an untracked file's
          line count is only known once its blob is in hand. */}
      {!binary && (
        <span className="fd-counts">
          <span className="add">+{add === null ? "…" : add}</span>
          <span className="del">−{del}</span>
        </span>
      )}
      {/* INC-41 DF-D6 · badges are a property of *this file*, so they travel with
          its name — right after the counts, the way "binary" reads as part of the
          line `A bin/ar [binary]`. Behind the elastic gap they ended up ~475px
          from the filename, hard against the panel's right edge, where they read
          as a column of their own belonging to nothing in particular. The gap
          (.fd-spacer, not .fd-path — styles.panel.css overrides styles.css's
          `.fd-path{flex:1}`) now sits last and simply absorbs the leftover. */}
      {badges
        .filter((b) => !GLYPH_BADGES.has(b))
        .map((b) => (
          <span className="fd-badge" key={b}>{b}</span>
        ))}
      <span className="fd-spacer" aria-hidden="true" />
    </summary>
  );
}

export function DiffView({ sid, onClose }: { sid: string; onClose?: () => void }) {
  const { toast, openPrompt, openModal } = useStore();
  const [data, setData] = useState<DiffResp | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [scope, setScope] = useState<DiffScope>("working-tree");
  const requestID = useRef(0);
  // Fold-all override: null = every file follows its own default (INC-41 RD-1 —
  // per-file disclosure, so one huge file no longer folds its small neighbours);
  // true/false = the user pressed Expand-all / Collapse-all. Bumping the epoch
  // remounts the <details> so a global toggle wins over any manual per-file
  // toggling since the last one.
  const [override, setOverride] = useState<boolean | null>(null);
  const [foldEpoch, setFoldEpoch] = useState(0);
  const setAll = (open: boolean) => {
    setOverride(open);
    setFoldEpoch((e) => e + 1);
  };
  // D2 file filter + D4 inline/split view. Split needs room; below ~900px it
  // falls back to inline so two columns never crush the diff column.
  const [fileQuery, setFileQuery] = useState("");
  const [view, setView] = useState<"inline" | "split">("inline");
  // DF-4 · soft-wrap long diff lines (see WRAP_KEY above). Off = Codex's default
  // (one horizontal scroll for the whole rail); on = nothing is clipped.
  const [wrap, setWrap] = useState(loadWrap);
  const toggleWrap = () =>
    setWrap((w) => {
      saveWrap(!w);
      return !w;
    });
  const [narrow, setNarrow] = useState(() => window.matchMedia("(max-width: 900px)").matches);
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 900px)");
    const sync = () => setNarrow(mq.matches);
    mq.addEventListener("change", sync);
    return () => mq.removeEventListener("change", sync);
  }, []);
  const effView = narrow ? "inline" : view;
  // DF-1 · the review rail is ~56% of the window, so below ~1400px the worktree
  // chip's text is the first thing with nowhere to go. It shrinks (never its
  // neighbours — see .diffwrap .diffbar in styles.css), and here it stops being
  // a half-word clipped mid-glyph and becomes an honest icon-only chip; the full
  // "worktree of <repo> · <branch>" stays one hover away in its title.
  const [chipCompact, setChipCompact] = useState(() => window.matchMedia("(max-width: 1400px)").matches);
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 1400px)");
    const sync = () => setChipCompact(mq.matches);
    mq.addEventListener("change", sync);
    return () => mq.removeEventListener("change", sync);
  }, []);

  const load = () => {
    const currentRequest = ++requestID.current;
    setData(null);
    setErr("");
    AR.diff(sid, scope)
      .then((d) => {
        if (currentRequest !== requestID.current) return;
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
      initial: loadGitPrefs().commitTemplate,
      submitLabel: thenPush ? "Commit & push" : "Commit",
      onSubmit: (message) => void doCommit(message, thenPush),
    });
  };
  const doCommit = async (message: string, thenPush = false) => {
    setBusy(true);
    try {
      await AR.commit(sid, message);
      if (thenPush) {
        const r = await AR.push(sid);
        toast(r.branch ? `committed & pushed ${r.branch}` : "committed & pushed", "info");
      } else {
        toast("committed", "info");
      }
      load();
    } catch (e: any) {
      toast(e.message);
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
      const r = await AR.push(sid);
      toast(r.branch ? `pushed ${r.branch}` : "pushed", "info");
      load();
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  // Apply the worktree's changes back onto its main checkout (INC-49) — Codex's
  // "Apply changes". Lands unstaged in the project so the user reviews there; a
  // conflict is reported and the project is left untouched.
  const applyBack = (mainRepo: string) => {
    openModal({
      kind: "confirm",
      title: "Apply changes to project?",
      body: `Applies this worktree's changes onto ${mainRepo} (left unstaged for you to review and commit there). If they don't apply cleanly, nothing is changed and the conflict is reported.`,
      confirmLabel: "Apply changes",
      onConfirm: async () => {
        setBusy(true);
        try {
          const r = await AR.applyWorktree(sid);
          toast(r.applied ? "applied to project — review the changes there" : "no changes to apply", "info");
          load();
        } catch (e: any) {
          toast(e.message);
        } finally {
          setBusy(false);
        }
      },
    });
  };

  // Remove the worktree checkout + prune (INC-49). A dirty worktree is refused
  // first; the backend's structured refusal turns into a force confirmation so
  // unapplied work is never silently discarded.
  const forceRemove = async () => {
    setBusy(true);
    try {
      await AR.removeWorktree(sid, true);
      toast("worktree removed", "info");
      load();
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };
  const removeWorktree = () => {
    openModal({
      kind: "confirm",
      title: "Remove worktree?",
      body: "Deletes this isolated checkout and prunes it from git. Your project and any applied changes are unaffected.",
      confirmLabel: "Remove worktree",
      danger: true,
      onConfirm: async () => {
        setBusy(true);
        try {
          await AR.removeWorktree(sid, false);
          toast("worktree removed", "info");
          load();
        } catch (e: any) {
          if (/unapplied changes/.test(e.message)) {
            // The confirm modal auto-closes itself right after this handler
            // resolves, which would clobber a modal opened synchronously here —
            // so defer the force prompt to the next tick.
            setTimeout(
              () =>
                openModal({
                  kind: "confirm",
                  title: "Discard unapplied changes?",
                  body: "This worktree has changes that haven't been applied to the project. Removing it deletes them permanently. Apply the changes first if you want to keep them.",
                  confirmLabel: "Delete anyway",
                  danger: true,
                  onConfirm: forceRemove,
                }),
              0,
            );
          } else {
            toast(e.message);
          }
        } finally {
          setBusy(false);
        }
      },
    });
  };

  // Turn the workspace into its own repo, then re-load — offered from the
  // non-repo / nested empty states so "no diff" is always actionable.
  const gitInit = async () => {
    setBusy(true);
    try {
      await AR.gitInit(sid);
      toast("workspace is now a git repository — future changes will show here", "info");
      load();
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  const scopeControl = (
    <Popover
      panelClass="diff-scope-menu"
      trigger={(open, toggle) => (
        <button
          className={"diff-scope-trigger" + (open ? " active" : "")}
          onClick={toggle}
          aria-label="Change diff scope"
          aria-haspopup="menu"
          aria-expanded={open}
          title="Choose which workspace changes to review"
        >
          {scope === "working-tree" ? "Working tree" : "Last turn"}
          <CaretDown size={12} />
        </button>
      )}
    >
      {(close) => (
        <PopSection label="Compare changes">
          <PopItem
            title="Working tree"
            desc="All uncommitted workspace changes"
            active={scope === "working-tree"}
            onClick={() => {
              setScope("working-tree");
              close();
            }}
          />
          <PopItem
            title="Last turn"
            desc="Since the latest human turn began"
            active={scope === "last-turn"}
            onClick={() => {
              setScope("last-turn");
              close();
            }}
          />
        </PopSection>
      )}
    </Popover>
  );

  // INC-41 RV-1 · the panel's only chrome is this one row. The Changes title bar
  // that used to sit above it (`.changes-panel-head`) was a second copy of the
  // topbar's `Changes` pill, so it's gone and its ✕ moved here — Codex's review
  // rail likewise opens straight onto the diff under a single toolbar.
  const closeBtn = onClose ? (
    <button
      className="sm ghost diff-iconbtn"
      onClick={onClose}
      aria-label="Close changes"
      title="Close changes (back to the conversation)"
    >
      <X size={15} />
    </button>
  ) : null;

  const stateBar = (
    <div className="diffbar diffbar-state">
      {scopeControl}
      <span className="spacer" />
      <button className="sm ghost diff-iconbtn" onClick={load} aria-label="Refresh changes" title="Refresh changes">
        <ArrowClockwise size={15} />
      </button>
      {closeBtn}
    </div>
  );

  if (err)
    return (
      <div className="diffwrap">
        {stateBar}
        <div className="diff-empty">
          <b>Couldn’t load changes</b>
          <span>{err}</span>
          <button onClick={load}>Try again</button>
        </div>
      </div>
    );
  if (!data) return <div className="diffwrap">{stateBar}<div className="diff-loading dim">Loading changes…</div></div>;

  if (scope === "last-turn" && data.available === false)
    return (
      <div className="diffwrap">
        {stateBar}
        <div className="diff-empty">
          <ClockCounterClockwise size={26} weight="light" />
          <b>Last turn unavailable</b>
          <span>{data.reason || "This task has no durable workspace baseline for its latest human turn."}</span>
          <span className="dim">Working tree remains available for the task's current uncommitted changes.</span>
        </div>
      </div>
    );

  if (!data.known)
    return (
      <div className="diffwrap">
        {stateBar}
        <div className="diff-empty">
          <FolderDashed size={26} weight="light" />
          <b>Workspace unavailable</b>
          <span>This session predates workspace metadata, so AgentRunner cannot reconstruct its changes view.</span>
          <button onClick={load}>Try again</button>
        </div>
      </div>
    );
  if (scope === "working-tree" && data.nested)
    return (
      <div className="diffwrap">
        {stateBar}
        <div className="diff-empty">
          <GitBranch size={26} weight="light" />
          <b>Changes can't be tracked here yet</b>
          <span>This task's workspace sits inside another repository, so its files aren't tracked on their own.</span>
          <button className="primary" onClick={gitInit} disabled={busy} title="git init in the workspace — safe, local-only">
            Track changes (git init)
          </button>
        </div>
      </div>
    );
  if (scope === "working-tree" && !data.isRepo)
    return (
      <div className="diffwrap">
        {stateBar}
        <div className="diff-empty">
          <GitBranch size={26} weight="light" />
          <b>No Git changes to review</b>
          <span>This task's workspace has no version control yet.</span>
          <button className="primary" onClick={gitInit} disabled={busy} title="git init in the workspace — safe, local-only">
            Track changes (git init)
          </button>
        </div>
      </div>
    );

  const files = splitDiff(data.diff || "");
  const untracked = data.untracked || [];
  const hiddenUntracked = data.hiddenUntracked || 0;
  const empty = files.length === 0 && untracked.length === 0 && hiddenUntracked === 0;

  // Per-file +/- counts (from the diff itself, so untracked-content blocks count
  // too) — Codex shows these next to each file and a total at the top.
  const stats = files.map((f) => ({ f, add: f.add, del: f.del }));
  const totalAdd = stats.reduce((s, x) => s + x.add, 0);
  const totalDel = stats.reduce((s, x) => s + x.del, 0);
  const q = fileQuery.trim().toLowerCase();
  const shown = q ? stats.filter((s) => s.f.path.toLowerCase().includes(q)) : stats;
  // DF-3 · untracked files are files: they go through the same filter, the same
  // count, the same Expand/Collapse-all as everything else in the review.
  const shownUntracked = q ? untracked.filter((p) => p.toLowerCase().includes(q)) : untracked;
  const fileCount = files.length + untracked.length;
  const shownCount = shown.length + shownUntracked.length;
  // Per-file default disclosure (RD-1) — computed over the whole review, not just
  // the filtered subset, so filtering never changes a file's default state.
  const defaults = defaultOpenByPath(files);
  const isOpen = (path: string) => override ?? defaults.get(path) ?? true;
  // An untracked entry that survives to this list is one git refused to inline
  // (binary, >256KB, or past the inline budget — webui/meta.go), so it folds by
  // default for the same reason DF-2 folds generated files: a review shouldn't
  // open on a wall of content nobody reads. Expand-all still opens it.
  const untrackedOpen = override ?? false;
  const allShownOpen =
    shownCount > 0 && shown.every((s) => isOpen(s.f.path)) && (shownUntracked.length === 0 || untrackedOpen);

  return (
    <div className={"diffwrap" + (wrap ? " diff-wrap" : "")}>
      <div className="diffbar">
        {scopeControl}
        {/* DF-6 · both numbers, always — Codex's toolbar reads `+649 -57`, and a
            review with nothing deleted reads `+1 −0`, not a lone `+1`. The old
            `> 0` guards meant the *same panel* stated its counts two different
            ways: this bar dropped the zero half while every file header below it
            (FileHead's `.fd-counts`, which never had a guard) kept it. Same
            markup, same colors, same tabular spacing as those headers now. */}
        {!empty && (
          <span className="diff-summary">
            <span className="add">+{totalAdd}</span>
            <span className="del">−{totalDel}</span>
          </span>
        )}
        {data.worktree && (
          <span
            // RV-1: nowrap — the badge used to wrap onto three lines inside the
            // toolbar ("worktree of / agentrunner · / detached"), single-handedly
            // pushing the bar past Codex's one-row height.
            //
            // DF-1: …and then, as an unshrinkable 195px sentence ("worktree of
            // agentrunner · main"), it pushed the bar past the *panel* instead —
            // the ✕ landed outside it. The repo name lives in the tooltip (and in
            // the `…` menu's Apply/Remove lines) now; the chip states the two
            // facts the review actually needs — this is a worktree, on this
            // branch — and it is the one control allowed to give way, ellipsizing
            // its branch as the panel narrows.
            className="diff-wt-badge inline-flex min-w-0 items-center gap-[4px] whitespace-nowrap text-[11px] text-ink-2 bg-panel-2 border border-line-2 rounded-[5px] px-[6px] py-[2px]"
            title={
              (data.mainRepo ? "Isolated worktree of " + data.mainRepo : "Isolated git worktree") +
              (data.branch ? " · branch " + data.branch : " · detached HEAD")
            }
          >
            <GitBranch size={12} className="shrink-0" />
            {/* One truncating run, not several shrink-0 words: whatever width is
                left, the chip ends in an ellipsis instead of a clipped glyph. */}
            {!chipCompact && (
              <span className="min-w-0 truncate">
                worktree <span className="dim">· {data.branch || "detached"}</span>
              </span>
            )}
          </span>
        )}
        <span className="spacer" />
        {/* RV-1 · the low-frequency, workspace-level actions (refresh, and the
            worktree's Apply / Remove) live behind one `…`, so the bar can no
            longer wrap to a second row in a worktree session. DF-1 · Expand /
            Collapse-all joined them: four resident controls (`…`, filter, split,
            Commit or push) plus the ✕ is what a 658px panel can seat at 1440
            without crushing anything — the same shape Codex's review header has. */}
        <Popover
          align="right"
          panelClass="diff-more-menu"
          trigger={(open, toggle) => (
            <button
              className={"sm ghost diff-iconbtn" + (open ? " active" : "")}
              onClick={toggle}
              aria-label="More changes actions"
              aria-haspopup="menu"
              aria-expanded={open}
              title="More actions"
            >
              <DotsThree size={18} weight="bold" />
            </button>
          )}
        >
          {(close) => (
            <PopSection label="Changes">
              {fileCount > 1 && (
                <PopItem
                  icon={allShownOpen ? <ArrowsInLineVertical size={15} /> : <ArrowsOutLineVertical size={15} />}
                  title={allShownOpen ? "Collapse all files" : "Expand all files"}
                  desc={allShownOpen ? "Fold every file down to its header" : "Open every file's diff"}
                  onClick={() => {
                    close();
                    setAll(!allShownOpen);
                  }}
                />
              )}
              <PopItem
                title="Refresh changes"
                desc="Re-read the workspace diff"
                onClick={() => {
                  close();
                  load();
                }}
              />
              {scope === "working-tree" && data.worktree && data.mainRepo && (
                <PopItem
                  title="Apply to project…"
                  desc={"Apply these changes back onto " + data.mainRepo + " (unstaged, for review)"}
                  disabled={busy || empty}
                  onClick={() => {
                    close();
                    applyBack(data.mainRepo!);
                  }}
                />
              )}
              {scope === "working-tree" && data.worktree && (
                <PopItem
                  title="Remove worktree…"
                  desc="Delete this worktree checkout and prune it from git"
                  danger
                  disabled={busy}
                  onClick={() => {
                    close();
                    removeWorktree();
                  }}
                />
              )}
            </PopSection>
          )}
        </Popover>
        {/* DF-1 · the filter is an icon, not a permanently-open 150px input —
            Codex's review header does the same (a magnifier button that opens a
            field). As a resident input it was the second-largest thing on a bar
            that already didn't fit, and flexbox paid for it by crushing the
            split/unified toggle to 2px and shoving the ✕ out of the panel. The
            field now opens in a popover, where it has room; the trigger stays
            lit while a query is filtering the list, so a filtered review can
            never look like an empty one. */}
        {fileCount > 1 && (
          <Popover
            align="right"
            panelClass="w-[248px] max-w-[calc(100vw-24px)]"
            trigger={(open, toggle) => (
              <button
                className={"sm ghost diff-iconbtn" + (open || fileQuery ? " active" : "")}
                onClick={toggle}
                aria-label="Filter files by path"
                aria-haspopup="menu"
                aria-expanded={open}
                title={fileQuery ? "Filtering files by “" + fileQuery + "”" : "Filter files by path"}
              >
                <MagnifyingGlass size={15} />
              </button>
            )}
          >
            {() => (
              <PopSection label="Filter files">
                <label className="mx-[6px] flex items-center gap-[6px] rounded-[8px] border border-line bg-panel px-[9px] py-[5px] text-dim focus-within:border-[var(--rs-accent)]">
                  <MagnifyingGlass size={13} className="shrink-0" />
                  <input
                    data-popover-autofocus
                    className="min-w-0 flex-1 border-0 bg-transparent p-0 text-[12px] text-ink outline-none"
                    value={fileQuery}
                    onChange={(e) => setFileQuery(e.target.value)}
                    placeholder="Filter files…"
                    aria-label="Filter files by path"
                  />
                </label>
                <div className="mx-[6px] mt-[6px] text-[11px] text-dim">
                  {q ? `${shownCount} of ${fileCount} files match` : `${fileCount} files changed`}
                </div>
              </PopSection>
            )}
          </Popover>
        )}
        {/* DF-4 · the Wrap switch. Same two icons, same wording, same aria-pressed
            contract as the conversation's code blocks (Markdown.tsx CodeBlock) —
            it was absurd that a fenced snippet in the chat could soft-wrap while
            the review, where long lines actually hurt, hard-clipped them behind a
            per-file scrollbar. Icon-only here because DF-1's whole point was that
            this bar has no spare width; the label lives in the tooltip. */}
        {!empty && (
          <button
            className={"sm ghost diff-iconbtn diff-wrap-btn" + (wrap ? " active" : "")}
            onClick={toggleWrap}
            aria-label="Wrap long lines"
            aria-pressed={wrap}
            title={wrap ? "Disable line wrap" : "Wrap long lines"}
          >
            {wrap ? <TextAlignLeft size={15} /> : <ArrowsHorizontal size={15} />}
          </button>
        )}
        {!empty && (
          <div className="diff-viewtoggle" role="group" aria-label="Diff layout">
            <button
              className={"sm icon" + (effView === "inline" ? " sel" : "")}
              onClick={() => setView("inline")}
              title="Inline view"
              aria-pressed={effView === "inline"}
            >
              <Rows size={14} />
            </button>
            <button
              className={"sm icon" + (effView === "split" ? " sel" : "")}
              onClick={() => setView("split")}
              disabled={narrow}
              title={narrow ? "Split view needs a wider window" : "Split view"}
              aria-pressed={effView === "split"}
            >
              <Columns size={14} />
            </button>
          </div>
        )}
        {scope === "working-tree" && !empty && (
          <Popover
            align="right"
            panelClass="w-[264px] max-w-[calc(100vw-24px)]"
            trigger={(open, toggle) => (
              <button
                className={"sm diff-commit-btn" + (open ? " active" : "")}
                onClick={toggle}
                disabled={busy}
                aria-label="Commit or push"
                aria-haspopup="menu"
                aria-expanded={open}
                title="Commit or push the workspace changes"
              >
                <GitCommit size={14} />
                Commit or push
                <CaretDown size={12} className="diff-commit-caret" />
              </button>
            )}
          >
            {(close) => (
              <PopSection label="Commit or push">
                <PopItem
                  title="Commit"
                  desc="git add -A && git commit locally (no push)"
                  onClick={() => {
                    close();
                    commit();
                  }}
                />
                <PopItem
                  title="Commit &amp; push"
                  desc="Commit locally, then push to the upstream branch"
                  onClick={() => {
                    close();
                    commitAndPush();
                  }}
                />
                <PopItem
                  title="Push"
                  desc="Push existing commits to the upstream branch"
                  onClick={() => {
                    close();
                    void doPush();
                  }}
                />
              </PopSection>
            )}
          </Popover>
        )}
        {closeBtn}
      </div>
      {/* INC-41 L4 · every "nothing to show" in this panel speaks the timeline's
          empty-state language (icon + title + one line of guidance) via the
          shared `.diff-empty` shape — a bare grey sentence was the odd one out. */}
      {empty && (
        <div className="diff-empty">
          <FileDashed size={26} weight="light" />
          {scope === "last-turn" ? (
            <>
              <b>No changes this turn</b>
              <span>The agent hasn't touched the workspace since the latest human turn began.</span>
            </>
          ) : (
            <>
              <b>No changes yet</b>
              <span>Edits the agent makes to the workspace will show up here.</span>
            </>
          )}
        </div>
      )}
      {!empty && q && shownCount === 0 && (
        <div className="diff-empty">
          <FileMagnifyingGlass size={26} weight="light" />
          <b>No matching files</b>
          <span>No changed file’s path contains “{fileQuery}”. Clear the filter to see all {fileCount} of them.</span>
        </div>
      )}
      {/* INC-41 DF-D5 · this note is one nowrap+ellipsis line by design (RV-1: a
          fact worth a sentence, not an 80px card) — but the sentence it carried
          needed ~430px of tail and the panel gives it ~230px, so its second half
          ("Source files remain visible.") was unreachable at *every* window
          width, with no title to fall back on. The tail is now short enough to
          land, and the full explanation lives in the row's tooltip. */}
      {hiddenUntracked > 0 && !q && (
        <div className="diff-hidden-note" role="status" title={HIDDEN_NOTE_TITLE}>
          <b>{hiddenUntracked.toLocaleString()} generated files hidden</b>
          <span>Source files all still shown.</span>
        </div>
      )}
      {/* INC-41 DF-3 · these used to be a grey `new files (untracked) · N` strip
          of bare paths: no glyph, no `+N −0`, no line numbers, nothing to open —
          a second visual language for the files a review most wants to read, and
          they sat *above* every real file. They are ordinary file cards now. */}
      {shownUntracked.map((path) => (
        <UntrackedFile
          key={path + ":" + foldEpoch}
          sid={sid}
          path={path}
          effView={effView}
          defaultOpen={untrackedOpen}
          // Same budget as FileBody's prefetch: on a small review, read the file
          // up front so its header can state a real `+N −0` instead of `+…`.
          prefetch={shownUntracked.length <= 25}
        />
      ))}
      {shown.map(({ f, add, del }) => {
        const parsed = parseFileDiff(f.lines);
        const lang = langFromPath(f.path);
        // A hunk header with no @@ context text is pure noise: a lone "⋯" band.
        // Drop it entirely when the file has a single hunk (nothing to separate);
        // with several hunks it becomes a compact hairline separator instead.
        const hunkCount = parsed.rows.reduce((n, r) => n + (r.kind === "hunk" ? 1 : 0), 0);
        const open = isOpen(f.path);
        return (
          <details className="filediff" key={f.path + ":" + foldEpoch} open={open}>
            {/* RV-3 · the disclosure caret: `list-style: none` killed the platform
                triangle, so a collapsed file was a lone header with no hint that a
                body was hiding under it. Header shape now lives in FileHead, which
                untracked files share (DF-3). */}
            <FileHead path={f.path} status={parsed.status} add={add} del={del} badges={parsed.badges} />
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
              prefetch={open && effView === "inline" && shown.length <= 25}
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
// disclosure — with their body read from the workspace on demand (AR.blob, the
// endpoint the "N unmodified lines" bands already use) and rendered as what it
// is: a file made entirely of added lines.
function UntrackedFile({
  sid,
  path,
  effView,
  defaultOpen,
  prefetch,
}: {
  sid: string;
  path: string;
  effView: "inline" | "split";
  defaultOpen: boolean;
  prefetch: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const [lines, setLines] = useState<string[] | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if ((!open && !prefetch) || lines || failed) return;
    let alive = true;
    AR.blob(sid, path)
      .then((r) => alive && setLines(r.lines))
      // Silent: a binary/oversized file is the expected failure here, not an
      // error the user has to act on. The card says so in place of its rows.
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [sid, path, open, prefetch, lines, failed]);

  const rows: DiffRow[] = (lines || []).map((text, i) => ({ kind: "add", newNo: i + 1, text }));
  const parsed: ParsedFileDiff = { badges: failed ? ["binary"] : [], status: "added", rows };
  // A file git can't show is a file with no countable lines — so it carries the
  // "binary" badge and FileHead prints no counts at all for it (DF-D3), exactly
  // as a *tracked* binary addition does. The two agree; neither invents a zero.
  const add = lines ? lines.length : failed ? 0 : null;

  return (
    <details
      className="filediff filediff-untracked"
      open={open}
      onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}
    >
      <FileHead path={path} status="added" add={add} del={0} badges={parsed.badges} />
      {failed ? (
        <div className="fd-nobody">Content isn’t shown — this file is binary or too large to display.</div>
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
// once (AR.blob) and reveals the hidden unmodified region in place; clicking the
// revealed region's header collapses it again. The split view keeps the plain
// hunk-separator rendering (its paired-column model has no per-row anchor to
// hang a band on), so context expansion lives in the default inline layout.
function FileBody({
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
  const [blobFailed, setBlobFailed] = useState(false);
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
    if (!prefetch || !needsBlob || blob || blobFailed) return;
    let alive = true;
    AR.blob(sid, path)
      .then((r) => alive && setBlob(r.lines))
      .catch(() => alive && setBlobFailed(true)); // silent: the diff itself still renders
    return () => {
      alive = false;
    };
  }, [sid, path, prefetch, needsBlob, blob, blobFailed]);

  const toggleGap = async (idx: number) => {
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
        const r = await AR.blob(sid, path);
        setBlob(r.lines);
      } catch (e: any) {
        toast(e.message);
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
  const band = (idx: number, gap: ContextGap, kind: "leading" | "interior" | "trailing") => {
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
    // the line-number gutter (same `calc(5ch + 27px)` as `.dl`, holding the
    // caret the way a row holds its number), and its label starts exactly where
    // the code column does. It used to be a `px-[10px]` flex row aligned with
    // neither, which read as a button bolted onto the diff rather than a fold in
    // it — Codex's band is a line of the file. Geometry lives in styles.diff.css
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
        <span className="fd-gap-label">{label}</span>
      </button>
    );
  };

  if (effView === "split") {
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
          const bandEl = gap ? band(i, gap, gap.start === 1 ? "leading" : "interior") : null;
          const revealed = gap && open.has(i) ? revealedRows(gap).map((cr, k) => ctxRow(cr, i + ":rv:" + k)) : null;
          const header = r.text ? (
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
