import { CalendarDots, Plus, ArrowUpRight } from "@phosphor-icons/react";
import { useStore } from "../store";
import { friendlyStatus } from "./pill";
import { projectLabel, scheduleLabel } from "../viewModels";

export function Scheduled() {
  const { runs, sessions, select, selectRun, openModal } = useStore();
  const activeRuns = runs.filter((run) => run.status === "running" && run.kind === "submit");
  const drivers = sessions.filter((session) => session.kind === "driver");
  const empty = activeRuns.length === 0 && drivers.length === 0;
  return (
    <div className="scheduled-page">
      <div className="page-heading">
        <div>
          <span className="page-eyebrow"><CalendarDots size={16} /> Scheduled</span>
          <h2>Scheduled work</h2>
          <p>Goals and repeating tasks stay visible here, including after a restart.</p>
        </div>
        <button className="primary page-action" onClick={() => openModal({ kind: "run" })}>
          <Plus size={15} /> New schedule
        </button>
      </div>
      <div className="scheduled-list">
        {empty ? (
          <div className="empty-state">
            <CalendarDots size={28} />
            <b>No scheduled work</b>
            <span>Start a goal or repeating task when work should continue on its own.</span>
          </div>
        ) : (
          <>
            {activeRuns.length > 0 && <div className="scheduled-section-title">Running now</div>}
            {activeRuns.map((run) => {
              const status = friendlyStatus(run.status);
              return (
                <button className="scheduled-row" key={run.id} onClick={() => selectRun(run.id)}>
                  <span className={"status-dot " + status.cls} />
                  <span className="scheduled-copy"><b>{run.label || run.id}</b><span>One-time · {projectLabel(run.workspace)}</span></span>
                  <span className="scheduled-status">{status.text}</span><ArrowUpRight size={15} />
                </button>
              );
            })}
            {drivers.length > 0 && <div className="scheduled-section-title">Goals & schedules</div>}
            {drivers.map((session) => {
              const status = friendlyStatus(session.status);
              return (
                <button className="scheduled-row" key={session.id} onClick={() => select(session.id)}>
                  <span className={"status-dot " + status.cls} />
                  <span className="scheduled-copy"><b>{session.title || session.id}</b><span>{scheduleLabel(session.schedule)} · {projectLabel(session.workspace)}</span></span>
                  <span className="scheduled-status">{status.text}</span><ArrowUpRight size={15} />
                </button>
              );
            })}
          </>
        )}
      </div>
    </div>
  );
}
