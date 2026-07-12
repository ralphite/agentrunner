import { friendlyStatus } from "./pill";
import { dedupeInspectNodes } from "../viewModels";
import { ArrowSquareOut } from "@phosphor-icons/react";

// One node of the `ar inspect --json` tree. `report` is the child's own inspect
// subtree (recursive), so a subagent carries its status, gen_steps, usage, and
// its own children.
export interface InspectNode {
  call_id?: string;
  agent?: string;
  session?: string;
  reason?: string;
  status?: string;
  gen_steps?: number;
  usage?: { input_tokens?: number; output_tokens?: number; billed?: number };
  report?: InspectNode;
  children?: InspectNode[];
}

function tokens(n?: number): string {
  if (!n) return "";
  if (n < 1000) return String(n);
  if (n < 1_000_000) return (n / 1000).toFixed(n < 10_000 ? 1 : 0) + "k";
  return (n / 1_000_000).toFixed(1) + "M";
}

// Status-dot tone per friendlyStatus cls (was .sa-dot.<cls> in styles.css).
// `run` pulses via the nr-pulse keyframes (leftover CSS); `idle` is a hollow
// ring (waiting-for-you reads as a quiet blue ring, distinct from activity).
const SA_DOT_TONE: Record<string, string> = {
  run: "bg-status-running animate-[nr-pulse_1.5s_ease-in-out_infinite]",
  appr: "bg-status-attention",
  closed: "bg-status-terminal",
  crash: "bg-status-failed",
  stranded: "bg-status-attention",
  idle: "bg-transparent border-[1.5px] border-status-ready",
};

// Row shell shared by the clickable and static variants. Subagents only ever
// renders inside SupervisionPanel's Agents section, so the panel-context
// overrides (26px rows, 12.5px, 4px/1px padding) are baked in directly.
const SA_ROW =
  "flex items-center gap-2 w-full min-h-[26px] py-1 px-px border-0 rounded-lg bg-transparent text-inherit text-left text-[12.5px]";

// Subagents mirrors Codex's Subagents panel: a session's spawned children, each
// with its status + token usage and a click that opens its (read-only) session.
// Recurses so a subagent that itself spawned workers shows them nested.
export function Subagents({ nodes, onOpen, depth = 0 }: { nodes: InspectNode[]; onOpen: (sid: string) => void; depth?: number }) {
  if (!nodes?.length) return null;
  const uniqueNodes = dedupeInspectNodes(nodes);
  return (
    <div className={"subagents" + (depth ? " nested ml-4 border-l border-line-2 pl-1" : "")}>
      {depth === 0 && (
        // Hidden inside the Supervision panel (its own "Agents" label heads the
        // section) — the panel is this component's only consumer.
        <h4 className="hidden">
          Subagents · {uniqueNodes.length}
        </h4>
      )}
      {uniqueNodes.map((c, i) => {
        const rep = c.report || c;
        const raw = c.reason || rep.reason || rep.status || "";
        const st = friendlyStatus(raw);
        const tok = rep.usage?.billed ?? ((rep.usage?.input_tokens || 0) + (rep.usage?.output_tokens || 0));
        const kids = dedupeInspectNodes(rep.children || []);
        const clickable = !!c.session;
        const row = (
          <>
            <span className={`sa-dot ${st.cls} w-[7px] h-[7px] rounded-full shrink-0 ${SA_DOT_TONE[st.cls] ?? "bg-dim"}`} />
            <span className="sa-name font-medium text-ink min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">{c.agent || "worker"}</span>
            <span className="sa-status text-xs text-dim">{st.text}</span>
            <span className="sa-spacer flex-1" />
            {rep.gen_steps ? <span className="sa-meta text-[11.5px] text-dim tabular-nums">{rep.gen_steps} steps</span> : null}
            {tok ? <span className="sa-meta text-[11.5px] text-dim tabular-nums">{tokens(tok)} tok</span> : null}
            {clickable && <span className="sa-open hidden">open <ArrowSquareOut size={12} /></span>}
          </>
        );
        return (
          <div key={c.call_id || c.session || i}>
            {clickable ? (
              <button className={`sa-row clickable ${SA_ROW} cursor-pointer hover:bg-panel-2`} type="button" onClick={() => onOpen(c.session!)} title={c.session}>
                {row}
              </button>
            ) : (
              <div className={`sa-row ${SA_ROW}`}>{row}</div>
            )}
            {kids.length > 0 && <Subagents nodes={kids} onOpen={onOpen} depth={depth + 1} />}
          </div>
        );
      })}
    </div>
  );
}
