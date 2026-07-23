import { useEffect, useMemo, useState, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent } from "react";
import {
  Archive as ArchiveBox,
  ArrowsOutSimple,
  CaretRight,
  ChatCircle,
  CircleNotch,
  Clock,
  DotsThree,
  EnvelopeSimple,
  EnvelopeSimpleOpen,
  Folder,
  FolderOpen,
  GearSix,
  GitBranch,
  GitFork,
  type Icon,
  MagnifyingGlass,
  Monitor,
  Moon,
  NotePencil,
  PencilSimple,
  PushPin,
  Question,
  Sun,
  Tray,
  X,
} from "@phosphor-icons/react";
import {
  SIDEBAR_DEFAULT_WIDTH,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_WIDTH,
  useAppStoreApi,
  useStore,
  type Page,
} from "../store";
import { useAppServices } from "../app/appServices";
import { sessionFriendlyStatus } from "./pill";
import { displayTitle } from "../title";
import { ContextMenu } from "./ContextMenu";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import { buildSidebarModel, isManagedWorktreeWorkspace, projectDisplayName, projectLabel, scheduledUnread, sessionUpdatedDate, visibleProjectSessions } from "../viewModels";
import { PROJECT_GROUP_LIMIT, visibleProjectGroups } from "../viewModels.nav";
import { relTimeAgo } from "../time";
import { keyLabel } from "../shortcuts";

type SidebarContext =
  | { kind: "session"; x: number; y: number; sid: string }
  | { kind: "project"; x: number; y: number; key: string; label: string; workspace?: string; ids: string[] };

type SidebarHover =
  | { kind: "session"; sid: string; top: number }
  | { kind: "project"; key: string; top: number };

// SB-4 · Collapsed project groups, mirrored into localStorage.
//
// The server overlay (INC-53 `projects[key].folded`) remains the source of
// truth once it lands, but it arrives one round-trip after mount — so on every
// cold load the rail painted every group open before snapping shut. The local
// mirror makes the fold survive a refresh *synchronously*; the overlay wins
// whenever it actually carries a fold for that key.
const COLLAPSED_KEY = "ar.sidebar.collapsedProjects";
const SECTION_FOLDS_KEY = "ar.sidebar.foldedSections";
type FoldableSection = "pinned" | "projects";

function loadCollapsedProjects(storage: Storage): Set<string> {
  try {
    const raw = JSON.parse(storage.getItem(COLLAPSED_KEY) || "[]");
    return new Set(Array.isArray(raw) ? raw.filter((key): key is string => typeof key === "string") : []);
  } catch {
    return new Set();
  }
}

function loadFoldedSections(storage: Storage): Set<FoldableSection> {
  try {
    const raw = JSON.parse(storage.getItem(SECTION_FOLDS_KEY) || "[]");
    return new Set(
      Array.isArray(raw)
        ? raw.filter((section): section is FoldableSection => section === "pinned" || section === "projects")
        : [],
    );
  } catch {
    return new Set();
  }
}

// Primary-nav destinations (New session / Scheduled). Kept as a small table
// rendered in a map so adding a destination is one row here + a page dispatch
// in App.tsx — no per-button JSX duplication. The Scheduled row alone carries
// the live activity dot, keyed off `key === "scheduled"`.
// `keys` is the row's resting shortcut badge (Codex parity, RH-4): tokens from
// shortcuts.ts, so the badge and the Settings → Keyboard shortcuts table can
// never disagree about what the app binds.
const NAV_DESTINATIONS: { key: Page; label: string; icon: Icon; keys?: string[] }[] = [
  { key: "home", label: "New session", icon: NotePencil, keys: ["mod", "alt", "N"] },
  { key: "scheduled", label: "Scheduled", icon: Clock },
];

