import { useEffect, useMemo, useRef, useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";

interface Line {
  raw: string;
  kind: string;
  text: string;
}

function summarize(raw: string): Line {
  try {
    const o = JSON.parse(raw);
    const kind = o.kind || o.type || "event";
    let text = "";
    if (o.text) text = o.text;
    else if (o.message?.parts) text = o.message.parts.map((p: any) => p.text || "").join("");
    else if (o.status) text = o.status;
    else text = JSON.stringify(o);
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
      toast("已请求停止", "info");
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
              停止
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
        {parsed.length === 0 && <div className="dim">等待输出…</div>}
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
