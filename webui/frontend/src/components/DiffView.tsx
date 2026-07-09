import { useEffect, useState } from "react";
import { AR } from "../api";
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
  const [data, setData] = useState<DiffResp | null>(null);
  const [err, setErr] = useState("");

  const load = () => {
    AR.diff(sid)
      .then((d) => {
        setData(d);
        setErr("");
      })
      .catch((e) => setErr(e.message));
  };
  useEffect(load, [sid]);

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

  const files = splitDiff(data.diff);
  const empty = files.length === 0 && data.untracked.length === 0;

  return (
    <div className="diffwrap">
      <div className="diffbar">
        <span className="mono dim">{data.workspace}</span>
        <span className="spacer" />
        <button className="sm" onClick={load}>
          Refresh
        </button>
      </div>
      {empty && <div className="dim" style={{ padding: 12 }}>No changes in the workspace.</div>}
      {data.untracked.length > 0 && (
        <div className="filediff">
          <div className="fd-head">
            new files (untracked) · {data.untracked.length}
          </div>
          <div className="fd-body">
            {data.untracked.map((f) => (
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
