import type { KeyboardEvent, MouseEvent, ReactNode } from "react";
import {
  Archive as ArchiveBox,
  ArrowsOutSimpleIcon,
  ChatCircle,
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
import { StatusIndicator } from "../ui/StatusIndicator";
import {
  LifecycleStatus,
  lifecycleStateFromStatusClass,
} from "../ui/LifecycleStatus";

export interface SidebarSessionItemProps {
  session: Session;
  title: string;
  actions?: ReactNode;
  when?: string;
  nested?: boolean;
  active?: boolean;
  unread?: boolean;
  archived?: boolean;
  pinned?: boolean;
  onSelect: () => void;
  onOpenContext: (x: number, y: number, returnFocus: HTMLElement) => void;
  onPreview: (top: number) => void;
  onPreviewEnd: () => void;
  onDismissPreview: () => void;
  onTogglePin: () => void;
  onToggleArchive: () => void;
}

export function SidebarSessionItem({
  session,
  title,
  actions,
  when = "",
  nested = false,
  active = false,
  unread = false,
  archived = false,
  onSelect,
  onOpenContext,
  onPreview,
  onPreviewEnd,
  onDismissPreview,
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
    onOpenContext(rect.left + 20, rect.top + rect.height, event.currentTarget);
  };

  return (
    <div
      className={`project-session-wrap max-[900px]:min-h-11 [@media(any-pointer:coarse)]:min-h-11${nested ? " nested" : ""}${active ? " current" : ""}${unread ? " unread" : ""}${archived ? " archived" : ""}`}
      data-session-id={session.id}
      onClick={onSelect}
      onContextMenu={(event: MouseEvent<HTMLDivElement>) => {
        event.preventDefault();
        const returnFocus =
          event.currentTarget.querySelector<HTMLElement>(".project-session");
        if (returnFocus) {
          onOpenContext(event.clientX, event.clientY, returnFocus);
        }
      }}
      onMouseEnter={(event) => {
        // T23 marquee: CSS can style the walk but cannot measure how far the
        // title overflows, so hand it the distance here. Publishing nothing when
        // the title already fits leaves the CSS rule inert for short rows.
        const label = event.currentTarget.querySelector<HTMLElement>(
          ".project-session-title",
        );
        if (label) {
          const overflow = label.scrollWidth - label.clientWidth;
          if (overflow > 1) {
            // The fade mask and the walk are both gated on this attribute: an
            // unconditional mask ate the last glyph of titles that fit fine.
            label.dataset.overflow = "";
            label.style.setProperty("--title-shift", `${overflow}px`);
          } else {
            delete label.dataset.overflow;
            label.style.removeProperty("--title-shift");
          }
        }
        onPreview(event.currentTarget.getBoundingClientRect().top);
      }}
      onMouseLeave={onPreviewEnd}
    >
      <button
        className="project-session max-[900px]:min-h-11 [@media(any-pointer:coarse)]:min-h-11"
        onKeyDown={openContextFromKeyboard}
        title={`${session.title || title}\n${status.text}${when ? ` · started ${when}` : ""}\n${session.id}`}
        aria-label={`${title}${isWorktree ? " · Worktree" : ""} · ${unread && status.cls !== "appr" ? "New activity" : status.text}${when ? ` · ${when}` : ""}`}
        aria-current={active ? "page" : undefined}
      >
        <span className="project-session-title">{title}</span>
        {(unread || ["appr", "stranded", "crash"].includes(status.cls)) && (
          actionCount > 1 && status.cls === "appr"
            ? <span className="status-count" title={status.text} aria-hidden="true">{actionCount}</span>
            : unread && status.cls !== "appr"
              ? (
                <StatusIndicator
                  className="status-dot unread"
                  label="New activity"
                  tone="info"
                  title="New activity"
                  aria-hidden="true"
                />
              )
              : (
                <LifecycleStatus
                  accessibleLabel={status.text}
                  className={`status-dot ${status.cls}`}
                  data-display="dot"
                  data-tone={
                    status.cls === "crash"
                      ? "danger"
                      : status.cls === "appr" || status.cls === "stranded"
                        ? "warning"
                        : "neutral"
                  }
                  state={lifecycleStateFromStatusClass(status.cls)}
                  title={status.text}
                  aria-hidden="true"
                />
              )
        )}
      </button>
      {(isWorktree || isRunning) && (
        <span className={`session-state-icons max-[900px]:inline-flex! [@media(any-pointer:coarse)]:inline-flex!${isRunning ? " running" : ""}`}>
          {isWorktree && (
            <span className="session-worktree-icon max-[900px]:inline-grid! [@media(any-pointer:coarse)]:inline-grid!" role="img" title="Worktree session" aria-label="Worktree session">
              <ArrowsOutSimpleIcon size={17} weight="regular" />
            </span>
          )}
          {isRunning && (
            <LifecycleStatus
              accessibleLabel="Session running"
              className="session-loading-icon"
              role="status"
              size="md"
              state="running"
            />
          )}
        </span>
      )}
      {actions && (
        <span
          className="session-quick-actions max-[900px]:inline-flex! [@media(any-pointer:coarse)]:inline-flex!"
          onClick={(event) => event.stopPropagation()}
          onContextMenu={(event) => {
            event.preventDefault();
            event.stopPropagation();
          }}
          onMouseEnter={onDismissPreview}
        >
          <Menu
            label={<DotsThree size={16} />}
            ariaLabel={`More actions for ${title}`}
            iconTrigger
            triggerClassName="session-touch-trigger max-[900px]:h-11! max-[900px]:w-11! [@media(any-pointer:coarse)]:h-11! [@media(any-pointer:coarse)]:w-11!"
          >
            {actions}
          </Menu>
        </span>
      )}
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
      <MenuLabel title={title}>
        <span className="block max-w-[188px] truncate">{title}</span>
      </MenuLabel>
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
  onOpenContext: (x: number, y: number, returnFocus: HTMLElement) => void;
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
    onOpenContext(rect.left + 20, rect.bottom, event.currentTarget);
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
            onOpenContext(
              event.clientX,
              event.clientY,
              event.currentTarget,
            );
          }}
          onKeyDown={openContextFromKeyboard}
        >
          <span
            className="proj-icon-slot"
            data-project-icon={folded ? "folded" : "expanded"}
            aria-hidden="true"
          >
            {folded
              ? <Folder className="proj-folder" size={16} />
              : <FolderOpen className="proj-folder" size={16} />}
          </span>
          <span className="proj-heading-text">
            <span className="proj-heading-name">{name}</span>
          </span>
        </button>
        <span
          className="project-heading-actions"
          onClick={onDismissPreview}
          onMouseEnter={onDismissPreview}
          onFocusCapture={onDismissPreview}
        >
          <Menu
            label={<DotsThree size={16} />}
            ariaLabel={`More actions for ${name}`}
            iconTrigger
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
        <button className="show-more project-show-more" onClick={onToggleOverflow}>
          {overflow === "more" ? "Show more sessions" : "Show fewer sessions"}
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
      onHoverStart?: () => void;
      onHoverEnd?: () => void;
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
      onHoverStart?: () => void;
      onHoverEnd?: () => void;
    };

