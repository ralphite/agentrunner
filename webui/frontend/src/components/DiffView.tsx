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
  if (!data) return <div className="diffwrap dim">载入 diff…</div>;

  if (!data.known)
    return (
      <div className="diffwrap dim">
        arwebui 不知道这个会话的 workspace(外部创建的会话不追踪)。仅 arwebui 建的
        会话可看 diff。
      </div>
    );
  if (!data.isRepo)
    return (
      <div className="diffwrap">
        <div className="dim">
          workspace <span className="mono">{data.workspace}</span> 不是 git 仓库,无 diff
          可展示。指向一个真实 git 仓库的 workspace 即可看到 Codex 式改动视图。
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
        <button className="sm" onClick={load}>
          刷新
        </button>
      </div>
      {empty && <div className="dim" style={{ padding: 12 }}>工作区无改动。</div>}
      {untracked.length > 0 && (
        <div className="filediff">
          <div className="fd-head">
            新文件 (untracked) · {untracked.length}
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
