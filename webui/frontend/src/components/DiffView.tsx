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
  const { toast } = useStore();
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
  const commit = async () => {
    const message = window.prompt("Commit message for these changes:", "changes from agent session");
    if (message === null) return;
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

  if (err) return <div className="diffwrap"><div className="chip bad">{err}</div></div>;
  if (!data) return <div className="diffwrap dim">loading diff…</div>;

  if (!data.known)
    return (
      <div className="diffwrap dim">
        arwebui doesn't know this session's workspace (sessions created outside arwebui
        aren't tracked), so there is no diff to show. Only sessions started here have one.
      </div>
    );
  if (!data.isRepo)
    return (
      <div className="diffwrap">
        <div className="dim">
          workspace <span className="mono">{data.workspace}</span> is not a git repository,
          so there is no diff to show. Point the session at a real git repo to get the diff view.
        </div>
      </div>
    );

  const files = splitDiff(data.diff || "");
  const untracked = data.untracked || [];
  const empty = files.length === 0 && untracked.length === 0;

  return (
    <div className="diffwrap">
      <div className="diffbar">
        <span className="mono dim">{data.workspace}</span>
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
      {files.map((f) => (
        <div className="filediff" key={f.path}>
          <div className="fd-head mono">{f.path}</div>
          <div className="fd-body">
            {f.lines.map((l, i) => (
              <div className={"dl " + lineClass(l)} key={i}>
                {l || " "}
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
