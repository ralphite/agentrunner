import { useEffect, useMemo, useRef, useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";

interface Line {
  raw: string;
  kind: string;
  text: string;
}

function argVal(a: any, ...keys: string[]): string {
  let o = a;
  if (typeof a === "string") {
    try {
      o = JSON.parse(a);
    } catch {
      return a;
    }
  }
  o = o || {};
  for (const k of keys) if (o[k]) return o[k];
  return "";
}

function summarize(raw: string): Line {
  try {
    const o = JSON.parse(raw);
    const kind = o.kind || o.type || "event";
    let text = "";
    switch (kind) {
      case "session_start":
        text = "session " + (o.session || "");
        break;
      case "generation_start":
        text = "turn " + (o.n ?? "?");
        break;
      case "tool_call":
        text =
          o.tool === "bash"
            ? "$ " + argVal(o.args, "command")
            : `${o.tool} ${argVal(o.args, "path", "file", "command")}`.trim();
        break;
      case "tool_result": {
        const r = typeof o.result === "string" ? o.result : JSON.stringify(o.result || {});
        text = "→ " + (o.tool || "") + " " + r.slice(0, 200);
        break;
      }
      case "message":
        text = o.text || (o.message?.parts || []).map((p: any) => p.text || "").join("");
        break;
      case "text_delta":
        text = o.text || "";
        break;
      case "end":
        text = "end · " + (o.status || o.reason || "");
        break;
      default:
        text = o.text || o.status || JSON.stringify(o);
    }
    return { raw, kind, text };
  } catch {
    return { raw, kind: "", text: raw };
  }
}

export function RunView({ runId }: { runId: string }) {
  const { runs, toast, refreshRuns } = useStore();
  const run = runs.find((r) => r.id === runId);
  const [lines, setLines] = useState<string[]>([]);
  const ref = useRef<HTMLDivElement>(null);
  const stick = useRef(true);

  useEffect(() => {
    setLines([]);
    const es = new EventSource(`/api/runs/${runId}/stream`);
    es.onmessage = (m) => setLines((p) => [...p, m.data]);
    // Close on the server's end event; otherwise EventSource auto-reconnects
    // and re-replays the whole backlog, duplicating the log without bound.
    es.addEventListener("end", () => es.close());
    return () => es.close();
  }, [runId]);

  useEffect(() => {
    const el = ref.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  });

  const parsed = useMemo(() => lines.map(summarize), [lines]);

  const stop = async () => {
    try {
      await AR.stopRun(runId);
      toast("stop requested", "info");
      setTimeout(refreshRuns, 800);
    } catch (e: any) {
      toast(e.message);
    }
  };

  return (
    <>
      <div className="topbar">
        <span className="sid">{run?.label || runId}</span>
        <span className={"pill " + (run?.status || "")}>{run?.status || "—"}</span>
        <span className="readonly-tag">{run?.kind} run</span>
        <span className="spacer" />
        <div className="actions">
          {run?.status === "running" && (
            <button className="sm danger" onClick={stop}>
              Stop
            </button>
          )}
        </div>
      </div>
      <div
        className="runlog"
        ref={ref}
        onScroll={() => {
          const el = ref.current;
          if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        }}
      >
        {parsed.length === 0 && <div className="dim">waiting for output…</div>}
        {parsed.map((l, i) => (
          <div className="runline" key={i}>
            <span className="rk">{l.kind}</span>
            <span className="rt">{l.text}</span>
          </div>
        ))}
      </div>
    </>
  );
}
