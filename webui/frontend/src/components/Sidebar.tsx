import { useEffect, useMemo, useState } from "react";
import {
  Archive as ArchiveBox,
  ArrowSquareOut,
  CalendarDots,
  CaretDown,
  CaretRight,
  Folder,
  FolderOpen,
  GitBranch,
  MagnifyingGlass,
  Monitor,
  Moon,
  NotePencil,
  PushPin,
  Robot,
  SidebarSimple,
  Sun,
  X,
} from "@phosphor-icons/react";
import { useStore } from "../store";
import { AR } from "../api";
import { friendlyStatus } from "./pill";
import { displayTitle } from "../title";
import { ContextMenu } from "./ContextMenu";
import { MenuItem, MenuLabel } from "./Menu";
import { copyText } from "../clipboard";
import { buildSidebarModel, projectLabel } from "../viewModels";
import { relTime, sessionDate } from "../time";

type SidebarContext =
  | { kind: "session"; x: number; y: number; sid: string }
  | { kind: "project"; x: number; y: number; label: string; workspace?: string; ids: string[] };

export function Sidebar({ onHide, onNavigate }: { onHide?: () => void; onNavigate?: () => void }) {
  const {
    health,
    sessions,
    sessionsReady,
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
    const openContext = (x: number, y: number) => setCtx({ kind: "session", x, y, sid: session.id });
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
          {when && <span className="task-when">{when}</span>}
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
      <div className="brand">
        <button className="brand-main" onClick={() => { showPage("home"); onNavigate?.(); }} aria-label="AgentRunner home">
          <span className="brand-mark"><Robot size={17} weight="bold" /></span>
          <span className="brand-name">AgentRunner</span>
        </button>
        <div className="brand-actions">
          <button className="sidebar-action" onClick={() => setSearching((value) => !value)} title="Search tasks">
            <MagnifyingGlass size={16} />
          </button>
          <button className="sidebar-action" onClick={onHide || toggleSidebar} title="Hide sidebar (⌘B)">
            <SidebarSimple size={16} />
          </button>
        </div>
      </div>

      <nav className="primary-nav" aria-label="Primary">
        <button className={!currentSid && currentPage === "home" ? "active" : ""} onClick={() => { showPage("home"); onNavigate?.(); }}>
          <NotePencil size={17} /> <span>New task</span>
        </button>
        <button className={!currentSid && currentPage === "scheduled" ? "active" : ""} onClick={() => { showPage("scheduled"); onNavigate?.(); }}>
          <CalendarDots size={17} /> <span>Scheduled</span>
          {runningRuns > 0 && <span className="nav-notice" title={`${runningRuns} running`} />}
        </button>
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
          <section className="sidebar-section">
            <div className="section-label">Pinned</div>
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
            <div className="sidebar-empty">{query ? "No matching tasks" : "No tasks yet"}</div>
          ) : null}
          {model.projects.map((project) => {
            const isExpanded = expanded.has(project.key) || !!query;
            const shown = isExpanded ? project.sessions : project.sessions.slice(0, 6);
            return (
              <div className="project-group" key={project.key}>
                <button
                  className="project-heading"
                  onClick={() => setExpanded((current) => {
                    const next = new Set(current);
                    next.has(project.key) ? next.delete(project.key) : next.add(project.key);
                    return next;
                  })}
                  title={project.workspace}
                  onContextMenu={(event) => {
                    event.preventDefault();
                    setCtx({ kind: "project", x: event.clientX, y: event.clientY, label: project.label, workspace: project.workspace, ids: project.sessions.map((session) => session.id) });
                  }}
                  onKeyDown={(event) => {
                    if (!((event.shiftKey && event.key === "F10") || event.key === "ContextMenu")) return;
                    event.preventDefault();
                    const rect = event.currentTarget.getBoundingClientRect();
                    setCtx({ kind: "project", x: rect.left + 20, y: rect.bottom, label: project.label, workspace: project.workspace, ids: project.sessions.map((session) => session.id) });
                  }}
                >
                  {isExpanded ? <CaretDown size={12} /> : <CaretRight size={12} />}
                  {isExpanded ? <FolderOpen size={16} /> : <Folder size={16} />}
                  <span>{project.label}</span>
                  {project.hint && <span className="project-hint">{project.hint}</span>}
                </button>
                {shown.map((session) => renderTask(session, true))}
                {!isExpanded && project.sessions.length > shown.length && (
                  <button className="show-more" onClick={() => setExpanded((current) => new Set(current).add(project.key))}>
                    Show {project.sessions.length - shown.length} more
                  </button>
                )}
                {isExpanded && !query && project.sessions.length > 6 && (
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
          {archivedCount > 0 && (
            <button className="archive-toggle" onClick={toggleShowArchived}>
              <ArchiveBox size={14} /> {showArchived ? "Hide" : "Show"} archived · {archivedCount}
            </button>
          )}
        </section>
      </div>

      <div className="side-foot">
        <span className={`daemon-indicator${health?.daemonUp ? " online" : ""}`} />
        <button className="daemon-copy" onClick={() => !health?.daemonUp && restartDaemon()} title={health?.daemonUp ? health.version : "Restart daemon"}>
          <span>
            {health?.daemonUp
              ? `Connected · ${(health.version || "").replace(/^agentrunner\s*/, "").split(" ")[0] || "daemon"}`
              : "Daemon unavailable — click to restart"}
          </span>
        </button>
        <button className="sidebar-action" onClick={cycleTheme} title={`Theme: ${theme}`}>{themeGlyph}</button>
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
      {ctx?.kind === "project" && (
        <ContextMenu x={ctx.x} y={ctx.y} onClose={() => setCtx(null)}>
          <MenuLabel>{ctx.label}</MenuLabel>
          {ctx.workspace && <MenuItem onClick={() => { copyText(ctx.workspace!); toast("copied project path", "info"); }}>Copy project path</MenuItem>}
          <MenuItem onClick={() => ctx.ids.filter((id) => unread.includes(id)).forEach(markRead)}>Mark all as read</MenuItem>
          <MenuItem onClick={() => ctx.ids.filter((id) => !archived.includes(id)).forEach(toggleArchive)}>Archive all tasks</MenuItem>
        </ContextMenu>
      )}
    </aside>
  );
}
