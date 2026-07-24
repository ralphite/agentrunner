import type { KeyboardEvent, MouseEvent, ReactNode } from "react";
import {
  Archive as ArchiveBox,
  ArrowsOutSimple,
  CaretRight,
  ChatCircle,
  CircleNotch,
  DotsThree,
  EnvelopeSimple,
  EnvelopeSimpleOpen,
  Folder,
  FolderOpen,
  GitBranch,
  GitFork,
  PencilSimple,
  PushPin,
  X,
} from "@phosphor-icons/react";
import type { Session } from "../types";
import { isManagedWorktreeWorkspace } from "../viewModels";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import { sessionFriendlyStatus } from "./pill";
import { IconButton } from "../ui/IconButton";

export interface SidebarSessionItemProps {
  session: Session;
  title: string;
  when?: string;
  nested?: boolean;
  active?: boolean;
  unread?: boolean;
  archived?: boolean;
  pinned?: boolean;
  onSelect: () => void;
  onOpenContext: (x: number, y: number) => void;
  onPreview: (top: number) => void;
  onPreviewEnd: () => void;
  onDismissPreview: () => void;
  onTogglePin: () => void;
  onToggleArchive: () => void;
}

export function SidebarSessionItem({
  session,
  title,
  when = "",
  nested = false,
  active = false,
  unread = false,
  archived = false,
  pinned = false,
  onSelect,
  onOpenContext,
  onPreview,
  onPreviewEnd,
  onDismissPreview,
  onTogglePin,
  onToggleArchive,
}: SidebarSessionItemProps) {
  const status = sessionFriendlyStatus(session);
  const isRunning = status.cls === "run";
  const isWorktree = isManagedWorktreeWorkspace(session.workspace);
  const actionCount =
    (session.attention?.approvals || 0) +
    (session.attention?.answers || 0);

  const openContextFromKeyboard = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (!((event.shiftKey && event.key === "F10") || event.key === "ContextMenu")) return;
    event.preventDefault();
    const rect = event.currentTarget.getBoundingClientRect();
    onOpenContext(rect.left + 20, rect.top + rect.height);
  };

  return (
    <div
      className={`project-session-wrap${nested ? " nested" : ""}${active ? " current" : ""}${unread ? " unread" : ""}${archived ? " archived" : ""}`}
      onContextMenu={(event: MouseEvent<HTMLDivElement>) => {
        event.preventDefault();
        onOpenContext(event.clientX, event.clientY);
      }}
      onMouseEnter={(event) => onPreview(event.currentTarget.getBoundingClientRect().top)}
      onMouseLeave={onPreviewEnd}
    >
      <button
        className="project-session"
        onClick={onSelect}
        onKeyDown={openContextFromKeyboard}
        title={`${session.title || title}\n${status.text}${when ? ` · started ${when}` : ""}\n${session.id}`}
        aria-label={`${title} · ${unread && status.cls !== "appr" ? "New activity" : status.text}${when ? ` · ${when}` : ""}`}
      >
        <span className="project-session-title">{title}</span>
        {(unread || ["appr", "stranded", "crash"].includes(status.cls)) && (
          actionCount > 1 && status.cls === "appr"
            ? <span className="status-count" title={status.text} aria-hidden="true">{actionCount}</span>
            : (
              <span
                className={`status-dot ${unread && status.cls !== "appr" ? "unread" : status.cls}`}
                title={unread && status.cls !== "appr" ? "New activity" : status.text}
              />
            )
        )}
      </button>
      {(isWorktree || isRunning) && (
        <span className={`session-state-icons${isRunning ? " running" : ""}`}>
          {isWorktree && (
            <span className="session-worktree-icon" role="img" title="Worktree session" aria-label="Worktree session">
              <ArrowsOutSimple size={17} />
            </span>
          )}
          {isRunning && (
            <CircleNotch className="session-loading-icon" size={17} role="status" aria-label="Session running" />
          )}
        </span>
      )}
      <span
        className="session-quick-actions"
        onMouseEnter={onDismissPreview}
      >
        <IconButton
          size="sm"
          variant="ghost"
          className="session-quick-action"
          aria-label={`${pinned ? "Unpin" : "Pin"} ${title}`}
          title={pinned ? "Unpin" : "Pin"}
          onClick={onTogglePin}
        >
          <PushPin size={17} weight={pinned ? "fill" : "regular"} />
        </IconButton>
        <IconButton
          size="sm"
          variant="ghost"
          className="session-quick-action"
          aria-label={`${archived ? "Unarchive" : "Archive"} ${title}`}
          title={archived ? "Unarchive" : "Archive"}
          onClick={onToggleArchive}
        >
          <ArchiveBox size={17} />
        </IconButton>
      </span>
    </div>
  );
}

