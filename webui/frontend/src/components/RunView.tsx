import { useEffect, useMemo, useRef, useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";

interface Line {
  raw: string;
  kind: string;
  text: string;
  // set on a drive run's child session_start: this line opens iteration N
  iter?: number;
  // set when the line is the driver's terminal verdict
  verdict?: { reason: string; n: number; ok: boolean };
}

// The driver's own verdict rides stderr as "driver <reason>: <n> iterations
// (best <i>)" (internal/cli/drive.go); a run_end may carry the same reasons.
const DRIVER_TERMINAL = ["satisfied", "stalled", "max_iterations", "child_failed", "budget", "budget_exhausted"];
const VERDICT_RE = /driver (\w+): (\d+) iterations?/;

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
    const line: Line = { raw, kind, text };
    if (kind === "run_end" && DRIVER_TERMINAL.includes(o.reason)) {
      line.verdict = { reason: o.reason, n: o.n ?? 0, ok: o.reason === "satisfied" };
    }
    return line;
  } catch {
    // Non-JSON = forwarded stderr; the driver's terminal verdict lives here.
    const m = raw.match(VERDICT_RE);
    if (m) return { raw, kind: "driver", text: raw, verdict: { reason: m[1], n: parseInt(m[2], 10), ok: m[1] === "satisfied" } };
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

  // A drive run's stream is each CHILD run's events in sequence — a fresh
  // child session_start opens the next iteration, so number them as dividers.
  const parsed = useMemo(() => {
    let iter = 0;
    return lines.map((raw) => {
      const l = summarize(raw);
      if (run?.kind === "drive" && l.kind === "session_start") l.iter = ++iter;
      return l;
    });
  }, [lines, run?.kind]);

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
      <div className="topbar flex flex-wrap items-center gap-[10px] px-[18px] py-[11px] border-b border-line bg-panel pointer-coarse:pt-[max(10px,env(safe-area-inset-top))]">
        <span className="sid font-mono text-[12.5px] font-semibold">{run?.label || runId}</span>
        <span className={"pill " + (run?.status || "")}>{run?.status || "—"}</span>
        <span className="readonly-tag text-[11px] text-dim border border-line rounded-md px-[7px] py-px">{run?.kind} run</span>
        <span className="spacer flex-1" />
        <div className="actions flex flex-wrap gap-[6px]">
          {run?.status === "running" && (
            <button className="sm danger" onClick={stop}>
              Stop
            </button>
          )}
        </div>
      </div>
      <div
        className="runlog flex-1 overflow-y-auto px-[22px] py-4 font-mono text-[12.5px]"
        ref={ref}
        onScroll={() => {
          const el = ref.current;
          if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        }}
      >
        {parsed.length === 0 && <div className="dim">waiting for output…</div>}
        {parsed.map((l, i) => (
          <div key={i}>
            {l.iter !== undefined && (
              <div className="run-iter flex items-center gap-[10px] mt-[14px] mb-[6px] text-dim text-[11px] before:h-px before:flex-1 before:bg-line after:h-px after:flex-1 after:bg-line">
                iteration {l.iter}
              </div>
            )}
            {l.verdict ? (
              <div
                className={
                  "run-verdict my-[10px] px-3 py-[6px] rounded-lg font-semibold" +
                  (l.verdict.ok ? " ok bg-green-soft text-green" : " warn bg-amber-soft text-amber")
                }
              >
                ■ driver {l.verdict.reason} · {l.verdict.n} iteration{l.verdict.n === 1 ? "" : "s"}
              </div>
            ) : (
              <div className="runline flex gap-[10px] py-[2px] border-b border-line-2">
                <span className="rk shrink-0 min-w-[120px] text-dim">{l.kind}</span>
                <span className="rt whitespace-pre-wrap wrap-break-word">{l.text}</span>
              </div>
            )}
          </div>
        ))}
      </div>
    </>
  );
}
