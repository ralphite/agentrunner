import {
  ArrowSquareOut,
  CaretRight,
  CheckCircle,
  Crosshair,
  Hourglass,
  UsersThree,
  WarningCircle,
  X,
} from "@phosphor-icons/react";
import type { Task } from "../types";
import { Subagents, type InspectNode } from "./Subagents";

export interface GoalState {
  goal: string;
  checks: number;
  max_checks?: number;
  paused?: boolean;
  verifiers?: number;
  claimed?: boolean;
}

export function SupervisionPanel({
  goal,
  goalEdit,
  children,
  tasks,
  approvals,
  onGoalEdit,
  onGoalSave,
  onGoalDiscard,
  onGoalAction,
  onOpenChild,
  onKillTask,
  onInspect,
  onClose,
}: {
  goal: GoalState | null;
  goalEdit: string | null;
  children: InspectNode[];
  tasks: Task[];
  approvals: number;
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
        {goal ? (
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
        {children.length > 0 ? <Subagents nodes={children} onOpen={onOpenChild} /> : <div className="supervision-empty">No subagents</div>}
      </section>

      <section className="supervision-section">
        <div className="supervision-label"><WarningCircle size={14} /> Attention</div>
        {approvals > 0 ? (
          <div className="attention-row"><span className="attention-dot" /> Approval requested <b>{approvals}</b></div>
        ) : (
          <div className="supervision-empty"><CheckCircle size={15} /> Nothing needs you</div>
        )}
      </section>

      {tasks.length > 0 && (
        <section className="supervision-section">
          <div className="supervision-label"><Hourglass size={14} /> Background work</div>
          {tasks.map((task) => (
            <div className="background-row" key={task.handle}>
              <span className="status-dot run" />
              <span title={task.detail || task.handle}>{task.tool} · {task.detail || task.handle}</span>
              <button title="Cancel background work" onClick={() => onKillTask(task.handle)}><X size={13} /></button>
            </div>
          ))}
        </section>
      )}

      <button className="supervision-details" onClick={onInspect}>
        View run details <span><ArrowSquareOut size={14} /><CaretRight size={12} /></span>
      </button>
    </aside>
  );
}
