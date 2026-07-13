import { Archive, ArrowUpRight, Folder, Tray } from "@phosphor-icons/react";
import { useStore } from "../store";
import { buildArchivedModel } from "../viewModels";
import { displayTitle } from "../title";
import { friendlyStatus } from "./pill";

export function SettingsArchived({ query, onClose }: { query: string; onClose: () => void }) {
  const { sessions, archived, toggleArchive, renames, select } = useStore();
  const model = buildArchivedModel(sessions, archived, query, (session) => displayTitle(renames, session.id, session.title));
  const total = model.projects.reduce((count, project) => count + project.sessions.length, 0);

  const openSession = (sid: string) => {
    select(sid);
    onClose();
  };

  return (
    <div className="rs-panel rs-archived">
      <h2 className="rs-panel-title">Archived sessions</h2>
      <p className="rs-panel-sub">Review sessions hidden from the sidebar and restore the ones you still need.</p>
      {archived.length === 0 ? (
        <div className="rs-archive-empty"><Tray size={26} /><b>No archived sessions</b><span>Archived conversations will appear here.</span></div>
      ) : total === 0 ? (
        <div className="rs-archive-empty"><Archive size={26} /><b>No matches</b><span>No archived session matches “{query}”.</span></div>
      ) : (
        <div className="rs-archive-groups">
          {model.projects.map((project) => (
            <section className="rs-archive-group" key={project.key}>
              <header>
                <Folder size={15} />
                <b>{project.label}</b>
                {project.hint && <span>{project.hint}</span>}
                <small>{project.sessions.length} session{project.sessions.length === 1 ? "" : "s"}</small>
              </header>
              {project.sessions.map((session) => {
                const status = friendlyStatus(session.status);
                return (
                  <div className="rs-archive-row" key={session.id}>
                    <button className="rs-archive-open" onClick={() => openSession(session.id)} title="Open archived session">
                      <span className="rs-archive-title">{displayTitle(renames, session.id, session.title)}</span>
                      <span className={`rs-archive-status ${status.cls}`}>{status.text}</span>
                      <ArrowUpRight size={14} />
                    </button>
                    <button className="rs-archive-restore" onClick={() => toggleArchive(session.id)}>Unarchive</button>
                  </div>
                );
              })}
            </section>
          ))}
        </div>
      )}
      {archived.length > 0 && <p className="rs-archive-note">Permanent session deletion is not available yet.</p>}
    </div>
  );
}