export function Sidebar({ onHide, onNavigate, onOpenPalette, onOpenSettings }: {
  onHide?: () => void;
  onNavigate?: () => void;
  onOpenPalette?: () => void;
  onOpenSettings?: () => void;
}) {
  const { api, clock, storage } = useAppServices();
  const store = useAppStoreApi();
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
    unread,
    markUnread,
    markRead,
    openHelp,
    projects,
    toggleProjectFolded,
    toggleProjectPinned,
    setProjectRemoved,
    openProjectIn,
    setProjectName,
    newSessionForProject,
    openModal,
    openPrompt,
    sidebarWidth,
    setSidebarWidth,
  } = useStore();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  // SB-4: locally-collapsed groups (localStorage-backed) + the section-level
  // "show every project" escape hatch.
  const [collapsed, setCollapsed] = useState<Set<string>>(() => loadCollapsedProjects(storage.local));
  const [showAllProjects, setShowAllProjects] = useState(false);
  const [showRemovedProjects, setShowRemovedProjects] = useState(false);
  const [foldedSections, setFoldedSections] = useState<Set<FoldableSection>>(
    () => loadFoldedSections(storage.local),
  );
  // The flat Sessions section has its own show-all toggle.
  const [showAllSessions, setShowAllSessions] = useState(false);
  const [ctx, setCtx] = useState<SidebarContext | null>(null);
  const [hoverPreview, setHoverPreview] = useState<SidebarHover | null>(null);
  const [branchByWorkspace, setBranchByWorkspace] = useState<Record<string, string>>({});

  const toggleSection = (section: FoldableSection) => {
    setFoldedSections((current) => {
      const next = new Set(current);
      if (next.has(section)) next.delete(section);
      else next.add(section);
      try {
        storage.local.setItem(SECTION_FOLDS_KEY, JSON.stringify([...next]));
      } catch {
        /* private mode / quota */
      }
      return next;
    });
  };

  const startSidebarResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = sidebarWidth;
    document.body.classList.add("sidebar-resizing");
    const move = (moveEvent: PointerEvent) => setSidebarWidth(startWidth + moveEvent.clientX - startX);
    const stop = () => {
      document.body.classList.remove("sidebar-resizing");
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
      window.removeEventListener("pointercancel", stop);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop);
    window.addEventListener("pointercancel", stop);
  };

  const resizeWithKeyboard = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    let next: number | null = null;
    if (event.key === "ArrowLeft") next = sidebarWidth - 16;
    else if (event.key === "ArrowRight") next = sidebarWidth + 16;
    else if (event.key === "Home") next = SIDEBAR_MIN_WIDTH;
    else if (event.key === "End") next = SIDEBAR_MAX_WIDTH;
    if (next === null) return;
    event.preventDefault();
    setSidebarWidth(next);
  };

  // RH-5: the sidebar no longer filters itself. Search is the ⌘K palette —
  // one entry point, reachable from the magnifier or the key — so this model is
  // always the unfiltered list (the `query` knob stays in buildSidebarModel for
  // Settings → Archived, which does search).
  const model = useMemo(
    () => buildSidebarModel(sessions, {
      pinned,
      archived,
      showArchived,
      query: "",
      titleOf: (session) => displayTitle(renames, session.id, session.title),
    }),
    [sessions, pinned, archived, showArchived, renames],
  );
  const archivedCount = sessions.filter((session) => archived.includes(session.id)).length;
  const runningRuns = runs.filter((run) => run.status === "running").length;
  const schedUnread = scheduledUnread(sessions, unread);
  // Pinning is a stable presentation sort within Projects. Removed projects
  // stay in the journal-derived model (and in search) but leave this rail until
  // the explicit recovery row reveals them.
  const orderedProjects = useMemo(() => {
    const visible = model.projects.filter((project) => showRemovedProjects || !projects[project.key]?.removed);
    return [...visible].sort(
      (a, b) => Number(!!projects[b.key]?.pinned) - Number(!!projects[a.key]?.pinned),
    );
  }, [model.projects, projects, showRemovedProjects]);
  const removedProjectCount = model.projects.filter((project) => projects[project.key]?.removed).length;
  const orderedIds = useMemo(
    () => [
      ...(foldedSections.has("pinned") ? [] : model.pinned.map((session) => session.id)),
      ...(foldedSections.has("projects") ? [] : orderedProjects.flatMap((project) => project.sessions.map((session) => session.id))),
      // The flat Sessions section is part of the rail, so it is part of the
      // rail's keyboard order too — it sits last, exactly where it renders.
      ...model.workspaceLessSessions.map((session) => session.id),
    ],
    [foldedSections, model.pinned, model.workspaceLessSessions, orderedProjects],
  );
  useEffect(() => setVisibleOrder(orderedIds), [orderedIds, setVisibleOrder]);

  // SB-1: bring the current row into the rail's viewport whenever the selection
  // changes (deep link, ⌘K jump, cold refresh) — the row can sit thousands of
  // pixels below a `.project-list` that never scrolls itself. `block: "nearest"`
  // is deliberate: a row already on screen stays put, so this never yanks the
  // list out from under a scrolling user. Deferred a frame because on the first
  // paint the row may not exist yet (sessions arrive after mount).
  useEffect(() => {
    if (!currentSid) return;
    const frame = requestAnimationFrame(() => {
      const row = document.querySelector<HTMLElement>(".project-session-wrap.current");
      row?.scrollIntoView?.({ block: "nearest" });
    });
    return () => cancelAnimationFrame(frame);
  }, [currentSid, sessionsReady, orderedIds]);

  // SB-4: the Projects section renders the 8 most recent groups (plus, always,
  // the group holding the open session) — the rest hide behind Show more.
  const { groups: shownProjects, hidden: hiddenProjects } = useMemo(
    () => visibleProjectGroups(orderedProjects, { expanded: showAllProjects, current: currentSid || undefined }),
    [orderedProjects, showAllProjects, currentSid],
  );

  // Workspace-less sessions use a plain heading with no folder, caret or indent.
  // Capping reuses visibleProjectSessions so the current session remains visible.
  const shownSessions = useMemo(
    () => visibleProjectSessions(
      { key: "__sessions__", label: "Sessions", sessions: model.workspaceLessSessions },
      { expanded: showAllSessions, current: currentSid || undefined },
    ),
    [model.workspaceLessSessions, showAllSessions, currentSid],
  );

  // Fold a group both locally (instant, survives refresh) and in the server
  // overlay (shared with the other surfaces that read `projects[key].folded`).
  const setProjectCollapsed = (key: string, next: boolean) => {
    setCollapsed((current) => {
      const updated = new Set(current);
      if (next) updated.add(key);
      else updated.delete(key);
      try {
        storage.local.setItem(COLLAPSED_KEY, JSON.stringify([...updated]));
      } catch {
        /* private mode / quota — the overlay still carries the fold */
      }
      return updated;
    });
    void toggleProjectFolded(key, next);
  };

  const restartDaemon = async () => {
    try {
      await api.daemonStart();
      toast("daemon start requested", "info");
      clock.setTimeout(refreshHealth, 800);
    } catch (error: any) {
      toast(error.message);
    }
  };

  const previewSession = (session: (typeof sessions)[number], top: number) => {
    // The hover preview and the right-click context menu are mutually
    // exclusive floating layers — while a menu is open, suppress the preview
    // so the two never stack and fight for the same corner (R3-1).
    if (ctx) return;
    setHoverPreview({ kind: "session", sid: session.id, top: Math.max(10, Math.min(top - 6, window.innerHeight - 154)) });
    const workspace = session.workspace;
    if (!workspace || Object.prototype.hasOwnProperty.call(branchByWorkspace, workspace)) return;
    setBranchByWorkspace((current) => ({ ...current, [workspace]: "" }));
    api.gitBranches(workspace)
      .then((info) => setBranchByWorkspace((current) => ({
        ...current,
        [workspace]: info.isRepo && info.current ? info.current : "Local workspace",
      })))
      .catch(() => setBranchByWorkspace((current) => ({ ...current, [workspace]: "Local workspace" })));
  };

  const renderSessionActions = (sid: string) => (
    <>
      <MenuLabel>{displayTitle(renames, sid, sessions.find((session) => session.id === sid)?.title)}</MenuLabel>
      <MenuItem onClick={() => togglePin(sid)}>
        <PushPin size={16} weight={pinned.includes(sid) ? "fill" : "regular"} /> {pinned.includes(sid) ? "Unpin" : "Pin"}
      </MenuItem>
      <MenuItem onClick={() => store.getState().openModal({ kind: "rename", sid })}>
        <PencilSimple size={16} /> Rename…
      </MenuItem>
      <MenuItem onClick={() => unread.includes(sid) ? markRead(sid) : markUnread(sid)}>
        {unread.includes(sid) ? <EnvelopeSimpleOpen size={16} /> : <EnvelopeSimple size={16} />}
        {unread.includes(sid) ? "Mark as read" : "Mark as unread"}
      </MenuItem>
      <MenuItem onClick={() => toggleArchive(sid)}>
        <ArchiveBox size={16} /> {archived.includes(sid) ? "Unarchive" : "Archive"}
      </MenuItem>
    </>
  );

  // One source for the project group's actions: the desktop right-click
  // ContextMenu and the touch ⋯ Menu on the heading row (INC-87.2) render the
  // same items, so the two entrances can never drift apart.
  const renderProjectActions = (key: string, label: string, workspace: string | undefined, ids: string[]) => {
    const overlay = projects[key];
    return (
      <>
        <MenuItem onClick={() => void toggleProjectPinned(key, !overlay?.pinned)}>
          <PushPin size={16} weight={overlay?.pinned ? "fill" : "regular"} /> {overlay?.pinned ? "Unpin project" : "Pin project"}
        </MenuItem>
        {workspace && (
          <MenuItem onClick={() => openProjectIn(workspace, "finder")}>
            <FolderOpen size={16} /> Reveal in Finder
          </MenuItem>
        )}
        {workspace && (
          <MenuItem onClick={() => openPrompt({
            title: "Create permanent worktree",
            label: "New branch name",
            placeholder: "feature/my-work",
            submitLabel: "Create",
            onSubmit: (branch) => {
              void api.makeWorktree(workspace, branch.trim())
                .then((result) => toast(`worktree created · ${result.path}`, "info"))
                .catch((error: any) => toast(error.message, "error", error.details));
            },
          })}>
            <GitFork size={16} /> Create permanent worktree
          </MenuItem>
        )}
        <MenuItem onClick={() => openPrompt({
          title: "Rename project",
          label: "Display name",
          initial: overlay?.displayName || "",
          placeholder: label,
          submitLabel: "Rename",
          onSubmit: (value) => setProjectName(key, value),
        })}>
          <PencilSimple size={16} /> Rename project
        </MenuItem>
        <MenuItem onClick={() => ids.filter((id) => !archived.includes(id)).forEach(toggleArchive)}>
          <ArchiveBox size={16} /> Archive chats
        </MenuItem>
        <MenuItem
          danger={!overlay?.removed}
          onClick={() => {
            if (overlay?.removed) {
              void setProjectRemoved(key, false);
              return;
            }
            openModal({
              kind: "confirm",
              title: "Remove project from sidebar?",
              body: `${label} will be hidden from Projects. Its chats, journal, and files stay intact, and you can restore it from Show removed projects.`,
              confirmLabel: "Remove",
              danger: true,
              onConfirm: () => setProjectRemoved(key, true),
            });
          }}
        >
          <X size={16} /> {overlay?.removed ? "Restore project" : "Remove"}
        </MenuItem>
      </>
    );
  };

  const renderSession = (session: (typeof sessions)[number], nested = false) => {
    const active = session.id === currentSid;
    const status = sessionFriendlyStatus(session);
    const isUnread = unread.includes(session.id);
    const title = displayTitle(renames, session.id, session.title);
    const when = relTimeAgo(sessionUpdatedDate(session));
    const isRunning = status.cls === "run";
    const isWorktree = isManagedWorktreeWorkspace(session.workspace);
    const actionCount =
      (session.attention?.approvals || 0) +
      (session.attention?.answers || 0);
    const openContext = (x: number, y: number) => {
      // Opening a context menu instantly dismisses any hover preview so the
      // two floating layers stay mutually exclusive (R3-1).
      setHoverPreview(null);
      setCtx({ kind: "session", x, y, sid: session.id });
    };
    return (
      <div
        key={session.id}
        className={`project-session-wrap${nested ? " nested" : ""}${active ? " current" : ""}${isUnread ? " unread" : ""}${archived.includes(session.id) ? " archived" : ""}`}
        onContextMenu={(event) => {
          event.preventDefault();
          openContext(event.clientX, event.clientY);
        }}
        onMouseEnter={(event) => previewSession(session, event.currentTarget.getBoundingClientRect().top)}
        onMouseLeave={() => setHoverPreview((current) => current?.kind === "session" && current.sid === session.id ? null : current)}
      >
        <button
          className="project-session"
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
          title={`${session.title || title}\n${status.text}${when ? ` · started ${when}` : ""}\n${session.id}`}
          aria-label={`${title} · ${isUnread && status.cls !== "appr" ? "New activity" : status.text}${when ? ` · ${when}` : ""}`}
        >
          <span className="project-session-title">{title}</span>
          {(isUnread || ["appr", "stranded", "crash"].includes(status.cls)) && (
            actionCount > 1 && status.cls === "appr"
              ? <span className="status-count" title={status.text} aria-hidden="true">{actionCount}</span>
              : <span className={`status-dot ${isUnread && status.cls !== "appr" ? "unread" : status.cls}`} title={isUnread && status.cls !== "appr" ? "New activity" : status.text} />
          )}
        </button>
        {(isWorktree || isRunning) && (
          <span className={`session-state-icons${isRunning ? " running" : ""}`}>
            {isWorktree && (
              <span className="session-worktree-icon" title="Worktree session" aria-label="Worktree session">
                <ArrowsOutSimple size={17} />
              </span>
            )}
            {isRunning && (
              <CircleNotch className="session-loading-icon" size={17} role="status" aria-label="Session running" />
            )}
          </span>
        )}
        {/* The row's complete menu already exists on right-click/Shift+F10 and
            in the open session title. Hover/focus therefore spends its two quiet
            slots on the frequent reversible actions instead of another `…`. */}
        <span
          className="session-quick-actions"
          onMouseEnter={() => setHoverPreview(null)}
        >
          <button
            className="session-quick-action"
            aria-label={`${pinned.includes(session.id) ? "Unpin" : "Pin"} ${title}`}
            title={pinned.includes(session.id) ? "Unpin" : "Pin"}
            onClick={() => togglePin(session.id)}
          >
            <PushPin size={17} weight={pinned.includes(session.id) ? "fill" : "regular"} />
          </button>
          <button
            className="session-quick-action"
            aria-label={`${archived.includes(session.id) ? "Unarchive" : "Archive"} ${title}`}
            title={archived.includes(session.id) ? "Unarchive" : "Archive"}
            onClick={() => toggleArchive(session.id)}
          >
            <ArchiveBox size={17} />
          </button>
        </span>
      </div>
    );
  };

  const themeGlyph = theme === "system" ? <Monitor size={15} /> : theme === "light" ? <Sun size={15} /> : <Moon size={15} />;

  return (
    <aside className="sidebar">
      <div
        className="sidebar-resize-handle max-[900px]:hidden!"
        role="separator"
        aria-label="Resize sidebar"
        aria-orientation="vertical"
        aria-valuemin={SIDEBAR_MIN_WIDTH}
        aria-valuemax={SIDEBAR_MAX_WIDTH}
        aria-valuenow={sidebarWidth}
        tabIndex={0}
        title="Drag to resize sidebar · double-click to reset"
        onPointerDown={startSidebarResize}
        onKeyDown={resizeWithKeyboard}
        onDoubleClick={() => setSidebarWidth(SIDEBAR_DEFAULT_WIDTH)}
      />
      {/* SB-10: 64px of chrome around a 30px wordmark cost a whole session row of
          rail. 6px above/below a 30px content row → a 44px well (Codex ~38px).
          SB-13: the 26px black rounded tile that used to sit here was the
          darkest block on the whole screen — maximum ink spent on a decoration
          that navigates nowhere new (the wordmark next to it already goes
          home). Codex's rail opens with a plain "ChatGPT Codex" wordmark and
          nothing else. Same here: text only, so the first thing the eye lands
          on is a session, not a logo. */}
      <div className="flex items-center justify-between min-h-[44px] pt-[6px] pr-[14px] pb-[6px] pl-[16px] max-[900px]:pt-0! max-[900px]:pb-0!">
        <button className="brand-main" onClick={() => { showPage("home"); onNavigate?.(); }} aria-label="AgentRunner home">
          <span className="text-[16px] font-[650] tracking-[-0.2px]">AgentRunner</span>
        </button>
        <div className="flex items-center gap-[2px]">
          <button
            className="w-[30px] h-[30px] grid place-items-center p-0 border-0 bg-transparent text-ink-2 rounded-[8px] hover:text-ink hover:bg-[color-mix(in_srgb,var(--ink)_6%,transparent)] max-[900px]:w-[44px]! max-[900px]:h-[44px]!"
            onClick={onOpenPalette}
            title={`Search sessions (${keyLabel("mod")}K)`}
            aria-label="Search sessions"
          >
            <MagnifyingGlass size={16} />
          </button>
          <button className="sidebar-close max-[900px]:w-[44px]! max-[900px]:h-[44px]!" onClick={onHide} title="Close sidebar" aria-label="Close sidebar">
            <X size={17} />
          </button>
        </div>
      </div>

      <nav className="primary-nav" aria-label="Primary">
        {NAV_DESTINATIONS.map(({ key, label, icon: DestIcon, keys }) => (
          <button
            key={key}
            className={!currentSid && currentPage === key && key !== "home" ? "active" : ""}
            onClick={() => { showPage(key); onNavigate?.(); }}
            title={keys ? `${label} (${keys.map(keyLabel).join("")})` : label}
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

      <div className="project-list">
        {model.pinned.length > 0 && (
          <section className="sidebar-section pinned-section">
            <button
              className="section-label section-toggle"
              onClick={() => toggleSection("pinned")}
              aria-expanded={!foldedSections.has("pinned")}
            >
              <CaretRight className={`section-caret${foldedSections.has("pinned") ? "" : " open"}`} size={10} weight="bold" />
              <PushPin size={12} weight="fill" /> Pinned
            </button>
            {!foldedSections.has("pinned") && model.pinned.map((session) => renderSession(session))}
          </section>
        )}

        {/* Loading and empty are rail-level states, not Projects-level ones:
            with SB-13 the rail has three sections, so "nothing here" means all
            three are empty — and a rail that *does* hold sessions must never paint
            "No sessions yet" under a heading. */}
        {!sessionsReady ? (
          <div className="sidebar-loading" role="status" aria-label="Loading sessions">
            <span />
            <span />
            <span />
          </div>
        ) : model.projects.length === 0 && model.workspaceLessSessions.length === 0 && model.pinned.length === 0 ? (
          <div className="sidebar-empty">
            <Tray size={22} />
            <b>No sessions yet</b>
            <span>Start a session to see it here.</span>
          </div>
        ) : null}

        {model.projects.length > 0 && (
        <section className="sidebar-section projects-section">
          <button
            className="section-label section-toggle"
            onClick={() => toggleSection("projects")}
            aria-expanded={!foldedSections.has("projects")}
          >
            <CaretRight className={`section-caret${foldedSections.has("projects") ? "" : " open"}`} size={10} weight="bold" />
            Projects
          </button>
          {!foldedSections.has("projects") && (<>
          {shownProjects.map((project) => {
            const overlay = projects[project.key];
            const name = projectDisplayName(project, overlay);
            // Persisted fold collapses the group entirely; the local `expanded`
            // set is the secondary show-all-vs-6 control within an unfolded
            // group. (Search no longer lives here — it is the ⌘K palette, RH-5.)
            // SB-4: the fold reads from the server overlay when it has one for
            // this key, else from the localStorage mirror (which is what the
            // very first paint has to go on).
            // INC-90: selection may keep this project heading inside the capped
            // section, but it never overrides the user's explicit fold. A folded
            // group hides every session row, including the current one.
            const persistedFold = overlay?.folded ?? collapsed.has(project.key);
            const folded = persistedFold;
            const showAll = expanded.has(project.key);
            const shown = visibleProjectSessions(project, { folded, expanded: showAll, current: currentSid || undefined });
            const openMenu = (x: number, y: number) => {
              setHoverPreview(null);
              setCtx({ kind: "project", x, y, key: project.key, label: name, workspace: project.workspace, ids: project.sessions.map((session) => session.id) });
            };
            return (
              <div className="project-group" key={project.key}>
                <div
                  className="project-heading-row"
                  onMouseEnter={(event) => {
                    if (ctx) return;
                    const top = event.currentTarget.getBoundingClientRect().top;
                    setHoverPreview({ kind: "project", key: project.key, top: Math.max(10, Math.min(top - 6, window.innerHeight - 132)) });
                  }}
                  onMouseLeave={() => setHoverPreview((current) => current?.kind === "project" && current.key === project.key ? null : current)}
                >
                <button
                  className="project-heading min-w-0 flex-1"
                  onClick={() => setProjectCollapsed(project.key, !persistedFold)}
                  title={project.workspace || name}
                  aria-expanded={!folded}
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
                  {/* SB-4 · the caret is the affordance: the heading is a
                      collapse control, and Codex's group rows say so.
                      SB-6 · caret and folder share one absolutely-positioned
                      slot in the left gutter (see tw.css) so the group
                      name's text lands on the same column as the session titles
                      nested under it; the folder rests there and the caret
                      takes the slot on hover/focus. */}
                  <span className="proj-icon-slot">
                    <CaretRight className={`proj-caret${!folded ? " open" : ""}`} size={11} weight="bold" aria-hidden="true" />
                    {/* SIDEBAR-FOLDER-ICON · the group icon is always a closed
                        Folder, matching Codex's gold (codex-crop-sidebar-projects):
                        every project group — expanded or not — keeps the same
                        closed folder so the icon column stays quiet and uniform.
                        Expanded state is encoded solely by the rotating caret
                        (.proj-caret.open above), not by swapping to FolderOpen. */}
                    <Folder className="proj-folder" size={16} />
                  </span>
                  {/* Duplicate project labels intentionally remain identical.
                      The full workspace already lives in this button's title
                      and hover preview, so a resting path subtitle would spend
                      a second line on information that is only occasionally
                      needed. */}
                  <span className="proj-heading-text">
                    <span className="proj-heading-name">{name}</span>
                  </span>
                </button>
                {/* Desktop reveals the two quiet controls on row hover/focus;
                    touch keeps the menu permanently reachable. The pencil is
                    New chat for this project; Rename stays in the menu. */}
                <span className="project-heading-actions" onClick={() => setHoverPreview(null)}>
                  <Menu
                    label={<DotsThree size={18} weight="bold" />}
                    ariaLabel={`More actions for ${name}`}
                  >
                    {renderProjectActions(project.key, name, project.workspace, project.sessions.map((session) => session.id))}
                  </Menu>
                  <button
                    className="project-quick-action max-[900px]:hidden!"
                    aria-label={`New chat in ${name}`}
                    title="New chat"
                    onClick={() => {
                      if (!project.workspace) return;
                      newSessionForProject(project.workspace);
                      onNavigate?.();
                    }}
                  >
                    <PencilSimple size={16} />
                  </button>
                </span>
                </div>
                {shown.map((session) => renderSession(session, true))}
                {!folded && !showAll && project.sessions.length > shown.length && (
                  <button className="show-more" onClick={() => setExpanded((current) => new Set(current).add(project.key))}>
                    Show more
                  </button>
                )}
                {!folded && showAll && project.sessions.length > 6 && (
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
          {/* SB-4 · section-level Show more. Same row language as the per-group
              one (`.show-more`), one level out: it governs how many *projects*
              the section renders, not how many sessions a project renders. */}
          {hiddenProjects > 0 && (
            <button
              className="show-more projects-show-more"
              onClick={() => setShowAllProjects(true)}
              aria-label={`Show all ${orderedProjects.length} projects`}
            >
              Show more · {hiddenProjects}
            </button>
          )}
          {showAllProjects && orderedProjects.length > PROJECT_GROUP_LIMIT && (
            <button
              className="show-more projects-show-more"
              onClick={() => setShowAllProjects(false)}
              aria-label={`Show only the ${PROJECT_GROUP_LIMIT} most recent projects`}
            >
              Show less
            </button>
          )}
          </>)}
        </section>
        )}

        {removedProjectCount > 0 && (
          <button
            className="archive-toggle removed-projects-toggle"
            onClick={() => setShowRemovedProjects((showing) => !showing)}
            aria-expanded={showRemovedProjects}
          >
            <X size={14} /> {showRemovedProjects ? "Hide" : "Show"} removed projects · {removedProjectCount}
          </button>
        )}

        {/* SB-13 · Sessions — the ones that belong to no project. Flat rows at the
            Pinned indent: no folder, no caret, nothing claiming a directory
            these sessions do not have. Renders only when it has something to say;
            an empty heading is worse than no heading. */}
        {model.workspaceLessSessions.length > 0 && (
          <section className="sidebar-section sessions-section">
            <div className="section-label">Sessions</div>
            {shownSessions.map((session) => renderSession(session))}
            {!showAllSessions && model.workspaceLessSessions.length > shownSessions.length && (
              <button className="show-more" onClick={() => setShowAllSessions(true)}>
                Show more · {model.workspaceLessSessions.length - shownSessions.length}
              </button>
            )}
            {showAllSessions && model.workspaceLessSessions.length > 6 && (
              <button className="show-more" onClick={() => setShowAllSessions(false)}>
                Show less
              </button>
            )}
          </section>
        )}

        {sessionsLoadingOlder && (
          <div className="sidebar-history-loading" role="status">Loading older sessions…</div>
        )}
        {archivedCount > 0 && (
          <button className="archive-toggle" onClick={toggleShowArchived}>
            <ArchiveBox size={14} /> {showArchived ? "Hide" : "Show"} archived · {archivedCount}
          </button>
        )}
      </div>

      <div className="side-foot">
        {/* INC-41 L3 · Three states, not two. `health === null` means the first
            /health call hasn't answered yet — rendering that as a red "Daemon
            offline" made every cold load flash a fake outage (and armed a
            restart click). Unknown is neutral and inert; only a health record
            that actually says daemonUp:false is an outage. */}
        {health?.daemonUp === false ? (
          <button
            className="account-badge"
            onClick={restartDaemon}
            title="Daemon offline — click to restart"
            aria-label="Daemon offline — click to restart"
          >
            <span className="account-avatar offline" aria-hidden="true">
              <span className="text-[11px] font-[680] tracking-[0.4px]">AR</span>
              <span className="account-presence" />
            </span>
            <span className="account-meta"><span>Daemon offline — restart</span></span>
          </button>
        ) : (
          <div
            className="account-badge"
            role="status"
            title={!health ? "Checking daemon status…" : `Connected to daemon · ${health.version || "unknown version"}`}
            aria-label={!health ? "Connecting to daemon" : "Connected to daemon"}
          >
            <span className={`account-avatar${!health ? " connecting" : " online"}`} aria-hidden="true">
              <span className="text-[11px] font-[680] tracking-[0.4px]">AR</span>
              <span className="account-presence" />
            </span>
            <span className="account-meta"><span>{!health ? "Connecting…" : "Connected"}</span></span>
          </div>
        )}
        {/* SB-12 · Three loose icon buttons — Settings, Help, Theme — sat on the
            account row spending a third of it on chrome nobody clicks in a
            session. Codex's bottom bar is identity only: avatar, name, presence
            dot. Ours keeps the identity and folds the three into one `…` menu
            (the same Menu the session header uses), so every action survives with
            its shortcut and its title — they just stop shouting. */}
        <Menu
          label={<DotsThree size={18} weight="bold" />}
          ariaLabel="More options"
          triggerClassName="max-[900px]:w-[44px]! max-[900px]:h-[44px]!"
        >
          {onOpenSettings && (
            <MenuItem onClick={onOpenSettings} title="Settings (⌘,)">
              <GearSix size={16} /> Settings <span className="menu-kbd">{keyLabel("mod")},</span>
            </MenuItem>
          )}
          <MenuItem onClick={openHelp} title="Keyboard shortcuts & help (?)">
            <Question size={16} /> Keyboard shortcuts & help <span className="menu-kbd">?</span>
          </MenuItem>
          <MenuItem onClick={cycleTheme} title={`Theme: ${theme}`}>
            {themeGlyph} Theme: {theme}
          </MenuItem>
        </Menu>
      </div>

      {hoverPreview && (() => {
        if (hoverPreview.kind === "project") {
          const project = model.projects.find((item) => item.key === hoverPreview.key);
          if (!project) return null;
          const overlay = projects[project.key];
          const name = projectDisplayName(project, overlay);
          return (
            <div className="project-preview" style={{ top: hoverPreview.top }} aria-hidden="true">
              <div className="project-preview-head">
                <Folder size={18} />
                <b>{name}</b>
                <PushPin size={16} weight={overlay?.pinned ? "fill" : "regular"} />
              </div>
              <div><ChatCircle size={16} /><span>{project.sessions.length} {project.sessions.length === 1 ? "chat" : "chats"}</span></div>
              <div className="project-preview-path"><FolderOpen size={16} /><span>{project.workspace || "No workspace"}</span></div>
            </div>
          );
        }
        const session = sessions.find((item) => item.id === hoverPreview.sid);
        if (!session) return null;
        const title = displayTitle(renames, session.id, session.title);
        const status = sessionFriendlyStatus(session);
        const workspace = session.workspace || "";
        const branch = workspace ? branchByWorkspace[workspace] : "";
        const when = relTimeAgo(sessionUpdatedDate(session));
        return (
          <div className="session-preview" style={{ top: hoverPreview.top }} aria-hidden="true">
            <div className="session-preview-head"><b>{title}</b>{when && <span>{when}</span>}</div>
            {/* SB-13: no workspace ⇒ no project, and the preview says so
                plainly rather than inventing an "Other sessions" folder. */}
            <div><Folder size={15} /><span>{projectLabel(workspace) || "No project"}</span></div>
            <div><GitBranch size={15} /><span>{branch || "Local"}</span></div>
            <div><span className={`status-dot ${status.cls}`} /><span>{status.text}</span></div>
          </div>
        );
      })()}

      {ctx?.kind === "session" && (
        <ContextMenu x={ctx.x} y={ctx.y} onClose={() => setCtx(null)}>
          {renderSessionActions(ctx.sid)}
        </ContextMenu>
      )}
      {ctx?.kind === "project" && (
        <ContextMenu x={ctx.x} y={ctx.y} onClose={() => setCtx(null)}>
          {renderProjectActions(ctx.key, ctx.label, ctx.workspace, ctx.ids)}
        </ContextMenu>
      )}
    </aside>
  );
}