export interface SidebarSessionActionsProps {
  title: string;
  pinned?: boolean;
  unread?: boolean;
  archived?: boolean;
  onTogglePin: () => void;
  onRename: () => void;
  onToggleRead: () => void;
  onToggleArchive: () => void;
}

export function SidebarSessionActions({
  title,
  pinned = false,
  unread = false,
  archived = false,
  onTogglePin,
  onRename,
  onToggleRead,
  onToggleArchive,
}: SidebarSessionActionsProps) {
  return (
    <>
      <MenuLabel>{title}</MenuLabel>
      <MenuItem onClick={onTogglePin}>
        <PushPin size={16} weight={pinned ? "fill" : "regular"} /> {pinned ? "Unpin" : "Pin"}
      </MenuItem>
      <MenuItem onClick={onRename}>
        <PencilSimple size={16} /> Rename…
      </MenuItem>
      <MenuItem onClick={onToggleRead}>
        {unread ? <EnvelopeSimpleOpen size={16} /> : <EnvelopeSimple size={16} />}
        {unread ? "Mark as read" : "Mark as unread"}
      </MenuItem>
      <MenuItem onClick={onToggleArchive}>
        <ArchiveBox size={16} /> {archived ? "Unarchive" : "Archive"}
      </MenuItem>
    </>
  );
}

export interface SidebarProjectActionsProps {
  pinned?: boolean;
  removed?: boolean;
  workspace?: string;
  onTogglePin: () => void;
  onReveal: () => void;
  onCreateWorktree: () => void;
  onRename: () => void;
  onArchiveChats: () => void;
  onToggleRemoved: () => void;
}

export function SidebarProjectActions({
  pinned = false,
  removed = false,
  workspace,
  onTogglePin,
  onReveal,
  onCreateWorktree,
  onRename,
  onArchiveChats,
  onToggleRemoved,
}: SidebarProjectActionsProps) {
  return (
    <>
      <MenuItem onClick={onTogglePin}>
        <PushPin size={16} weight={pinned ? "fill" : "regular"} /> {pinned ? "Unpin project" : "Pin project"}
      </MenuItem>
      {workspace && (
        <MenuItem onClick={onReveal}>
          <FolderOpen size={16} /> Reveal in Finder
        </MenuItem>
      )}
      {workspace && (
        <MenuItem onClick={onCreateWorktree}>
          <GitFork size={16} /> Create permanent worktree
        </MenuItem>
      )}
      <MenuItem onClick={onRename}>
        <PencilSimple size={16} /> Rename project
      </MenuItem>
      <MenuItem onClick={onArchiveChats}>
        <ArchiveBox size={16} /> Archive chats
      </MenuItem>
      <MenuItem danger={!removed} onClick={onToggleRemoved}>
        <X size={16} /> {removed ? "Restore project" : "Remove"}
      </MenuItem>
    </>
  );
}

export type SidebarProjectOverflow = "more" | "less" | null;

export interface SidebarProjectItemProps {
  name: string;
  workspace?: string;
  folded?: boolean;
  removed?: boolean;
  children?: ReactNode;
  actions: ReactNode;
  overflow?: SidebarProjectOverflow;
  onToggle: () => void;
  onOpenContext: (x: number, y: number) => void;
  onPreview: (top: number) => void;
  onPreviewEnd: () => void;
  onDismissPreview: () => void;
  onNewChat: () => void;
  onToggleOverflow?: () => void;
}

