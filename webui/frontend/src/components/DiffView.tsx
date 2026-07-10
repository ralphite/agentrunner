import { useEffect, useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";
import type { DiffResp } from "../types";

interface FileDiff {
  path: string;
  lines: string[];
}

function splitDiff(diff: string): FileDiff[] {
  if (!diff.trim()) return [];
  const files: FileDiff[] = [];
  let cur: FileDiff | null = null;
  for (const line of diff.split("\n")) {
    if (line.startsWith("diff --git")) {
      const m = line.match(/ b\/(.+)$/);
      cur = { path: m ? m[1] : line, lines: [] };
      files.push(cur);
    } else if (cur) {
      cur.lines.push(line);
    }
  }
  return files;
}

function lineClass(l: string): string {
  if (l.startsWith("+") && !l.startsWith("+++")) return "add";
  if (l.startsWith("-") && !l.startsWith("---")) return "del";
  if (l.startsWith("@@")) return "hunk";
  if (l.startsWith("+++") || l.startsWith("---") || l.startsWith("index ")) return "meta";
  return "";
}

export function DiffView({ sid }: { sid: string }) {
  const { toast, openPrompt } = useStore();
  const [data, setData] = useState<DiffResp | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

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
  const stats = files.map((f) => {
    let add = 0;
    let del = 0;
    for (const l of f.lines) {
      if (l.startsWith("+") && !l.startsWith("+++")) add++;
      else if (l.startsWith("-") && !l.startsWith("---")) del++;
    }
    return { f, add, del };
  });
  const totalAdd = stats.reduce((s, x) => s + x.add, 0);
  const totalDel = stats.reduce((s, x) => s + x.del, 0);

  return (
    <div className="diffwrap">
      <div className="diffbar">
        <span className="mono dim">{data.workspace}</span>
        {!empty && (
          <span className="diff-summary">
            {files.length} file{files.length === 1 ? "" : "s"}
            {totalAdd > 0 && <span className="add"> +{totalAdd}</span>}
            {totalDel > 0 && <span className="del"> −{totalDel}</span>}
          </span>
        )}
        <span className="spacer" />
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
                + {f}
              </div>
            ))}
          </div>
        </div>
      )}
      {stats.map(({ f, add, del }) => (
        <details className="filediff" key={f.path} open>
          <summary className="fd-head mono">
            <span className="fd-path">{f.path}</span>
            <span className="fd-counts">
              {add > 0 && <span className="add">+{add}</span>}
              {del > 0 && <span className="del">−{del}</span>}
            </span>
          </summary>
          <div className="fd-body">
            {f.lines.map((l, i) => (
              <div className={"dl " + lineClass(l)} key={i}>
                {l || " "}
              </div>
            ))}
          </div>
        </details>
      ))}
    </div>
  );
}
