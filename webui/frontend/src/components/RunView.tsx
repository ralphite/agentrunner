import { useEffect, useMemo, useRef, useState } from "react";
import { useAppServices } from "../app/appServices";
import { useStore } from "../store";
import {
  RunHeader,
  RunLogEmptyState,
  RunLogItem,
  type RunLogLine,
} from "./RunParts";

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

function summarize(raw: string): RunLogLine {
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
    const line: RunLogLine = { raw, kind, text };
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
  const { api, clock, streams } = useAppServices();
  const { runs, toast, refreshRuns } = useStore();
  const run = runs.find((r) => r.id === runId);
  const [lines, setLines] = useState<string[]>([]);
  const ref = useRef<HTMLDivElement>(null);
  const stick = useRef(true);

  useEffect(() => {
    setLines([]);
    const es = streams.open(`/api/runs/${runId}/stream`);
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
      await api.stopRun(runId);
      toast("stop requested", "info");
      clock.setTimeout(refreshRuns, 800);
    } catch (e: any) {
      toast(e.message);
    }
  };

  const title = run?.label || runId;

  return (
    <>
      <RunHeader
        title={title}
        status={run?.status}
        kind={run?.kind}
        onStop={run?.status === "running" ? stop : undefined}
      />
      <div
        className="runlog min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-3 py-3 font-mono text-[12px] leading-[1.5]"
        ref={ref}
        role="log"
        aria-label="Run output"
        onScroll={() => {
          const el = ref.current;
          if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        }}
      >
        <div className="mx-auto w-full max-w-[760px]">
          {parsed.length === 0 && <RunLogEmptyState />}
          {parsed.map((l, i) => (
            <RunLogItem key={i} line={l} />
          ))}
        </div>
      </div>
    </>
  );
}