export function SidebarProjectItem({
  name,
  workspace,
  folded = false,
  removed = false,
  children,
  actions,
  overflow = null,
  onToggle,
  onOpenContext,
  onPreview,
  onPreviewEnd,
  onDismissPreview,
  onNewChat,
  onToggleOverflow,
}: SidebarProjectItemProps) {
  const openContextFromKeyboard = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (!((event.shiftKey && event.key === "F10") || event.key === "ContextMenu")) return;
    event.preventDefault();
    const rect = event.currentTarget.getBoundingClientRect();
    onOpenContext(rect.left + 20, rect.bottom);
  };

  return (
    <div className="project-group" data-project-state={removed ? "removed" : "visible"}>
      <div
        className="project-heading-row"
        onMouseEnter={(event) => onPreview(event.currentTarget.getBoundingClientRect().top)}
        onMouseLeave={onPreviewEnd}
      >
        <button
          className="project-heading min-w-0 flex-1"
          onClick={onToggle}
          title={workspace || name}
          aria-expanded={!folded}
          onContextMenu={(event) => {
            event.preventDefault();
            onOpenContext(event.clientX, event.clientY);
          }}
          onKeyDown={openContextFromKeyboard}
        >
          <span className="proj-icon-slot">
            <CaretRight className={`proj-caret${!folded ? " open" : ""}`} size={11} weight="bold" aria-hidden="true" />
            <Folder className="proj-folder" size={16} />
          </span>
          <span className="proj-heading-text">
            <span className="proj-heading-name">{name}</span>
          </span>
        </button>
        <span className="project-heading-actions" onClick={onDismissPreview}>
          <Menu
            label={<DotsThree size={18} weight="bold" />}
            ariaLabel={`More actions for ${name}`}
          >
            {actions}
          </Menu>
          <IconButton
            size="sm"
            variant="ghost"
            className="project-quick-action max-[900px]:hidden!"
            aria-label={`New chat in ${name}`}
            title="New chat"
            onClick={onNewChat}
          >
            <PencilSimple size={16} />
          </IconButton>
        </span>
      </div>
      {children}
      {!folded && overflow && (
        <button className="show-more" onClick={onToggleOverflow}>
          {overflow === "more" ? "Show more" : "Show less"}
        </button>
      )}
    </div>
  );
}

export type SidebarPreviewCardProps =
  | {
      kind: "project";
      top: number;
      name: string;
      pinned?: boolean;
      chats: number;
      workspace?: string;
      inline?: boolean;
    }
  | {
      kind: "session";
      top: number;
      title: string;
      when?: string;
      project?: string;
      branch?: string;
      status: { text: string; cls: string };
      inline?: boolean;
    };

export function SidebarPreviewCard(props: SidebarPreviewCardProps) {
  const style = props.inline
    ? { position: "static" as const }
    : { top: props.top };
  if (props.kind === "project") {
    return (
      <div className="project-preview" style={style} aria-hidden="true">
        <div className="project-preview-head">
          <Folder size={18} />
          <b>{props.name}</b>
          <PushPin size={16} weight={props.pinned ? "fill" : "regular"} />
        </div>
        <div><ChatCircle size={16} /><span>{props.chats} {props.chats === 1 ? "chat" : "chats"}</span></div>
        <div className="project-preview-path"><FolderOpen size={16} /><span>{props.workspace || "No workspace"}</span></div>
      </div>
    );
  }

  return (
    <div className="session-preview" style={style} aria-hidden="true">
      <div className="session-preview-head"><b>{props.title}</b>{props.when && <span>{props.when}</span>}</div>
      <div><Folder size={15} /><span>{props.project || "No project"}</span></div>
      <div><GitBranch size={15} /><span>{props.branch || "Local"}</span></div>
      <div><span className={`status-dot ${props.status.cls}`} /><span>{props.status.text}</span></div>
    </div>
  );
}

export type SidebarConnectionState = "checking" | "connected" | "offline";

export interface SidebarConnectionStatusProps {
  state: SidebarConnectionState;
  version?: string;
  onRestart: () => void;
}

export function SidebarConnectionStatus({
  state,
  version,
  onRestart,
}: SidebarConnectionStatusProps) {
  if (state === "offline") {
    return (
      <button
        className="account-badge"
        onClick={onRestart}
        title="Daemon offline — click to restart"
        aria-label="Daemon offline — click to restart"
      >
        <span className="account-avatar offline" aria-hidden="true">
          <span className="text-[11px] font-[680] tracking-[0.4px]">AR</span>
          <span className="account-presence" />
        </span>
        <span className="account-meta"><span>Daemon offline — restart</span></span>
      </button>
    );
  }

  const checking = state === "checking";
  return (
    <div
      className="account-badge"
      role="status"
      title={checking ? "Checking daemon status…" : `Connected to daemon · ${version || "unknown version"}`}
      aria-label={checking ? "Connecting to daemon" : "Connected to daemon"}
    >
      <span className={`account-avatar ${checking ? "connecting" : "online"}`} aria-hidden="true">
        <span className="text-[11px] font-[680] tracking-[0.4px]">AR</span>
        <span className="account-presence" />
      </span>
      <span className="account-meta"><span>{checking ? "Connecting…" : "Connected"}</span></span>
    </div>
  );
}
