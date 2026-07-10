import {
  CaretRight,
  CheckCircle,
  Crosshair,
  Hourglass,
  UsersThree,
  WarningCircle,
  X,
} from "@phosphor-icons/react";
import type { Task } from "../types";
import { friendlyStatus } from "./pill";
import { dedupeInspectNodes } from "../viewModels";
import { Subagents, type InspectNode } from "./Subagents";

// backgroundLabel turns a raw `ar ps` row ("spawn_agent" +
// "running agent=worker task=…") into a person-readable line (W7). The
// detail is a key=value string; a missing/empty task must not render a
// dangling "task=".
export function backgroundLabel(task: Task): string {
  const detail = task.detail || "";
  const agent = /agent=([^\s]+)/.exec(detail)?.[1];
  const taskText = /task=(.*)$/.exec(detail)?.[1]?.trim();
  if (task.tool === "spawn_agent") {
    const who = agent ? `agent “${agent}”` : "a sub-agent";
    return taskText ? `${who} — ${taskText}` : `${who} is working in the background`;
  }
  return `${task.tool}${detail ? " · " + detail : ""}`;
}

export interface GoalState {
  goal: string;
  checks: number;
  max_checks?: number;
  paused?: boolean;
  verifiers?: number;
  claimed?: boolean;
}

export function SupervisionPanel({
  loading,
  goal,
  goalEdit,
  children,
  tasks,
  approvals,
  sessionIdle,
  recovery,
  onGoalEdit,
  onGoalSave,
  onGoalDiscard,
  onGoalAction,
  onOpenChild,
  onKillTask,
  onInspect,
  onClose,
}: {
  loading: boolean;
  goal: GoalState | null;
  goalEdit: string | null;
  children: InspectNode[];
  tasks: Task[];
  approvals: number;
  // The conversation itself is idle (not mid-turn): background work running
  // in that state is worth the user's attention (W35).
  sessionIdle: boolean;
  recovery: boolean;
  onGoalEdit: (value: string) => void;
  onGoalSave: () => void;
  onGoalDiscard: () => void;
  onGoalAction: (action: "pause" | "resume" | "cancel") => void;
  onOpenChild: (sid: string) => void;
  onKillTask: (handle: string) => void;
  onInspect: () => void;
  onClose: () => void;
}) {
  return (
    <aside className="supervision-panel" aria-label="Supervision">
      <div className="supervision-head">
        <div><UsersThree size={17} /> <b>Supervision</b></div>
        <button onClick={onClose} title="Hide supervision" aria-label="Hide supervision"><X size={15} /></button>
      </div>

      <section className="supervision-section">
        <div className="supervision-label"><Crosshair size={14} /> Goal</div>
        {loading ? (
          <div className="supervision-empty supervision-loading"><Hourglass size={14} className="spin" /> Checking goal…</div>
        ) : goal ? (
          <>
            {goalEdit === null ? (
              <div className="goal-copy">{goal.goal}</div>
            ) : (
              <input className="goal-input" autoFocus value={goalEdit} onChange={(event) => onGoalEdit(event.target.value)} onKeyDown={(event) => {
                if (event.key === "Enter") onGoalSave();
                if (event.key === "Escape") onGoalDiscard();
              }} />
            )}
            <div className="goal-meta">
              <span>{goal.checks}{goal.max_checks ? `/${goal.max_checks}` : ""} checks</span>
              {goal.paused && <span>Paused</span>}
              {goal.verifiers === 0 && <span>Self-certified</span>}
            </div>
            <div className="goal-actions">
              {goalEdit === null ? (
                <>
                  <button onClick={() => onGoalEdit(goal.goal)}>Edit</button>
                  <button onClick={() => onGoalAction(goal.paused ? "resume" : "pause")}>{goal.paused ? "Resume" : "Pause"}</button>
                  <button className="danger" onClick={() => onGoalAction("cancel")}>Cancel</button>
                </>
              ) : (
                <>
                  <button className="primary" onClick={onGoalSave}>Save</button>
                  <button onClick={onGoalDiscard}>Discard</button>
                </>
              )}
            </div>
          </>
        ) : (
          <div className="supervision-empty"><CheckCircle size={15} /> No active goal</div>
        )}
      </section>

      <section className="supervision-section supervision-agents">
        <div className="supervision-label"><UsersThree size={14} /> Agents</div>
        {loading ? (
          <div className="supervision-empty supervision-loading"><Hourglass size={14} className="spin" /> Checking agents…</div>
        ) : children.length > 0 ? <Subagents nodes={children} onOpen={onOpenChild} /> : <div className="supervision-empty">No subagents</div>}
      </section>

      <section className="supervision-section">
        <div className="supervision-label"><WarningCircle size={14} /> Attention</div>
        {(() => {
          // Attention is everything that deserves a human look (W35), not just
          // approvals: an agent that stopped abnormally, or background work
          // still burning tokens while the conversation itself has gone idle
          // (the abandoned-reviewer case: 195k tokens spent after "done").
          const rows: React.ReactNode[] = [];
          if (approvals > 0) {
            rows.push(
              <div className="attention-row" key="appr">
                <span className="attention-dot" /> Approval requested <b>{approvals}</b>
              </div>,
            );
          }
          if (recovery) {
            rows.push(
              <div className="attention-row" key="recovery">
                <span className="attention-dot" /> Task needs recovery
              </div>,
            );
          }
          if (!loading) {
            for (const node of dedupeInspectNodes(children)) {
              const st = friendlyStatus(node.reason || node.report?.reason || node.report?.status || "");
              if (st.cls === "crash" || st.cls === "stranded") {
                rows.push(
                  <div className="attention-row" key={"agent-" + (node.call_id || node.session)}>
                    <span className="attention-dot" /> {node.agent || "agent"} — {st.text}
                  </div>,
                );
              }
            }
            if (tasks.length > 0 && sessionIdle) {
              rows.push(
                <div className="attention-row" key="bg-idle">
                  <span className="attention-dot" /> Background work still running — it keeps
                  spending tokens; stop it below if it's no longer needed
                </div>,
              );
            }
          }
          return rows.length > 0 ? rows : (
            loading
              ? <div className="supervision-empty supervision-loading"><Hourglass size={14} className="spin" /> Checking attention…</div>
              : <div className="supervision-empty"><CheckCircle size={15} /> Nothing needs you</div>
          );
        })()}
      </section>

      {tasks.length > 0 && (
        <section className="supervision-section">
          <div className="supervision-label"><Hourglass size={14} /> Background work</div>
          {tasks.map((task) => (
            <div className="background-row" key={task.handle}>
              <span className="status-dot run" />
              <span title={task.detail || task.handle}>{backgroundLabel(task)}</span>
              <button title="Stop this background work (ar kill)" onClick={() => onKillTask(task.handle)}><X size={13} /></button>
            </div>
          ))}
        </section>
      )}

      <button className="supervision-details" onClick={onInspect} title="Open the raw inspect data (JSON) for this session's run tree">
        Inspect data (JSON) <span><CaretRight size={12} /></span>
      </button>
    </aside>
  );
}
