import { ArrowUpRight, Folder } from "@phosphor-icons/react";
import type { Session } from "../types";
import type { ProjectGroup } from "../viewModels";
import { sessionFriendlyStatus } from "./pill";
import { Button } from "../ui/Button";

interface ArchivedSessionItemProps {
  session: Session;
  title: string;
  onOpen: (sessionId: string) => void;
  onUnarchive: (sessionId: string) => void;
}

export function ArchivedSessionItem({
  session,
  title,
  onOpen,
  onUnarchive,
}: ArchivedSessionItemProps) {
  const status = sessionFriendlyStatus(session);
  return (
    <div className="rs-archive-row flex min-w-0 items-stretch gap-2 max-[520px]:grid max-[520px]:grid-cols-[minmax(0,1fr)_auto] max-[520px]:rounded-[8px] max-[520px]:p-2.5">
      <button
        type="button"
        className="rs-archive-open grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 gap-y-0.5 border-0 bg-transparent p-0 text-left text-inherit"
        onClick={() => onOpen(session.id)}
        title="Open archived session"
        aria-label={`Open ${title}`}
      >
        <span
          className="rs-archive-title block min-w-0 truncate text-[13px]"
          title={title}
        >
          {title}
        </span>
        <span
          className={`rs-archive-status ${status.cls} col-start-1 block min-w-0 truncate`}
        >
          {status.text}
        </span>
        <ArrowUpRight
          className="col-start-2 row-span-2 row-start-1 shrink-0 self-center text-dim"
          size={14}
          aria-hidden="true"
        />
      </button>
      <Button
        size="md"
        variant="outline"
        className="rs-archive-restore shrink-0 self-center whitespace-nowrap rounded-[6px] border border-line bg-transparent px-2.5 py-1.5 text-[12px] text-ink-2 hover:bg-panel-2"
        onClick={() => onUnarchive(session.id)}
      >
        Unarchive
      </Button>
    </div>
  );
}

interface ArchivedProjectGroupProps {
  project: ProjectGroup;
  titleOf: (session: Session) => string;
  onOpen: (sessionId: string) => void;
  onUnarchive: (sessionId: string) => void;
}

export function ArchivedProjectGroup({
  project,
  titleOf,
  onOpen,
  onUnarchive,
}: ArchivedProjectGroupProps) {
  return (
    <section className="rs-archive-group min-w-0">
      <header className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2 py-2 text-[12px] text-dim">
        <Folder className="shrink-0" size={15} />
        <span className="min-w-0">
          <b
            className="block truncate text-[13px] text-ink-2"
            title={project.workspace}
          >
            {project.label}
          </b>
          {project.hint && (
            <span className="block truncate" title={project.workspace}>
              {project.hint}
            </span>
          )}
        </span>
        <small className="shrink-0 whitespace-nowrap text-[11px] text-dim">
          {project.sessions.length} session
          {project.sessions.length === 1 ? "" : "s"}
        </small>
      </header>
      {project.sessions.map((session) => (
        <ArchivedSessionItem
          key={session.id}
          session={session}
          title={titleOf(session)}
          onOpen={onOpen}
          onUnarchive={onUnarchive}
        />
      ))}
    </section>
  );
}
