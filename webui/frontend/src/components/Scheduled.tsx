import { CalendarDots, Plus, ArrowUpRight } from "@phosphor-icons/react";
import { useStore } from "../store";
import { friendlyStatus } from "./pill";

export function Scheduled() {
  const { runs, selectRun, openModal } = useStore();
  return (
    <div className="scheduled-page">
      <div className="page-heading">
        <div>
          <span className="page-eyebrow"><CalendarDots size={16} /> Scheduled</span>
          <h2>Background work</h2>
          <p>One-shot, goal, and repeating runs continue without keeping a task open.</p>
        </div>
        <button className="primary page-action" onClick={() => openModal({ kind: "run" })}>
          <Plus size={15} /> New run
        </button>
      </div>
      <div className="scheduled-list">
        {runs.length === 0 ? (
          <div className="empty-state">
            <CalendarDots size={28} />
            <b>No background runs</b>
            <span>Start one when work should continue on its own.</span>
          </div>
        ) : runs.map((run) => {
          const status = friendlyStatus(run.status);
          return (
            <button className="scheduled-row" key={run.id} onClick={() => selectRun(run.id)}>
              <span className={"status-dot " + status.cls} />
              <span className="scheduled-copy">
                <b>{run.label || run.id}</b>
                <span>{run.kind} · {run.workspace || "No workspace"}</span>
              </span>
              <span className="scheduled-status">{status.text}</span>
              <ArrowUpRight size={15} />
            </button>
          );
        })}
      </div>
    </div>
  );
}
