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
  // A parked wait on this node (G39): a child stuck on an approval carries
  // kind:"approval" + the pending ask, so parent surfaces can show and
  // answer it (the child journal is the only durable home of that ask).
  waiting?: {
    kind?: string;
    approval_id?: string;
    tool?: string;
    args?: string;
    answer_with?: string;
    question?: string;
    ask_questions?: Array<{
      question: string;
      options?: Array<{ label: string; description?: string }>;
      multi_select?: boolean;
      allow_free_text?: boolean;
    }>;
  };
  delegations?: InspectDelegation[];
}

export interface InspectDelegation {
  assigned_to?: string;
  workspace?: {
    mode?: string;
    path?: string;
  };
}

export interface ChildAnswerRequest {
  agent: string;
  session: string;
}

export function childAnswerRequests(nodes: InspectNode[]): ChildAnswerRequest[] {
  const requests: ChildAnswerRequest[] = [];
  const visit = (level: InspectNode[]) => {
    for (const node of dedupeInspectNodes(level || [])) {
      const report = node.report || node;
      if (
        node.session &&
        report.waiting?.kind === "input" &&
        (report.waiting.ask_questions?.length || 0) > 0
      ) {
        requests.push({ agent: node.agent || "agent", session: node.session });
      }
      visit(report.children || []);
    }
  };
  visit(nodes);
  return requests;
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
  const indent = depth ? ["ml-3", "ml-6", "ml-9", "ml-12"][Math.min(depth, 4) - 1] : "";
  return (
    <div className={depth ? "subagents nested contents" : "subagents"}>
      {depth === 0 && (
        <h4>
          Subagents · {uniqueNodes.length}
        </h4>
      )}
      {uniqueNodes.map((c, i) => {
        const rep = c.report || c;
        // A parked wait is more precise than the report's broad `waiting`
        // status. G39 made the approval discoverable, but the row still said
        // Ready because friendlyStatus("waiting") describes ordinary idle.
        const raw =
          rep.waiting?.kind === "input" && (rep.waiting.ask_questions?.length || 0) > 0
            ? "waiting:answer"
            : rep.waiting?.kind
              ? `waiting:${rep.waiting.kind}`
              : c.reason || rep.reason || rep.status || "";
        const st = friendlyStatus(raw);
        const tok = rep.usage?.billed ?? ((rep.usage?.input_tokens || 0) + (rep.usage?.output_tokens || 0));
        const kids = dedupeInspectNodes(rep.children || []);
        const clickable = !!c.session;
        const row = (
          <>
            {/* QA-0719 S4 二修 · identity is flex-none, not flex-1: with basis
                0 the trailing meta group claimed its full auto basis first and
                identity only got the leftover grow — the deficit landed on the
                one shrinkable child (status), clipping "Completed" to "Co…"
                while the metas' own truncation never fired. Content-sized and
                unshrinkable, identity always renders whole; sa-spacer (flex-1)
                takes the slack and the meta group is where tight rows give. */}
            <span className="flex min-w-0 flex-none items-center gap-2 max-[520px]:col-span-3">
              <span className={"sa-dot shrink-0 " + st.cls} aria-hidden="true" />
              <span className="sa-name">{c.agent || "worker"}</span>
              <span className="sa-status min-w-0 truncate max-[520px]:flex-1 max-[520px]:basis-0">{st.text}</span>
            </span>
            <span className="sa-spacer max-[520px]:hidden" />
            {/* QA-0719 S4: this trailing group was shrink-0, so with a full
                payload ("24 steps · 103k tok · open") it ate the row and the
                IDENTITY columns collapsed to "w…"/"C…" — Completed and
                Cancelled became indistinguishable. Identity outranks
                decoration: steps/tok may shrink and truncate, the open
                affordance stays whole. */}
            <span className="flex min-w-0 shrink items-center gap-2 max-[520px]:col-span-3 max-[520px]:justify-end">
              {rep.gen_steps ? <span className="sa-meta min-w-0 truncate">{rep.gen_steps} steps</span> : null}
              {tok ? <span className="sa-meta min-w-0 truncate">{tokens(tok)} tok</span> : null}
              {clickable && <span className="sa-open inline-flex shrink-0 items-center gap-1">open <ArrowSquareOut size={12} /></span>}
            </span>
          </>
        );
        return (
          <div key={c.call_id || c.session || i} className={depth ? `${indent} border-l border-line pl-2` : ""} data-depth={depth}>
            {clickable ? (
              <button className="sa-row clickable max-[520px]:grid max-[520px]:grid-cols-[auto_minmax(0,1fr)_auto] max-[520px]:gap-x-2 max-[520px]:gap-y-1" type="button" onClick={() => onOpen(c.session!)} title={c.session}>
                {row}
              </button>
            ) : (
              <div className="sa-row max-[520px]:grid max-[520px]:grid-cols-[auto_minmax(0,1fr)_auto] max-[520px]:gap-x-2 max-[520px]:gap-y-1">{row}</div>
            )}
            {kids.length > 0 && <Subagents nodes={kids} onOpen={onOpen} depth={depth + 1} />}
          </div>
        );
      })}
    </div>
  );
}
