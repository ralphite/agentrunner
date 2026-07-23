import { Archive, ArrowUpRight, Folder, Tray } from "@phosphor-icons/react";
import { useStore } from "../store";
import { buildArchivedModel } from "../viewModels";
import { displayTitle } from "../title";
import { sessionFriendlyStatus } from "./pill";

export function SettingsArchived({ query, onClose }: { query: string; onClose: () => void }) {
  const { sessions, archived, toggleArchive, renames, select } = useStore();
  const model = buildArchivedModel(sessions, archived, query, (session) => displayTitle(renames, session.id, session.title));
  const total = model.projects.reduce((count, project) => count + project.sessions.length, 0);

  const openSession = (sid: string) => {
    select(sid);
    onClose();
  };

  return (
    <div className="rs-panel rs-archived min-w-0">
      <h2 className="rs-panel-title">Archived sessions</h2>
      <p className="rs-panel-sub">Review sessions hidden from the sidebar and restore the ones you still need.</p>
      {archived.length === 0 ? (
        <div className="rs-archive-empty"><Tray size={26} /><b>No archived sessions</b><span>Archived conversations will appear here.</span></div>
      ) : total === 0 ? (
        <div className="rs-archive-empty"><Archive size={26} /><b>No matches</b><span>No archived session matches “{query}”.</span></div>
      ) : (
        <div className="rs-archive-groups">
          {model.projects.map((project) => (
            <section className="rs-archive-group min-w-0" key={project.key}>
              <header className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2 py-2 text-[12px] text-dim">
                <Folder className="shrink-0" size={15} />
                <span className="min-w-0">
                  <b className="block truncate text-[13px] text-ink-2" title={project.workspace}>{project.label}</b>
                  {project.hint && <span className="block truncate" title={project.workspace}>{project.hint}</span>}
                </span>
                <small className="shrink-0 whitespace-nowrap text-[11px] text-dim">{project.sessions.length} session{project.sessions.length === 1 ? "" : "s"}</small>
              </header>
              {project.sessions.map((session) => {
                const status = sessionFriendlyStatus(session);
                const title = displayTitle(renames, session.id, session.title);
                return (
                  <div className="rs-archive-row flex min-w-0 items-stretch gap-2 max-[520px]:grid max-[520px]:grid-cols-[minmax(0,1fr)_auto] max-[520px]:rounded-[8px] max-[520px]:p-2.5" key={session.id}>
                    <button
                      className="rs-archive-open grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 gap-y-0.5 border-0 bg-transparent p-0 text-left text-inherit"
                      onClick={() => openSession(session.id)}
                      title="Open archived session"
                      aria-label={`Open ${title}`}
                    >
                      <span className="rs-archive-title block min-w-0 truncate text-[13px]" title={title}>{title}</span>
                      <span className={`rs-archive-status ${status.cls} col-start-1 block min-w-0 truncate`}>{status.text}</span>
                      <ArrowUpRight className="col-start-2 row-span-2 row-start-1 shrink-0 self-center text-dim" size={14} aria-hidden="true" />
                    </button>
                    <button className="rs-archive-restore shrink-0 self-center whitespace-nowrap rounded-[6px] border border-line bg-transparent px-2.5 py-1.5 text-[12px] text-ink-2 hover:bg-panel-2" onClick={() => toggleArchive(session.id)}>Unarchive</button>
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
