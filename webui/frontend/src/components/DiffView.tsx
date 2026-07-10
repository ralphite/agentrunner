import { useEffect, useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";
import type { DiffResp } from "../types";
import { parseFileDiff, splitDiff, splitPath } from "../diffSummary";

export function DiffView({ sid }: { sid: string }) {
  const { toast, openPrompt } = useStore();
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
      initial: "changes from agent session",
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

  return (
    <div className="diffwrap">
      <div className="diffbar">
        <span className="diffbar-path mono dim" title={data.workspace}>{data.workspace}</span>
        {!empty && (
          <span className="diff-summary">
            {files.length} file{files.length === 1 ? "" : "s"}
            {totalAdd > 0 && <span className="add"> +{totalAdd}</span>}
            {totalDel > 0 && <span className="del"> −{totalDel}</span>}
          </span>
        )}
        <span className="spacer" />
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
        <button className="sm" onClick={load}>
          Refresh
        </button>
      </div>
      {empty && <div className="dim" style={{ padding: 12 }}>No changes in the workspace.</div>}
      {untracked.length > 0 && (
        <div className="filediff">
          <div className="fd-head">
            new files (untracked) · {untracked.length}
          </div>
          <div className="fd-body">
            {untracked.map((f) => (
              <div className="dl add" key={f}>
                <span className="dl-no" />
                <span className="dl-no" />
                <span className="dl-text">+ {f}</span>
              </div>
            ))}
          </div>
        </div>
      )}
      {stats.map(({ f, add, del }) => {
        const parsed = parseFileDiff(f.lines);
        const { dir, base } = splitPath(f.path);
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
            <div className="fd-body">
              {parsed.rows.map((r, i) =>
                r.kind === "hunk" ? (
                  <div className="dl-hunk" key={i}>{r.text || "⋯"}</div>
                ) : (
                  <div className={"dl " + (r.kind === "ctx" ? "" : r.kind)} key={i}>
                    <span className="dl-no">{r.oldNo ?? ""}</span>
                    <span className="dl-no">{r.newNo ?? ""}</span>
                    <span className="dl-text">{r.text || " "}</span>
                  </div>
                ),
              )}
            </div>
          </details>
        );
      })}
    </div>
  );
}