export function SidebarPreviewCard(props: SidebarPreviewCardProps) {
  const style = props.inline
    ? { position: "static" as const }
    : { top: props.top };
  if (props.kind === "project") {
    return (
      <div
        className="project-preview"
        style={style}
        aria-hidden="true"
        onMouseEnter={props.onHoverStart}
        onMouseLeave={props.onHoverEnd}
      >
        <div className="project-preview-head">
          <Folder size={18} />
          <b>{props.name}</b>
          <PushPin size={16} weight={props.pinned ? "fill" : "regular"} />
        </div>
        <div><ChatCircle size={16} /><span>{props.chats} {props.chats === 1 ? "chat" : "chats"}</span></div>
        <div className="project-preview-path">
          <FolderOpen size={16} />
          <span title={props.workspace || "No workspace"}>{props.workspace || "No workspace"}</span>
        </div>
      </div>
    );
  }

  return (
    <div
      className="session-preview"
      style={style}
      aria-hidden="true"
      onMouseEnter={props.onHoverStart}
      onMouseLeave={props.onHoverEnd}
    >
      <div className="session-preview-head"><b>{props.title}</b></div>
      <div><Folder size={15} /><span>{props.project || "No project"}</span></div>
      {props.branch && <div><GitBranch size={15} /><span>{props.branch}</span></div>}
      <div>
        <LifecycleStatus
          accessibleLabel={props.status.text}
          className={`status-dot ${props.status.cls}`}
          state={lifecycleStateFromStatusClass(props.status.cls)}
          aria-hidden="true"
        />
        <span>{props.status.text}</span>
      </div>
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

  // A healthy daemon is the baseline, not a sidebar destination. Keep the
  // recovery affordance for an actual outage, but don't spend a permanent row
  // restating that the local app is connected.
  return null;
}
