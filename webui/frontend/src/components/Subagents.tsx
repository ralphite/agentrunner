import { friendlyStatus } from "./pill";
import { dedupeInspectNodes } from "../viewModels";
import { ArrowSquareOut } from "@phosphor-icons/react";
import {
  LifecycleStatus,
  lifecycleStateFromStatusClass,
} from "../ui/LifecycleStatus";

// One node of the `ar inspect --json` tree. `report` is the child's own inspect
// subtree (recursive), so a subagent carries its status, gen_steps, usage, and
// its own children.
export interface InspectNode {
  call_id?: string;
  agent?: string;
  // Newer inspect projections may surface a user-facing task identity on the
  // child itself. Older journals keep it on the matching delegation instead.
  task?: string;
  title?: string;
  name?: string;
  description?: string;
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
  call_id?: string;
  description?: string;
  task?: string;
  title?: string;
  name?: string;
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

// A child opening prompt may carry an implementation-only workspace preamble.
// The actual delegated task follows the blank line; never expose the preamble,
// a worktree path, or the generated child session id as the person's identity.
export function subagentTaskLabel(value?: string): string | undefined {
  let clean = value?.trim();
  if (!clean) return undefined;
  if (clean.startsWith("[workspace note]")) {
    const taskStart = clean.indexOf("\n\n");
    clean = taskStart >= 0 ? clean.slice(taskStart + 2).trim() : "";
  }
  // Preserve the entire delegated task. The visual label clamps to two lines,
  // while its title/accessible name retain later sentences that may be the
  // only part distinguishing several otherwise identical `worker` agents.
  return clean.replace(/\s+/g, " ") || undefined;
}

export function subagentPrimaryIdentity(
  node: InspectNode,
  delegation?: InspectDelegation,
): string {
  const task =
    subagentTaskLabel(delegation?.title) ||
    subagentTaskLabel(delegation?.task) ||
    subagentTaskLabel(delegation?.name) ||
    subagentTaskLabel(delegation?.description) ||
    subagentTaskLabel(node.title) ||
    subagentTaskLabel(node.task) ||
    subagentTaskLabel(node.name) ||
    subagentTaskLabel(node.description);
  return task || node.agent?.trim() || "worker";
}

// Subagents mirrors Codex's Subagents panel: a session's spawned children, each
// with its status + token usage and a click that opens its (read-only) session.
// Recurses so a subagent that itself spawned workers shows them nested.
export function Subagents({
  nodes,
  delegations = [],
  onOpen,
  depth = 0,
}: {
  nodes: InspectNode[];
  delegations?: InspectDelegation[];
  onOpen: (sid: string) => void;
  depth?: number;
}) {
  if (!nodes?.length) return null;
  const uniqueNodes = dedupeInspectNodes(nodes);
  return (
    <div className={depth ? "subagents nested contents" : "subagents"}>
      {depth === 0 && (
        <h4>
          Subagents · {uniqueNodes.length}
        </h4>
      )}
      {uniqueNodes.map((node, index) => {
        const delegation = delegations.find(
          (item) =>
            (!!item.assigned_to && item.assigned_to === node.session) ||
            (!!item.call_id && item.call_id === node.call_id),
        );
        return (
          <SubagentItem
            key={node.call_id || node.session || index}
            node={node}
            delegation={delegation}
            onOpen={onOpen}
            depth={depth}
          />
        );
      })}
    </div>
  );
}

export function SubagentItem({
  node,
  delegation,
  onOpen,
  depth = 0,
}: {
  node: InspectNode;
  delegation?: InspectDelegation;
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
  const identity = subagentPrimaryIdentity(node, delegation);
  const role = node.agent?.trim();
  const secondary = [
    role && role !== identity ? role : "",
    status.text,
    report.gen_steps ? `${report.gen_steps} steps` : "",
    tokenCount ? `${tokens(tokenCount)} tok` : "",
  ]
    .filter(Boolean)
    .join(" · ");
  const indent = depth
    ? ["ml-3", "ml-6", "ml-9", "ml-12"][Math.min(depth, 4) - 1]
    : "";
  const row = (
    <>
      <span className="flex min-w-0 flex-1 items-start gap-2">
        <LifecycleStatus
          accessibleLabel={status.text}
          className={`sa-dot mt-[5px] shrink-0 ${status.cls}`}
          state={lifecycleStateFromStatusClass(status.cls)}
          aria-hidden="true"
        />
        <span className="min-w-0 flex-1">
          <span className="sa-name line-clamp-2 block !max-w-none !flex-1 whitespace-normal text-left font-medium leading-4">
            {identity}
          </span>
          <span className="sa-status block min-w-0 whitespace-normal text-left text-[12px] leading-4 text-dim">
            {secondary}
          </span>
        </span>
      </span>
      <span className="flex min-w-0 shrink-0 items-center gap-2">
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
          className="sa-row clickable"
          type="button"
          onClick={() => onOpen(node.session!)}
          title={`${identity} · ${secondary}`}
        >
          {row}
        </button>
      ) : (
        <div className="sa-row">
          {row}
        </div>
      )}
      {children.length > 0 && (
        <Subagents
          nodes={children}
          delegations={report.delegations || []}
          onOpen={onOpen}
          depth={depth + 1}
        />
      )}
    </div>
  );
}
