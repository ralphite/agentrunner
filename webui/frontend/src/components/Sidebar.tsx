import { useEffect, useMemo, useState } from "react";
import "../styles.nav.css";
import {
  Archive as ArchiveBox,
  ArrowSquareOut,
  CalendarDots,
  CaretRight,
  Code,
  Folder,
  FolderOpen,
  GearSix,
  GitBranch,
  GitPullRequest,
  type Icon,
  MagnifyingGlass,
  Monitor,
  Moon,
  NotePencil,
  PencilSimple,
  Plugs,
  PlusCircle,
  PushPin,
  Question,
  Robot,
  SidebarSimple,
  SquaresFour,
  Sun,
  Terminal,
  Tray,
  X,
} from "@phosphor-icons/react";
import { useStore, type Page } from "../store";
import { AR } from "../api";
import { friendlyStatus } from "./pill";
import { displayTitle } from "../title";
import { ContextMenu } from "./ContextMenu";
import { MenuItem, MenuLabel } from "./Menu";
import { copyText } from "../clipboard";
import { buildSidebarModel, daemonVersionLabel, projectDisplayName, projectLabel, scheduledUnread, visibleProjectSessions } from "../viewModels";
import { relTime, sessionDate } from "../time";

type SidebarContext =
  | { kind: "session"; x: number; y: number; sid: string }
  | { kind: "project"; x: number; y: number; key: string; label: string; workspace?: string; ids: string[] };

// Primary-nav destinations (Codex parity: New task / Scheduled / Plugins /
// Sites / Pull requests / Chat), in Codex's order and iconography. Kept as a
// small table rendered in a map so adding a destination is one row here + a
// page dispatch in App.tsx — no per-button JSX duplication. The Scheduled row
// alone carries the live activity dot, keyed off `key === "scheduled"`.
const NAV_DESTINATIONS: { key: Page; label: string; icon: Icon }[] = [
  { key: "home", label: "New task", icon: NotePencil },
  { key: "scheduled", label: "Scheduled", icon: CalendarDots },
  { key: "plugins", label: "Plugins", icon: Plugs },
  { key: "sites", label: "Sites", icon: SquaresFour },
  { key: "pulls", label: "Pull requests", icon: GitPullRequest },
  { key: "chat", label: "Chat", icon: PlusCircle },
];

