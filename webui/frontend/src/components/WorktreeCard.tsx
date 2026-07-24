export interface WorktreeSessionItem {
  id: string;
  title: string;
}

interface WorktreeCardProps {
  workspace: string;
  sessions: WorktreeSessionItem[];
  onOpenSession: (sessionId: string) => void;
}

export function WorktreeCard({
  workspace,
  sessions,
  onOpenSession,
}: WorktreeCardProps) {
  return (
    <section className="rs-wt-card min-w-0 overflow-hidden max-[500px]:rounded-[8px] max-[500px]:p-2.5">
      <div className="rs-wt-head min-w-0 max-[500px]:flex-col max-[500px]:items-start max-[500px]:gap-1">
        <span
          className="rs-wt-path mono min-w-0 max-w-full flex-1 whitespace-normal [overflow-wrap:anywhere]"
          title={workspace}
        >
          {workspace}
        </span>
        <span className="rs-wt-count shrink-0 whitespace-nowrap">
          {sessions.length} conversation{sessions.length === 1 ? "" : "s"}
        </span>
      </div>
      <div className="rs-wt-sessions min-w-0">
        {sessions.map((session) => (
          <button
            type="button"
            key={session.id}
            className="rs-wt-session min-w-0 max-w-full w-full whitespace-normal break-words [overflow-wrap:anywhere] text-left"
            onClick={() => onOpenSession(session.id)}
            title="Open this session"
          >
            {session.title}
          </button>
        ))}
      </div>
    </section>
  );
}
