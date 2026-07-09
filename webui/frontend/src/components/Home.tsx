import { useStore } from "../store";
import { Composer } from "./Composer";
import { friendlyStatus } from "./pill";
import { relTime, sessionDate } from "../time";
import { displayTitle } from "../title";

// Home mirrors Codex's landing: a heading, one Codex-style composer card, and a
// grid of task cards (titled by their opening message, like Codex).
export function Home() {
  const { sessions, runs, select, selectRun, toast, renames } = useStore();

  const cards = [
    ...sessions.map((s) => ({
      key: "s" + s.id,
      title: displayTitle(renames, s.id, s.title),
      time: relTime(sessionDate(s.id)),
      cls: friendlyStatus(s.status).cls,
      badge: friendlyStatus(s.status).text,
      onClick: () => select(s.id),
    })),
    ...runs.map((r) => ({
      key: "r" + r.id,
      title: r.label || r.id,
      time: r.kind,
      cls: friendlyStatus(r.status).cls,
      badge: friendlyStatus(r.status).text,
      onClick: () => selectRun(r.id),
    })),
  ];

  return (
    <div className="home">
      <div className="hero">
        <h2>What should we work on?</h2>
        <Composer variant="home" onError={(m) => toast(m)} />
      </div>

      <div className="tasklist">
        <div className="grp-label">Tasks · {cards.length}</div>
        {cards.length === 0 ? (
          <div className="dim">No tasks yet. Start one with the composer above.</div>
        ) : (
          <div className="task-grid">
            {cards.map((c) => (
              <div className="task-card" key={c.key} onClick={c.onClick}>
                <div className="tc-title">{c.title}</div>
                <div className="tc-sub">
                  <span className={"nr-dot " + c.cls} />
                  <span className="dim">{c.badge}</span>
                  <span className="spacer" />
                  <span className="dim">{c.time}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