export function Sidebar({ onHide, onNavigate, onOpenSettings }: { onHide?: () => void; onNavigate?: () => void; onOpenSettings?: () => void }) {
  const {
    health,
    sessions,
    sessionsReady,
    sessionsLoadingOlder,
    runs,
    currentSid,
    currentPage,
    select,
    showPage,
    refreshHealth,
    toast,
    archived,
    showArchived,
    toggleShowArchived,
    toggleArchive,
    pinned,
    togglePin,
    renames,
    theme,
    cycleTheme,
    setVisibleOrder,
    toggleSidebar,
    unread,
    markUnread,
    markRead,
    openHelp,
    projects,
    toggleProjectFolded,
    openProjectIn,
    setProjectName,
    openPrompt,
  } = useStore();
  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [ctx, setCtx] = useState<SidebarContext | null>(null);
  const [hoverPreview, setHoverPreview] = useState<{ sid: string; top: number } | null>(null);
  const [branchByWorkspace, setBranchByWorkspace] = useState<Record<string, string>>({});

  const model = useMemo(
    () => buildSidebarModel(sessions, {
      pinned,
      archived,
      showArchived,
      query,
      titleOf: (session) => displayTitle(renames, session.id, session.title),
    }),
    [sessions, pinned, archived, showArchived, query, renames],
  );
  const archivedCount = sessions.filter((session) => archived.includes(session.id)).length;
  const runningRuns = runs.filter((run) => run.status === "running").length;
  const schedUnread = scheduledUnread(sessions, unread);
  const orderedIds = useMemo(
    () => [...model.pinned.map((session) => session.id), ...model.projects.flatMap((project) => project.sessions.map((session) => session.id))],
    [model],
  );
  useEffect(() => setVisibleOrder(orderedIds), [orderedIds, setVisibleOrder]);

  const restartDaemon = async () => {
    try {
      await AR.daemonStart();
      toast("daemon start requested", "info");
      setTimeout(refreshHealth, 800);
    } catch (error: any) {
      toast(error.message);
    }
  };

  const previewTask = (session: (typeof sessions)[number], top: number) => {
    // The hover preview and the right-click context menu are mutually
    // exclusive floating layers — while a menu is open, suppress the preview
    // so the two never stack and fight for the same corner (R3-1).
    if (ctx) return;
    setHoverPreview({ sid: session.id, top: Math.max(10, Math.min(top - 6, window.innerHeight - 154)) });
    const workspace = session.workspace;
    if (!workspace || Object.prototype.hasOwnProperty.call(branchByWorkspace, workspace)) return;
    setBranchByWorkspace((current) => ({ ...current, [workspace]: "" }));
    AR.gitBranches(workspace)
      .then((info) => setBranchByWorkspace((current) => ({
        ...current,
        [workspace]: info.isRepo && info.current ? info.current : "Local workspace",
      })))
      .catch(() => setBranchByWorkspace((current) => ({ ...current, [workspace]: "Local workspace" })));
  };

  const renderTask = (session: (typeof sessions)[number], nested = false) => {
    const active = session.id === currentSid;
    const status = friendlyStatus(session.status);
    const isUnread = unread.includes(session.id);
    const isPinned = pinned.includes(session.id);
    const title = displayTitle(renames, session.id, session.title);
    const when = relTime(sessionDate(session.id));
    const openContext = (x: number, y: number) => {
      // Opening a context menu instantly dismisses any hover preview so the
      // two floating layers stay mutually exclusive (R3-1).
      setHoverPreview(null);
      setCtx({ kind: "session", x, y, sid: session.id });
    };
    return (
      <div
        key={session.id}
        className={`project-task-wrap${nested ? " nested" : ""}${active ? " current" : ""}${isUnread ? " unread" : ""}${archived.includes(session.id) ? " archived" : ""}`}
        onContextMenu={(event) => {
          event.preventDefault();
          openContext(event.clientX, event.clientY);
        }}
        onMouseEnter={(event) => previewTask(session, event.currentTarget.getBoundingClientRect().top)}
        onMouseLeave={() => setHoverPreview((current) => current?.sid === session.id ? null : current)}
      >
        <button
          className="project-task"
          onClick={() => {
            select(session.id);
            onNavigate?.();
          }}
          onKeyDown={(event) => {
            if ((event.shiftKey && event.key === "F10") || event.key === "ContextMenu") {
              event.preventDefault();
              const rect = event.currentTarget.getBoundingClientRect();
              openContext(rect.left + 20, rect.top + rect.height);
            }
          }}
          title={`${session.title || title}\n${status.text}${when ? ` · started ${when} ago` : ""}\n${session.id}`}
          aria-label={`${title} · ${isUnread ? "New activity" : status.text}${when ? ` · ${when} ago` : ""}`}
        >
          <span className="project-task-title">{title}</span>
          {when && <span className="flex-none text-[10.5px] text-dim">{when}</span>}
          <span className={`status-dot ${isUnread ? "unread" : status.cls}`} title={isUnread ? "New activity" : status.text} />
          <ArrowSquareOut className="task-open" size={13} />
        </button>
        <button
          className={`task-pin${isPinned ? " active" : ""}`}
          tabIndex={-1}
          title={isPinned ? "Unpin task" : "Pin task"}
          aria-label={isPinned ? "Unpin task" : "Pin task"}
          onClick={(event) => {
            event.stopPropagation();
            togglePin(session.id);
          }}
        >
          <PushPin size={13} weight={isPinned ? "fill" : "regular"} />
        </button>
        <button
          className="task-archive"
          tabIndex={-1}
          title={archived.includes(session.id) ? "Unarchive task" : "Archive task"}
          aria-label={archived.includes(session.id) ? "Unarchive task" : "Archive task"}
          onClick={(event) => {
            event.stopPropagation();
            toggleArchive(session.id);
          }}
        >
          <ArchiveBox size={13} />
        </button>
      </div>
    );
  };

  const themeGlyph = theme === "system" ? <Monitor size={15} /> : theme === "light" ? <Sun size={15} /> : <Moon size={15} />;

  return (
    <aside className="sidebar">
      <div className="flex items-center justify-between min-h-[64px] pt-[12px] pr-[14px] pb-[8px] pl-[16px]">
        <button className="brand-main" onClick={() => { showPage("home"); onNavigate?.(); }} aria-label="AgentRunner home">
          <span className="w-[26px] h-[26px] grid place-items-center text-accent-ink bg-accent rounded-[8px]"><Robot size={17} weight="bold" /></span>
          <span className="text-[16px] font-[650] tracking-[-0.2px]">AgentRunner</span>
        </button>
        <div className="flex items-center gap-[2px]">
          <button className="w-[30px] h-[30px] grid place-items-center p-0 border-0 bg-transparent text-ink-2 rounded-[8px] hover:text-ink hover:bg-[color-mix(in_srgb,var(--ink)_6%,transparent)]" onClick={() => setSearching((value) => !value)} title="Search tasks">
            <MagnifyingGlass size={16} />
          </button>
          <button className="w-[30px] h-[30px] grid place-items-center p-0 border-0 bg-transparent text-ink-2 rounded-[8px] hover:text-ink hover:bg-[color-mix(in_srgb,var(--ink)_6%,transparent)]" onClick={onHide || toggleSidebar} title="Hide sidebar (⌘B)">
            <SidebarSimple size={16} />
          </button>
        </div>
      </div>

      <nav className="primary-nav" aria-label="Primary">
        {NAV_DESTINATIONS.map(({ key, label, icon: DestIcon }) => (
          <button
            key={key}
            className={!currentSid && currentPage === key ? "active" : ""}
            onClick={() => { showPage(key); onNavigate?.(); }}
          >
            <DestIcon size={17} /> <span>{label}</span>
            {key === "scheduled" && (schedUnread.length > 0 || runningRuns > 0) && (
              <span
                className={`nav-notice${schedUnread.length > 0 ? " unread" : " running"}`}
                title={schedUnread.length > 0 ? `${schedUnread.length} with new activity` : `${runningRuns} running`}
              />
            )}
          </button>
        ))}
      </nav>

      {searching && (
        <div className="side-search">
          <MagnifyingGlass size={14} />
          <input
            autoFocus
            value={query}
            placeholder="Search title, id, or workspace"
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key !== "Escape") return;
              if (query) setQuery("");
              else setSearching(false);
            }}
          />
          {query && <button onClick={() => setQuery("")} aria-label="Clear search"><X size={13} /></button>}
        </div>
      )}

      <div className="project-list">
        {model.pinned.length > 0 && (
          <section className="sidebar-section pinned-section">
            <div className="section-label"><PushPin size={12} weight="fill" /> Pinned</div>
            {model.pinned.map((session) => renderTask(session))}
          </section>
        )}

        <section className="sidebar-section projects-section">
          <div className="section-label">Projects</div>
          {!sessionsReady ? (
            <div className="sidebar-loading" role="status" aria-label="Loading tasks">
              <span />
              <span />
              <span />
            </div>
          ) : model.projects.length === 0 ? (
            <div className="sidebar-empty">
              <Tray size={22} />
              <b>{query ? "No matching tasks" : "No tasks yet"}</b>
              <span>{query ? "Try a different search." : "Start a task to see it here."}</span>
            </div>
          ) : null}
          {model.projects.map((project) => {
            const overlay = projects[project.key];
            const name = projectDisplayName(project, overlay);
            const searching = !!query;
            // Persisted fold collapses the group entirely; a search always
            // reveals matches, so it overrides fold. The local `expanded` set
            // is the secondary show-all-vs-6 control within an unfolded group.
            const folded = (overlay?.folded ?? false) && !searching;
            const showAll = expanded.has(project.key) || searching;
            const shown = visibleProjectSessions(project, { folded: overlay?.folded, expanded: showAll, searching });
            const openMenu = (x: number, y: number) => {
              setHoverPreview(null);
              setCtx({ kind: "project", x, y, key: project.key, label: name, workspace: project.workspace, ids: project.sessions.map((session) => session.id) });
            };
            return (
              <div className="project-group" key={project.key}>
                <button
                  className="project-heading"
                  onClick={() => toggleProjectFolded(project.key, !(overlay?.folded ?? false))}
                  title={project.workspace || name}
                  onContextMenu={(event) => {
                    event.preventDefault();
                    openMenu(event.clientX, event.clientY);
                  }}
                  onKeyDown={(event) => {
                    if (!((event.shiftKey && event.key === "F10") || event.key === "ContextMenu")) return;
                    event.preventDefault();
                    const rect = event.currentTarget.getBoundingClientRect();
                    openMenu(rect.left + 20, rect.bottom);
                  }}
                >
                  <CaretRight size={12} className={`proj-caret${!folded ? " open" : ""}`} />
                  {!folded ? <FolderOpen size={16} /> : <Folder size={16} />}
                  <span>{name}</span>
                  {project.hint && <span className="project-hint">{project.hint}</span>}
                </button>
                {shown.map((session) => renderTask(session, true))}
                {!folded && !showAll && project.sessions.length > shown.length && (
                  <button className="show-more" onClick={() => setExpanded((current) => new Set(current).add(project.key))}>
                    Show {project.sessions.length - shown.length} more
                  </button>
                )}
                {!folded && showAll && !searching && project.sessions.length > 6 && (
                  <button
                    className="show-more"
                    onClick={() => setExpanded((current) => {
                      const next = new Set(current);
                      next.delete(project.key);
                      return next;
                    })}
                  >
                    Show less
                  </button>
                )}
              </div>
            );
          })}
          {sessionsLoadingOlder && (
            <div className="sidebar-history-loading" role="status">Loading older tasks…</div>
          )}
          {archivedCount > 0 && (
            <button className="archive-toggle" onClick={toggleShowArchived}>
              <ArchiveBox size={14} /> {showArchived ? "Hide" : "Show"} archived · {archivedCount}
            </button>
          )}
        </section>
      </div>

      <div className="side-foot">
        {/* INC-41 L3 · Three states, not two. `health === null` means the first
            /health call hasn't answered yet — rendering that as a red "Daemon
            offline" made every cold load flash a fake outage (and armed a
            restart click). Unknown is neutral and inert; only a health record
            that actually says daemonUp:false is an outage. */}
        <button
          className="account-badge"
          onClick={() => health && !health.daemonUp && restartDaemon()}
          title={
            !health
              ? "Checking daemon status…"
              : health.daemonUp
                ? (health.version || "daemon")
                : "Daemon offline — click to restart"
          }
          aria-label={
            !health
              ? "Connecting to daemon"
              : health.daemonUp
                ? "Connected to daemon"
                : "Daemon offline — click to restart"
          }
        >
          <span
            className={`account-avatar${!health ? " connecting" : health.daemonUp ? " online" : " offline"}`}
            aria-hidden="true"
          >
            <span className="text-[11px] font-[680] tracking-[0.4px]">AR</span>
            <span className="account-presence" />
          </span>
          <span className="account-meta">
            <b>AgentRunner</b>
            <span>
              {!health
                ? "Connecting…"
                : health.daemonUp
                  ? `Connected · ${daemonVersionLabel(health.version)}`
                  : "Daemon offline — restart"}
            </span>
          </span>
        </button>
        <div className="flex flex-none items-center gap-[2px]">
          {onOpenSettings && (
            <button className="w-[30px] h-[30px] grid place-items-center p-0 border-0 bg-transparent text-ink-2 rounded-[8px] hover:text-ink hover:bg-[color-mix(in_srgb,var(--ink)_6%,transparent)]" onClick={onOpenSettings} title="Settings (⌘,)" aria-label="Open settings">
              <GearSix size={16} />
            </button>
          )}
          <button className="w-[30px] h-[30px] grid place-items-center p-0 border-0 bg-transparent text-ink-2 rounded-[8px] hover:text-ink hover:bg-[color-mix(in_srgb,var(--ink)_6%,transparent)]" onClick={openHelp} title="Keyboard shortcuts & help (?)" aria-label="Help and keyboard shortcuts">
            <Question size={16} />
          </button>
          <button className="w-[30px] h-[30px] grid place-items-center p-0 border-0 bg-transparent text-ink-2 rounded-[8px] hover:text-ink hover:bg-[color-mix(in_srgb,var(--ink)_6%,transparent)]" onClick={cycleTheme} title={`Theme: ${theme}`} aria-label="Toggle theme">
            {themeGlyph}
          </button>
        </div>
      </div>

      {hoverPreview && (() => {
        const session = sessions.find((item) => item.id === hoverPreview.sid);
        if (!session) return null;
        const title = displayTitle(renames, session.id, session.title);
        const status = friendlyStatus(session.status);
        const workspace = session.workspace || "";
        const branch = workspace ? branchByWorkspace[workspace] : "";
        const when = relTime(sessionDate(session.id));
        return (
          <div className="task-preview" style={{ top: hoverPreview.top }} aria-hidden="true">
            <div className="task-preview-head"><b>{title}</b>{when && <span>{when}</span>}</div>
            <div><Folder size={15} /><span>{projectLabel(workspace)}</span></div>
            <div><GitBranch size={15} /><span>{branch || "Local"}</span></div>
            <div><span className={`status-dot ${status.cls}`} /><span>{status.text}</span></div>
          </div>
        );
      })()}

      {ctx?.kind === "session" && (
        <ContextMenu x={ctx.x} y={ctx.y} onClose={() => setCtx(null)}>
          <MenuLabel>{displayTitle(renames, ctx.sid, sessions.find((session) => session.id === ctx.sid)?.title)}</MenuLabel>
          <MenuItem onClick={() => togglePin(ctx.sid)}>{pinned.includes(ctx.sid) ? "Unpin" : "Pin"}</MenuItem>
          <MenuItem onClick={() => useStore.getState().openModal({ kind: "rename", sid: ctx.sid })}>Rename…</MenuItem>
          <MenuItem onClick={() => unread.includes(ctx.sid) ? markRead(ctx.sid) : markUnread(ctx.sid)}>{unread.includes(ctx.sid) ? "Mark as read" : "Mark as unread"}</MenuItem>
          <MenuItem onClick={() => toggleArchive(ctx.sid)}>{archived.includes(ctx.sid) ? "Unarchive" : "Archive"}</MenuItem>
          <MenuLabel>Copy</MenuLabel>
          <MenuItem onClick={() => { copyText(ctx.sid); toast("copied session id", "info"); }}>Session ID</MenuItem>
          <MenuItem onClick={() => { copyText(`${location.origin}/#${ctx.sid}`); toast("copied link", "info"); }}>Task link</MenuItem>
        </ContextMenu>
      )}
      {ctx?.kind === "project" && (() => {
        const overlay = projects[ctx.key];
        const lastOpened = overlay?.lastOpened ? relTime(new Date(overlay.lastOpened)) : "";
        const menuKey = ctx.key;
        const menuWorkspace = ctx.workspace;
        return (
          <ContextMenu x={ctx.x} y={ctx.y} onClose={() => setCtx(null)}>
            <MenuLabel>{ctx.label}{lastOpened ? ` · opened ${lastOpened} ago` : ""}</MenuLabel>
            {menuWorkspace && (
              <>
                <MenuLabel>Open in</MenuLabel>
                <MenuItem onClick={() => openProjectIn(menuWorkspace, "vscode")}>
                  <span className="inline-flex items-center gap-[8px]"><Code size={14} /> VS Code</span>
                </MenuItem>
                <MenuItem onClick={() => openProjectIn(menuWorkspace, "finder")}>
                  <span className="inline-flex items-center gap-[8px]"><FolderOpen size={14} /> Finder</span>
                </MenuItem>
                <MenuItem onClick={() => openProjectIn(menuWorkspace, "terminal")}>
                  <span className="inline-flex items-center gap-[8px]"><Terminal size={14} /> Terminal</span>
                </MenuItem>
              </>
            )}
            <MenuLabel>Project</MenuLabel>
            <MenuItem onClick={() => openPrompt({
              title: "Rename project",
              label: "Display name",
              initial: overlay?.displayName || "",
              placeholder: ctx.label,
              submitLabel: "Rename",
              onSubmit: (value) => setProjectName(menuKey, value),
            })}>
              <span className="inline-flex items-center gap-[8px]"><PencilSimple size={14} /> Rename project…</span>
            </MenuItem>
            {overlay?.displayName && <MenuItem onClick={() => setProjectName(menuKey, "")}>Reset to default name</MenuItem>}
            {menuWorkspace && <MenuItem onClick={() => { copyText(menuWorkspace); toast("copied project path", "info"); }}>Copy project path</MenuItem>}
            <MenuItem onClick={() => ctx.ids.filter((id) => unread.includes(id)).forEach(markRead)}>Mark all as read</MenuItem>
            <MenuItem onClick={() => ctx.ids.filter((id) => !archived.includes(id)).forEach(toggleArchive)}>Archive all tasks</MenuItem>
          </ContextMenu>
        );
      })()}
    </aside>
  );
}
