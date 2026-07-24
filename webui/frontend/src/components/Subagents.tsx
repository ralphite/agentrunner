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
  return (
    <div className={depth ? "subagents nested contents" : "subagents"}>
      {depth === 0 && (
        <h4>
          Subagents · {uniqueNodes.length}
        </h4>
      )}
      {uniqueNodes.map((node, index) => (
        <SubagentItem
          key={node.call_id || node.session || index}
          node={node}
          onOpen={onOpen}
          depth={depth}
        />
      ))}
    </div>
  );
}

export function SubagentItem({
  node,
  onOpen,
  depth = 0,
}: {
  node: InspectNode;
  onOpen: (sid: string) => void;
  depth?: number;
}) {
  const report = node.report || node;
  const raw =
    report.waiting?.kind === "input" &&
    (report.waiting.ask_questions?.length || 0) > 0
      ? "waiting:answer"
      : report.waiting?.kind
        ? `waiting:${report.waiting.kind}`
        : node.reason || report.reason || report.status || "";
  const status = friendlyStatus(raw);
  const tokenCount =
    report.usage?.billed ??
    ((report.usage?.input_tokens || 0) + (report.usage?.output_tokens || 0));
  const children = dedupeInspectNodes(report.children || []);
  const clickable = !!node.session;
  const indent = depth
    ? ["ml-3", "ml-6", "ml-9", "ml-12"][Math.min(depth, 4) - 1]
    : "";
  const row = (
    <>
      <span className="flex min-w-0 flex-none items-center gap-2 max-[520px]:col-span-3">
        <span className={`sa-dot shrink-0 ${status.cls}`} aria-hidden="true" />
        <span className="sa-name">{node.agent || "worker"}</span>
        <span className="sa-status min-w-0 truncate max-[520px]:flex-1 max-[520px]:basis-0">
          {status.text}
        </span>
      </span>
      <span className="sa-spacer max-[520px]:hidden" />
      <span className="flex min-w-0 shrink items-center gap-2 max-[520px]:col-span-3 max-[520px]:justify-end">
        {report.gen_steps ? (
          <span className="sa-meta min-w-0 truncate">
            {report.gen_steps} steps
          </span>
        ) : null}
        {tokenCount ? (
          <span className="sa-meta min-w-0 truncate">
            {tokens(tokenCount)} tok
          </span>
        ) : null}
        {clickable && (
          <span className="sa-open inline-flex shrink-0 items-center gap-1">
            open <ArrowSquareOut size={12} />
          </span>
        )}
      </span>
    </>
  );
  return (
    <div
      className={depth ? `${indent} border-l border-line pl-2` : ""}
      data-depth={depth}
    >
      {clickable ? (
        <button
          className="sa-row clickable max-[520px]:grid max-[520px]:grid-cols-[auto_minmax(0,1fr)_auto] max-[520px]:gap-x-2 max-[520px]:gap-y-1"
          type="button"
          onClick={() => onOpen(node.session!)}
          title={node.session}
        >
          {row}
        </button>
      ) : (
        <div className="sa-row max-[520px]:grid max-[520px]:grid-cols-[auto_minmax(0,1fr)_auto] max-[520px]:gap-x-2 max-[520px]:gap-y-1">
          {row}
        </div>
      )}
      {children.length > 0 && (
        <Subagents nodes={children} onOpen={onOpen} depth={depth + 1} />
      )}
    </div>
  );
}
