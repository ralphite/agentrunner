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
  GearSix,
  GitBranch,
  GitFork,
  PencilSimple,
  PushPin,
  WarningCircle,
  X,
} from "@phosphor-icons/react";
import type { Session } from "../types";
import { isManagedWorktreeWorkspace } from "../viewModels";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import { sessionFriendlyStatus, statusWorthShowing } from "./pill";
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
      onMouseEnter={(event) => onPreview(event.currentTarget.getBoundingClientRect().top)}
      onMouseLeave={onPreviewEnd}
    >
      {/* No native `title=` on the row: the hover preview card already carries
          title/project/branch/status, and the OS tooltip both duplicated it and
          rendered on top of the card. `aria-label` keeps the same text for AT. */}
      <button
        className="project-session max-[900px]:min-h-11 [@media(any-pointer:coarse)]:min-h-11"
        onKeyDown={openContextFromKeyboard}
        aria-label={`${title}${isWorktree ? " · Worktree" : ""} · ${unread && status.cls !== "appr" ? "New activity" : status.text}${when ? ` · ${when}` : ""}`}
        aria-current={active ? "page" : undefined}
      >
        <span className="project-session-title">{title}</span>
      </button>
      {/* This strip owns the last ~60px of the row, so it sits squarely on the
          only path from the row to the preview card. Starting the close
          countdown here made the card unreachable: any move slower than
          ~300px/s spent the whole grace period still inside the row, the card
          died mid-travel, and it could not come back because the row's
          mouseenter had already fired. Passing over the actions is not leaving
          the row — the row's own mouseleave is. A click or keyboard focus still
          dismisses immediately, which is what keeps the card off the menu. */}
      {actions && (
        <span
          className="session-quick-actions max-[900px]:inline-flex! [@media(any-pointer:coarse)]:inline-flex!"
          onClick={(event) => {
            event.stopPropagation();
            onDismissPreview();
          }}
          onContextMenu={(event) => {
            event.preventDefault();
            event.stopPropagation();
          }}
          onFocusCapture={onDismissPreview}
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
      {/* Status and state icons live OUTSIDE the title button and to the RIGHT
          of the quick actions. Both facts matter: inside the button, the badge
          shifted left every time hover revealed the ⋯ (the button is the flex
          child that gives up width); to the left of it, the ⋯ would push it.
          Out here and last, the row spends hover width on the title's clip
          point alone and the icons never move. */}
      {(unread || ["appr", "stranded", "crash"].includes(status.cls)) && (
        <span className="session-status-icon">
          {/* A dot, never a count. "2" in a filled circle read as a numbered
              badge competing with the title; the row only has to say *that*
              something needs you. The exact number is one click away, and
              `accessibleLabel` still carries it for screen readers. */}
          {unread && status.cls !== "appr" ? (
            <StatusIndicator
              className="status-dot unread"
              label="New activity"
              tone="info"
              title="New activity"
              aria-hidden="true"
            />
          ) : (
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
          )}
        </span>
      )}
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
  // Explicit registry entry (INC-104): Remove deletes the declaration itself
  // (vs. the derived group's hide/restore), and the dialog opens in edit mode.
  explicit?: boolean;
  chatCount?: number;
  onTogglePin: () => void;
  onReveal: () => void;
  onCreateWorktree: () => void;
  onEdit: () => void;
  onArchiveChats: () => void;
  onToggleRemoved: () => void;
}

export function SidebarProjectActions({
  pinned = false,
  removed = false,
  workspace,
  explicit = false,
  chatCount = 0,
  onTogglePin,
  onReveal,
  onCreateWorktree,
  onEdit,
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
      <MenuItem onClick={onEdit}>
        <GearSix size={16} /> Edit project
      </MenuItem>
      {/* Archiving nothing is not an action; a muted row says why it's inert. */}
      <MenuItem onClick={onArchiveChats} disabled={chatCount === 0}>
        <ArchiveBox size={16} /> Archive chats
      </MenuItem>
      <MenuItem danger={!removed} onClick={onToggleRemoved}>
        <X size={16} /> {explicit ? "Remove" : removed ? "Restore project" : "Remove"}
      </MenuItem>
    </>
  );
}

export type SidebarProjectOverflow = "more" | "less" | null;

export interface SidebarProjectItemProps {
  name: string;
  folded?: boolean;
  removed?: boolean;
  // Rendered as a muted child row when the group is open but holds no
  // sessions — an explicit project the user declared but hasn't chatted in
  // yet (INC-104): "No chats".
  emptyLabel?: string;
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
  folded = false,
  removed = false,
  emptyLabel,
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
        {/* The workspace path lives in the hover preview card, not in a native
            `title=` — the OS tooltip drew over the card and repeated it. */}
        <button
          className="project-heading min-w-0 flex-1"
          onClick={onToggle}
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
        {/* Same as the session row: this strip is on the only path to the
            preview card, so hovering it must not start the close countdown —
            that is what made the card unreachable. Clicks and keyboard focus
            still dismiss immediately. */}
        <span
          className="project-heading-actions"
          onClick={onDismissPreview}
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
      {!folded && emptyLabel && <div className="project-empty-chats">{emptyLabel}</div>}
      {!folded && overflow && (
        <button className="show-more project-show-more" onClick={onToggleOverflow}>
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
      // Explicit projects list every source folder (INC-104); the card is the
      // one honest place a multi-folder project discloses its full extent, and
      // where a folder gone from disk gets its warning.
      folders?: string[];
      missing?: string[];
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
        {(props.folders?.length ? props.folders : [props.workspace || "No workspace"]).map((folder) => {
          const gone = props.missing?.includes(folder);
          return (
            <div className="project-preview-path" key={folder}>
              {gone ? <WarningCircle size={16} /> : <FolderOpen size={16} />}
              <span>{folder}{gone ? " · missing" : ""}</span>
            </div>
          );
        })}
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
      {/* Only states that mean something to the reader. "Ready" and "Stopped"
          are internal lifecycle marks dressed up as news: every idle session is
          "Ready", and a stopped one continues on the next message just the
          same. Show the line when it asks for attention or reports a failure —
          otherwise the card says the useful things and stops. */}
      {statusWorthShowing(props.status.cls) && (
        <div>
          <LifecycleStatus
            accessibleLabel={props.status.text}
            className={`status-dot ${props.status.cls}`}
            state={lifecycleStateFromStatusClass(props.status.cls)}
            aria-hidden="true"
          />
          <span>{props.status.text}</span>
        </div>
      )}
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
