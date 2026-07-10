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

// Subagents mirrors Codex's Subagents panel: a session's spawned children, each
// with its status + token usage and a click that opens its (read-only) session.
// Recurses so a subagent that itself spawned workers shows them nested.
export function Subagents({ nodes, onOpen, depth = 0 }: { nodes: InspectNode[]; onOpen: (sid: string) => void; depth?: number }) {
  if (!nodes?.length) return null;
  const uniqueNodes = dedupeInspectNodes(nodes);
  return (
    <div className={"subagents" + (depth ? " nested" : "")}>
      {depth === 0 && (
        <h4>
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
            <span className={"sa-dot " + st.cls} />
            <span className="sa-name">{c.agent || "worker"}</span>
            <span className="sa-status">{st.text}</span>
            <span className="sa-spacer" />
            {rep.gen_steps ? <span className="sa-meta">{rep.gen_steps} steps</span> : null}
            {tok ? <span className="sa-meta">{tokens(tok)} tok</span> : null}
            {clickable && <span className="sa-open">open <ArrowSquareOut size={12} /></span>}
          </>
        );
        return (
          <div key={c.call_id || c.session || i}>
            {clickable ? (
              <button className="sa-row clickable" type="button" onClick={() => onOpen(c.session!)} title={c.session}>
                {row}
              </button>
            ) : (
              <div className="sa-row">{row}</div>
            )}
            {kids.length > 0 && <Subagents nodes={kids} onOpen={onOpen} depth={depth + 1} />}
          </div>
        );
      })}
    </div>
  );
}
