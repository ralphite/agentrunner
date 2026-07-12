import { useEffect, useRef, useState } from "react";
import {
  Rows,
  Columns,
  MagnifyingGlass,
  GitBranch,
  GitCommit,
  CaretDown,
  CaretUp,
  CaretUpDown,
  ArrowClockwise,
  ArrowsOutLineVertical,
  ArrowsInLineVertical,
  FileDashed,
  FileMagnifyingGlass,
  FolderDashed,
  ClockCounterClockwise,
} from "@phosphor-icons/react";
import { AR } from "../api";
import { useStore } from "../store";
import { loadGitPrefs } from "../theme";
import type { DiffResp, DiffScope } from "../types";
import { parseFileDiff, shouldExpandDiffByDefault, splitDiff, splitPath, splitRows, highlightLine, hunkGaps, langFromPath, type DiffRow, type FileStatus, type ParsedFileDiff } from "../diffSummary";
import { Popover, PopItem, PopSection } from "./Popover";

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

const rowSign = (r?: DiffRow) => (!r ? "" : r.kind === "add" ? "+" : r.kind === "del" ? "−" : " ");
const halfKind = (r: DiffRow | undefined, side: "left" | "right") =>
  !r ? "empty" : side === "left" && r.kind === "del" ? "del" : side === "right" && r.kind === "add" ? "add" : "";

