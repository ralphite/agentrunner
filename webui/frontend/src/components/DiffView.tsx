import { useEffect, useState } from "react";
import { Rows, Columns, MagnifyingGlass, GitBranch } from "@phosphor-icons/react";
import { AR } from "../api";
import { useStore } from "../store";
import { loadGitPrefs } from "../theme";
import type { DiffResp } from "../types";
import { parseFileDiff, splitDiff, splitPath, splitRows, highlightLine, langFromPath, type DiffRow } from "../diffSummary";

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

const rowSign = (r?: DiffRow) => (!r ? "" : r.kind === "add" ? "+" : r.kind === "del" ? "−" : " ");
const halfKind = (r: DiffRow | undefined, side: "left" | "right") =>
  !r ? "empty" : side === "left" && r.kind === "del" ? "del" : side === "right" && r.kind === "add" ? "add" : "";

export function DiffView({ sid }: { sid: string }) {
  const { toast, openPrompt, openModal } = useStore();
  const [data, setData] = useState<DiffResp | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
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
    AR.diff(sid)
      .then((d) => {
        setData(d);
        setErr("");
      })
      .catch((e) => setErr(e.message));
  };
  useEffect(load, [sid]);

  // Codex review→commit: stage & commit the workspace changes from the diff.
  const commit = () => {
    openPrompt({
      title: "Commit changes",
      label: "commit message",
      // Seed from the Settings › Git commit-message template (INC-41 H4).
      initial: loadGitPrefs().commitTemplate,
      submitLabel: "Commit",
      onSubmit: (message) => void doCommit(message),
    });
  };
  const doCommit = async (message: string) => {
    setBusy(true);
    try {
      await AR.commit(sid, message);
      toast("committed", "info");
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

  if (err) return <div className="diffwrap"><div className="chip bad">{err}</div></div>;
  if (!data) return <div className="diffwrap dim">loading diff…</div>;

  if (!data.known)
    return (
      <div className="diffwrap">
        <div className="diff-empty">
          <b>Workspace unavailable</b>
          <span>This session predates workspace metadata, so AgentRunner cannot reconstruct its changes view.</span>
          <button onClick={load}>Try again</button>
        </div>
      </div>
    );
  if (data.nested)
    return (
      <div className="diffwrap">
        <div className="diff-empty">
          <b>Changes can't be tracked here yet</b>
          <span>This task's workspace sits inside another repository, so its files aren't tracked on their own.</span>
          <button className="primary" onClick={gitInit} disabled={busy} title="git init in the workspace — safe, local-only">
            Track changes (git init)
          </button>
        </div>
      </div>
    );
  if (!data.isRepo)
    return (
      <div className="diffwrap">
        <div className="diff-empty">
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
  const empty = files.length === 0 && untracked.length === 0;

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
        {(() => {
          // Middle-ellipsize the workspace path (keep the head + the last
          // segment) so a long absolute path stays identifiable at both ends;
          // the full path is on hover (title).
          const ws = data.workspace || "";
          const cut = ws.lastIndexOf("/");
          const head = cut > 0 ? ws.slice(0, cut + 1) : "";
          const tail = cut >= 0 ? ws.slice(cut + 1) : ws;
          return (
            <span className="diffbar-path mono dim" title={data.workspace}>
              <span className="dp-head">{head}</span>
              <span className="dp-tail">{tail}</span>
            </span>
          );
        })()}
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
        {!empty && (
          <span className="diff-summary">
            {files.length} file{files.length === 1 ? "" : "s"}
            {totalAdd > 0 && <span className="add"> +{totalAdd}</span>}
            {totalDel > 0 && <span className="del"> −{totalDel}</span>}
          </span>
        )}
        <span className="spacer" />
        {files.length > 1 && (
          <label className="diff-filter" title="Filter files by path">
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
          <button className="sm" onClick={() => setAll(!allOpen)} title={allOpen ? "Collapse all files" : "Expand all files"}>
            {allOpen ? "Collapse all" : "Expand all"}
          </button>
        )}
        {!empty && (
          <button className="sm primary" onClick={commit} disabled={busy} title="git add -A && git commit the workspace changes (local commit, no push)">
            Commit changes…
          </button>
        )}
        {data.worktree && data.mainRepo && (
          <button
            className="sm"
            onClick={() => applyBack(data.mainRepo!)}
            disabled={busy || empty}
            title={"Apply these changes back onto " + data.mainRepo + " (unstaged, for review)"}
          >
            Apply to project…
          </button>
        )}
        {data.worktree && (
          <button className="sm" onClick={removeWorktree} disabled={busy} title="Delete this worktree checkout and prune it from git">
            Remove worktree…
          </button>
        )}
        <button className="sm" onClick={load}>
          Refresh
        </button>
      </div>
      {empty && <div className="dim" style={{ padding: 12 }}>No changes in the workspace.</div>}
      {!empty && q && shown.length === 0 && (
        <div className="dim" style={{ padding: 12 }}>No files match “{fileQuery}”.</div>
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
            {effView === "split" ? (
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
            ) : (
              <div className="fd-body">
                {parsed.rows.map((r, i) =>
                  r.kind === "hunk" ? (
                    r.text ? (
                      <div className="dl-hunk" key={i}>{r.text}</div>
                    ) : hunkCount > 1 ? (
                      <div className="dl-hunk dl-hunk-blank" key={i} aria-hidden="true" />
                    ) : null
                  ) : (
                    <div className={"dl " + (r.kind === "ctx" ? "" : r.kind)} key={i}>
                      <span className="dl-no">{r.oldNo ?? ""}</span>
                      <span className="dl-no">{r.newNo ?? ""}</span>
                      <span className="dl-text">
                        <span className="dl-sign">{rowSign(r)}</span>
                        {renderCode(r.text, lang)}
                      </span>
                    </div>
                  ),
                )}
              </div>
            )}
          </details>
        );
      })}
    </div>
  );
}
