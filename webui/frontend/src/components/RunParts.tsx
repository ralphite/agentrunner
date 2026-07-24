import { Stop as StopIcon } from "@phosphor-icons/react";
import { friendlyStatus } from "./pill";
import {
  StatusIndicator,
  type StatusIndicatorTone,
} from "../ui/StatusIndicator";

export interface RunLogLine {
  raw: string;
  kind: string;
  text: string;
  iter?: number;
  verdict?: { reason: string; n: number; ok: boolean };
}

interface RunHeaderProps {
  title: string;
  status?: string;
  kind?: string;
  onStop?: () => void;
}

function runStatusTone(status: string): StatusIndicatorTone {
  const cls = friendlyStatus(status).cls;
  if (cls === "run") return "success";
  if (cls === "idle") return "info";
  if (cls === "appr" || cls === "stranded") return "warning";
  if (cls === "crash") return "danger";
  return "neutral";
}

export function RunHeader({
  title,
  status,
  kind,
  onStop,
}: RunHeaderProps) {
  return (
    <div className="topbar min-w-0 overflow-hidden">
      <span
        className="run-topbar-nav-slot hidden h-9 w-9 shrink-0 max-[900px]:block"
        aria-hidden="true"
      />
      <span
        className="sid min-w-0 flex-1 truncate text-[13px] font-medium"
        title={title}
      >
        {title}
      </span>
      {status && (
        <StatusIndicator
          className={`pill shrink-0 whitespace-nowrap ${status}`}
          display="pill"
          label={status}
          tone={runStatusTone(status)}
        />
      )}
      {kind && (
        <span className="readonly-tag !ml-0 shrink-0 whitespace-nowrap">
          {kind} run
        </span>
      )}
      <div className="actions shrink-0">
        {status === "running" && onStop && (
          <button
            type="button"
            className="topbar-tool stop"
            onClick={onStop}
            title="Stop run"
            aria-label="Stop run"
          >
            <StopIcon size={14} weight="fill" />
            <span className="topbar-tool-label">Stop</span>
          </button>
        )}
      </div>
    </div>
  );
}

export function RunLogItem({ line }: { line: RunLogLine }) {
  return (
    <div className="border-b border-line-2 last:border-b-0">
      {line.iter !== undefined && (
        <div className="run-iter py-2 text-[11px] font-medium text-dim">
          iteration {line.iter}
        </div>
      )}
      {line.verdict ? (
        <div
          className={`run-verdict py-2 font-medium${
            line.verdict.ok ? " ok text-green" : " warn text-amber"
          }`}
        >
          ■ series {line.verdict.reason} · {line.verdict.n} iteration
          {line.verdict.n === 1 ? "" : "s"}
        </div>
      ) : (
        <div className="runline grid min-w-0 grid-cols-[104px_minmax(0,1fr)] gap-3 py-[5px]">
          <span
            className="rk min-w-0 truncate text-[10px] text-dim"
            title={line.kind || "output"}
          >
            {line.kind || "output"}
          </span>
          <span className="rt min-w-0 whitespace-pre-wrap break-words text-ink-2">
            {line.text}
          </span>
        </div>
      )}
    </div>
  );
}

export function RunLogEmptyState() {
  return <div className="dim py-1">waiting for output…</div>;
}