export function DiffView({ sid }: { sid: string }) {
  const { toast, openPrompt, openModal } = useStore();
  const [data, setData] = useState<DiffResp | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [scope, setScope] = useState<DiffScope>("working-tree");
  const requestID = useRef(0);
  // fold-all state; bumping the epoch remounts the <details> so a global
  // toggle wins over any manual per-file toggling since the last one.
  const [allOpen, setAllOpen] = useState(true);
  const [foldEpoch, setFoldEpoch] = useState(0);
  const setAll = (open: boolean) => {
    setAllOpen(open);
    setFoldEpoch((e) => e + 1);
  };
  // D2 file filter + D4 inline/split view. Split needs room; below ~900px it
  // falls back to inline so two columns never crush the diff column.
  const [fileQuery, setFileQuery] = useState("");
  const [view, setView] = useState<"inline" | "split">("inline");
  const [narrow, setNarrow] = useState(() => window.matchMedia("(max-width: 900px)").matches);
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 900px)");
    const sync = () => setNarrow(mq.matches);
    mq.addEventListener("change", sync);
    return () => mq.removeEventListener("change", sync);
  }, []);
  const effView = narrow ? "inline" : view;

  const load = () => {
    const currentRequest = ++requestID.current;
    setData(null);
    setErr("");
    AR.diff(sid, scope)
      .then((d) => {
        if (currentRequest !== requestID.current) return;
        // Decide disclosure before exposing the payload to React. Otherwise a
        // large diff gets one fully-expanded paint before a later effect can
        // collapse it, which is exactly when the browser is most vulnerable to
        // jank or failure.
        setAllOpen(shouldExpandDiffByDefault(d.diff || ""));
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

  const stateBar = (
    <div className="diffbar diffbar-state">
      {scopeControl}
      <span className="spacer" />
      <button className="sm" onClick={load}>Refresh</button>
    </div>
  );

  if (err) return <div className="diffwrap">{stateBar}<div className="chip bad">{err}</div></div>;
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

  return (
    <div className="diffwrap">
      <div className="diffbar">
        {scopeControl}
        {!empty && (
          <span className="diff-summary">
            {totalAdd > 0 && <span className="add">+{totalAdd}</span>}
            {totalDel > 0 && <span className="del"> −{totalDel}</span>}
          </span>
        )}
        {data.worktree && (
          <span
            className="diff-wt-badge inline-flex items-center gap-[4px] text-[11px] text-ink-2 bg-panel-2 border border-line-2 rounded-[5px] px-[6px] py-[2px]"
            title={data.mainRepo ? "Isolated worktree of " + data.mainRepo : "Isolated git worktree"}
          >
            <GitBranch size={12} />
            worktree of <b className="text-ink font-medium">{(data.mainRepo || "").split("/").filter(Boolean).pop() || "project"}</b>
            {data.branch ? <span className="dim">· {data.branch}</span> : <span className="dim">· detached</span>}
          </span>
        )}
        <span className="spacer" />
        {files.length > 1 && (
          <label className={"diff-filter" + (fileQuery ? " has-query" : "")} title="Filter files by path">
            <MagnifyingGlass size={13} />
            <input
              value={fileQuery}
              onChange={(e) => setFileQuery(e.target.value)}
              placeholder="Filter files…"
              aria-label="Filter files by path"
            />
          </label>
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
        {files.length > 1 && (
          <button
            className="sm ghost diff-iconbtn"
            onClick={() => setAll(!allOpen)}
            aria-label={allOpen ? "Collapse all files" : "Expand all files"}
            title={allOpen ? "Collapse all files" : "Expand all files"}
          >
            {allOpen ? <ArrowsInLineVertical size={15} /> : <ArrowsOutLineVertical size={15} />}
          </button>
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
        {scope === "working-tree" && data.worktree && data.mainRepo && (
          <button
            className="sm"
            onClick={() => applyBack(data.mainRepo!)}
            disabled={busy || empty}
            title={"Apply these changes back onto " + data.mainRepo + " (unstaged, for review)"}
          >
            Apply to project…
          </button>
        )}
        {scope === "working-tree" && data.worktree && (
          <button className="sm" onClick={removeWorktree} disabled={busy} title="Delete this worktree checkout and prune it from git">
            Remove worktree…
          </button>
        )}
        <button className="sm ghost diff-iconbtn" onClick={load} aria-label="Refresh changes" title="Refresh changes">
          <ArrowClockwise size={15} />
        </button>
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
      {!empty && q && shown.length === 0 && (
        <div className="diff-empty">
          <FileMagnifyingGlass size={26} weight="light" />
          <b>No matching files</b>
          <span>No changed file’s path contains “{fileQuery}”. Clear the filter to see all {files.length} of them.</span>
        </div>
      )}
      {hiddenUntracked > 0 && !q && (
        <div className="diff-hidden-note" role="status">
          <b>{hiddenUntracked.toLocaleString()} generated or excess untracked files hidden</b>
          <span>Dependency/build output is omitted to keep review responsive. Source files remain visible.</span>
        </div>
      )}
      {untracked.length > 0 && !q && (
        <div className="filediff">
          <div className="fd-head">
            new files (untracked) · {untracked.length}
          </div>
          <div className="fd-body">
            {untracked.map((f) => (
              <div className="dl add" key={f}>
                <span className="dl-no" />
                <span className="dl-text"><span className="dl-sign">+</span>{f}</span>
              </div>
            ))}
          </div>
        </div>
      )}
      {shown.map(({ f, add, del }) => {
        const parsed = parseFileDiff(f.lines);
        const { dir, base } = splitPath(f.path);
        const lang = langFromPath(f.path);
        // A hunk header with no @@ context text is pure noise: a lone "⋯" band.
        // Drop it entirely when the file has a single hunk (nothing to separate);
        // with several hunks it becomes a compact hairline separator instead.
        const hunkCount = parsed.rows.reduce((n, r) => n + (r.kind === "hunk" ? 1 : 0), 0);
        return (
          <details className="filediff" key={f.path + ":" + foldEpoch} open={allOpen}>
            <summary className="fd-head mono">
              <span className={"fd-glyph fd-glyph-" + parsed.status} title={parsed.status} aria-hidden="true">
                {STATUS_GLYPH[parsed.status]}
              </span>
              <span className="fd-path" title={f.path}>
                {dir && <span className="fd-dir">{dir}</span>}
                {base}
              </span>
              {parsed.badges.map((b) => (
                <span className="fd-badge" key={b}>{b}</span>
              ))}
              <span className="fd-counts">
                {add > 0 && <span className="add">+{add}</span>}
                {del > 0 && <span className="del">−{del}</span>}
              </span>
            </summary>
            <FileBody sid={sid} path={f.path} parsed={parsed} lang={lang} effView={effView} hunkCount={hunkCount} />
          </details>
        );
      })}
    </div>
  );
}

// FileBody renders one file's diff rows, and — in the inline view — the
// clickable "N unmodified lines" collapser bands Codex shows before the first
// hunk and between hunks (P3). Clicking a band fetches the file's current text
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
}: {
  sid: string;
  path: string;
  parsed: ParsedFileDiff;
  lang: string;
  effView: "inline" | "split";
  hunkCount: number;
}) {
  const toast = useStore((s) => s.toast);
  const gaps = hunkGaps(parsed.rows);
  // Fetched file text (null until first reveal) and the set of hunk-row indices
  // whose gap is currently expanded.
  const [blob, setBlob] = useState<string[] | null>(null);
  const [open, setOpen] = useState<Set<number>>(new Set());
  const [loadingIdx, setLoadingIdx] = useState<number | null>(null);

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
        setLoadingIdx(null);
        return;
      }
      setLoadingIdx(null);
    }
    setOpen((prev) => new Set(prev).add(idx));
  };

  // The revealed unmodified lines for a gap, sliced by 1-based new-file numbers.
  const revealedRows = (gap: { start: number; end: number }): DiffRow[] => {
    if (!blob) return [];
    const out: DiffRow[] = [];
    for (let ln = gap.start; ln <= gap.end && ln - 1 < blob.length; ln++) {
      out.push({ kind: "ctx", newNo: ln, oldNo: ln, text: blob[ln - 1] });
    }
    return out;
  };

  // Collapser band shown before a hunk that hides unmodified lines. Expanded, it
  // becomes a thin "collapse" header above the revealed lines. `leading` marks
  // the gap that reaches the top of the file (start === 1): it can only open
  // downward, so it gets a single caret, while interior gaps span both
  // directions and get a two-way caret — matching Codex's context bands.
  const band = (idx: number, gap: { start: number; end: number }, leading: boolean) => {
    const n = gap.end - gap.start + 1;
    const expanded = open.has(idx);
    const caret = expanded ? <CaretUp size={12} /> : leading ? <CaretDown size={12} /> : <CaretUpDown size={12} />;
    return (
      <button
        type="button"
        className="flex items-center gap-[8px] w-full px-[10px] py-[6px] bg-panel-2 text-dim font-mono text-[11px] text-left cursor-pointer border-x-0 border-y border-y-line-2 hover:bg-blue-soft hover:text-blue disabled:opacity-60 disabled:cursor-default"
        onClick={() => void toggleGap(idx)}
        disabled={loadingIdx === idx}
        title={expanded ? "Hide these unmodified lines" : "Show these unmodified lines"}
      >
        <span className="shrink-0 inline-flex items-center justify-center border border-line-2 rounded-[5px] bg-panel px-[2px] py-[1px]">
          {caret}
        </span>
        {loadingIdx === idx ? "Loading…" : `${n.toLocaleString()} unmodified line${n === 1 ? "" : "s"}`}
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
          const bandEl = gap ? band(i, gap, gap.start === 1) : null;
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
    </div>
  );
}
