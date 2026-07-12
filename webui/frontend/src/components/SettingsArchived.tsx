import { Archive, ArrowUpRight, Folder, Tray } from "@phosphor-icons/react";
import { useStore } from "../store";
import { buildArchivedModel } from "../viewModels";
import { displayTitle } from "../title";
import { friendlyStatus } from "./pill";

// Status tint for the archived row's status text (was .rs-archive-status.appr/
// .stranded/.crash); everything else stays dim.
const STATUS_TONE: Record<string, string> = {
  appr: "text-amber",
  stranded: "text-amber",
  crash: "text-red",
};

const EMPTY = "rs-archive-empty grid min-h-[220px] place-content-center justify-items-center gap-[5px] text-dim text-center";

export function SettingsArchived({ query, onClose }: { query: string; onClose: () => void }) {
  const { sessions, archived, toggleArchive, renames, select } = useStore();
  const model = buildArchivedModel(sessions, archived, query, (session) => displayTitle(renames, session.id, session.title));
  const total = model.projects.reduce((count, project) => count + project.sessions.length, 0);

  const openTask = (sid: string) => {
    select(sid);
    onClose();
  };

  return (
    <div className="rs-panel rs-archived max-w-[660px] mx-auto">
      <h2 className="rs-panel-title m-0 mb-[4px] text-[19px] font-[650]">Archived tasks</h2>
      <p className="rs-panel-sub m-0 mb-[22px] text-dim text-[13px] leading-[1.5]">Review tasks hidden from the sidebar and restore the ones you still need.</p>
      {archived.length === 0 ? (
        <div className={EMPTY}><Tray size={26} /><b className="text-ink text-[14px]">No archived tasks</b><span className="text-[12.5px]">Archived conversations will appear here.</span></div>
      ) : total === 0 ? (
        <div className={EMPTY}><Archive size={26} /><b className="text-ink text-[14px]">No matches</b><span className="text-[12.5px]">No archived task matches “{query}”.</span></div>
      ) : (
        <div className="rs-archive-groups grid gap-[16px] mt-[20px]">
          {model.projects.map((project) => (
            <section className="rs-archive-group overflow-hidden border border-line rounded-[12px] bg-panel" key={project.key}>
              <header className="flex min-h-[42px] items-center gap-[7px] px-[12px] py-[8px] border-b border-line-2 bg-panel-2">
                <Folder size={15} />
                <b className="text-[13px] font-semibold">{project.label}</b>
                {project.hint && <span className="text-dim text-[11.5px]">{project.hint}</span>}
                <small className="ml-auto text-dim text-[11.5px]">{project.sessions.length} task{project.sessions.length === 1 ? "" : "s"}</small>
              </header>
              {project.sessions.map((session) => {
                const status = friendlyStatus(session.status);
                return (
                  <div
                    className="rs-archive-row flex items-center gap-[10px] py-[7px] pr-[9px] pl-[12px] border-t border-line-2 first-of-type:border-t-0 max-[640px]:flex-col max-[640px]:items-stretch max-[640px]:gap-[4px]"
                    key={session.id}
                  >
                    <button
                      className="rs-archive-open min-w-0 flex-1 grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-[10px] px-[3px] py-[5px] border-0 rounded-none bg-transparent hover:bg-transparent text-left"
                      onClick={() => openTask(session.id)}
                      title="Open archived task"
                    >
                      <span className="rs-archive-title overflow-hidden text-ellipsis whitespace-nowrap">{displayTitle(renames, session.id, session.title)}</span>
                      <span className={`rs-archive-status ${status.cls} text-[11.5px] ${STATUS_TONE[status.cls] ?? "text-dim"}`}>{status.text}</span>
                      <ArrowUpRight size={14} />
                    </button>
                    <button
                      className="rs-archive-restore grow-0 shrink-0 basis-auto px-[10px] py-[5px] rounded-[8px] text-[12px] max-[640px]:self-end"
                      onClick={() => toggleArchive(session.id)}
                    >
                      Unarchive
                    </button>
                  </div>
                );
              })}
            </section>
          ))}
        </div>
      )}
      {archived.length > 0 && <p className="rs-archive-note mt-[14px] mx-[2px] mb-0 text-dim text-[11.5px]">Permanent deletion is not available because the daemon has no delete-task contract.</p>}
    </div>
  );
}
