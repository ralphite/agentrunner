import { useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent } from "react";
import {
  Archive as ArchiveBox,
  CaretRight,
  Clock,
  DotsThree,
  GearSix,
  type Icon,
  MagnifyingGlass,
  Monitor,
  Moon,
  NotePencil,
  Plus,
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
import {
  buildSidebarModel,
  isManagedWorktreeWorkspace,
  orderProjectGroups,
  projectDisplayName,
  projectNameForWorkspace,
  scheduledUnread,
  sessionUpdatedDate,
  visibleProjectSessions,
  type ProjectGroup,
  type SidebarOrganize,
  type SidebarSort,
} from "../viewModels";
import { relTimeAgo } from "../time";
import { keyLabel } from "../shortcuts";
import { Spinner } from "../ui/Spinner";
import {
  SidebarConnectionStatus,
  SidebarPreviewCard,
  SidebarProjectActions,
  SidebarProjectItem,
  SidebarSessionActions,
  SidebarSessionItem,
} from "./SidebarItems";
import { IconButton } from "../ui/IconButton";

type SidebarContext =
  | { kind: "session"; x: number; y: number; sid: string; returnFocus: HTMLElement }
  | { kind: "project"; x: number; y: number; key: string; label: string; workspace?: string; ids: string[]; returnFocus: HTMLElement };

type SidebarHover =
  | { kind: "session"; sid: string; top: number }
  | { kind: "project"; key: string; top: number };

// Grace period before a hover preview closes. The card's left edge meets the
// row's right edge, so a straight move onto it hands over within a frame; the
// delay is for the diagonal case, where the card is clamped away from the row's
// own line and the pointer clips a few pixels of dead rail on the way.
// (It is NOT what buys travel time across the row's quick actions — hovering
// those no longer starts the countdown at all, which is what used to make the
// card unreachable.)
const HOVER_PREVIEW_CLOSE_DELAY_MS = 220;

// SB-4 · Collapsed project groups, mirrored into localStorage.
//
// The server overlay (INC-53 `projects[key].folded`) remains the source of
// truth once it lands, but it arrives one round-trip after mount — so on every
// cold load the rail painted every group open before snapping shut. The local
// mirror makes the fold survive a refresh *synchronously*; the overlay wins
// whenever it actually carries a fold for that key.
const COLLAPSED_KEY = "ar.sidebar.collapsedProjects";
const SECTION_FOLDS_KEY = "ar.sidebar.foldedSections";
// INC-104 organize menu. Per-browser presentation preferences, same family as
// the section folds above — NOT the shared registry, so two open ports can
// disagree about layout without fighting over a file.
const ORGANIZE_KEY = "ar.sidebar.organize";
const SORT_KEY = "ar.sidebar.sort";
type FoldableSection = "pinned" | "projects";

function loadChoice<T extends string>(storage: Storage, key: string, allowed: readonly T[], fallback: T): T {
  try {
    const raw = storage.getItem(key);
    return allowed.includes(raw as T) ? (raw as T) : fallback;
  } catch {
    return fallback;
  }
}

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
    projectDefs,
    deleteProject,
    toggleProjectFolded,
    toggleProjectPinned,
    setProjectRemoved,
    openProjectIn,
    newSessionForProject,
    openModal,
    openPrompt,
    sidebarWidth,
    setSidebarWidth,
  } = useStore();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  // Locally-collapsed groups (localStorage-backed).
  const [collapsed, setCollapsed] = useState<Set<string>>(() => loadCollapsedProjects(storage.local));
  const [showRemovedProjects, setShowRemovedProjects] = useState(false);
  const [foldedSections, setFoldedSections] = useState<Set<FoldableSection>>(
    () => loadFoldedSections(storage.local),
  );
  const [organize, setOrganizeState] = useState<SidebarOrganize>(
    () => loadChoice(storage.local, ORGANIZE_KEY, ["by-project", "one-list"], "by-project"),
  );
  const [sort, setSortState] = useState<SidebarSort>(
    () => loadChoice(storage.local, SORT_KEY, ["priority", "updated", "manual"], "priority"),
  );
  const setOrganize = (next: SidebarOrganize) => {
    setOrganizeState(next);
    try { storage.local.setItem(ORGANIZE_KEY, next); } catch { /* private mode / quota */ }
  };
  const setSort = (next: SidebarSort) => {
    setSortState(next);
    try { storage.local.setItem(SORT_KEY, next); } catch { /* private mode / quota */ }
  };
  // The flat Sessions section has its own show-all toggle.
  const [showAllSessions, setShowAllSessions] = useState(false);
  const [ctx, setCtx] = useState<SidebarContext | null>(null);
  const [hoverPreview, setHoverPreview] = useState<SidebarHover | null>(null);
  const [branchByWorkspace, setBranchByWorkspace] = useState<Record<string, string>>({});
  const hoverPreviewCloseTimer = useRef<number | null>(null);

  const cancelHoverPreviewClose = () => {
    if (hoverPreviewCloseTimer.current === null) return;
    window.clearTimeout(hoverPreviewCloseTimer.current);
    hoverPreviewCloseTimer.current = null;
  };

  const clearHoverPreview = () => {
    cancelHoverPreviewClose();
    setHoverPreview(null);
  };

  const scheduleHoverPreviewClose = () => {
    cancelHoverPreviewClose();
    hoverPreviewCloseTimer.current = window.setTimeout(() => {
      hoverPreviewCloseTimer.current = null;
      setHoverPreview(null);
    }, HOVER_PREVIEW_CLOSE_DELAY_MS);
  };

  useEffect(() => () => cancelHoverPreviewClose(), []);

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
      projectDefs,
      organize,
    }),
    [sessions, pinned, archived, showArchived, renames, projectDefs, organize],
  );
  const archivedCount = sessions.filter((session) => archived.includes(session.id)).length;
  const runningRuns = runs.filter((run) => run.status === "running").length;
  const schedUnread = scheduledUnread(sessions, unread);
  // Pinning is a stable presentation sort within Projects. Removed projects
  // stay in the journal-derived model (and in search) but leave this rail until
  // the explicit recovery row reveals them. Ordering itself lives in
  // orderProjectGroups (INC-94 behaviour preserved as sort "priority").
  const orderedProjects = useMemo(() => {
    const visible = model.projects.filter((project) => showRemovedProjects || !projects[project.key]?.removed);
    return orderProjectGroups(visible, { overlays: projects, sort });
  }, [model.projects, projects, showRemovedProjects, sort]);
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

  // The Projects section renders every group. Order comes from the model
  // (activity mtime) plus explicit pins — selecting a session is not a
  // mutation, so it never truncates or reorders the rail.
  const shownProjects = orderedProjects;

  // Workspace-less sessions use a plain heading with no folder, caret or indent.
  // Capping reuses visibleProjectSessions so the current session remains visible.
  // In one-list mode this flat list IS the whole rail, so it breathes to 20.
  const flatCap = organize === "one-list" ? 20 : 6;
  const shownSessions = useMemo(
    () => visibleProjectSessions(
      { key: "__sessions__", label: "Sessions", sessions: model.workspaceLessSessions },
      { expanded: showAllSessions, cap: flatCap, current: currentSid || undefined },
    ),
    [model.workspaceLessSessions, showAllSessions, currentSid, flatCap],
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
    cancelHoverPreviewClose();
    setHoverPreview({ kind: "session", sid: session.id, top: Math.max(10, Math.min(top - 6, window.innerHeight - 154)) });
    const workspace = session.workspace;
    if (!workspace || Object.prototype.hasOwnProperty.call(branchByWorkspace, workspace)) return;
    setBranchByWorkspace((current) => ({ ...current, [workspace]: "" }));
    api.gitBranches(workspace)
      .then((info) => setBranchByWorkspace((current) => ({
        ...current,
        [workspace]: info.isRepo
          ? (info.current || "Detached HEAD")
          : "",
      })))
      .catch(() => setBranchByWorkspace((current) => ({
        ...current,
        [workspace]: isManagedWorktreeWorkspace(workspace) ? "Detached HEAD" : "",
      })));
  };

  const restoreSessionActionFocusAfterMutation = (sid: string) => {
    requestAnimationFrame(() => {
      const touchActionsVisible =
        window.innerWidth <= 900 ||
        window.matchMedia?.("(any-pointer: coarse)").matches;
      const rows = Array.from(
        document.querySelectorAll<HTMLElement>(
          ".sidebar .project-session-wrap",
        ),
      );
      const sameRow = rows.find((row) => row.dataset.sessionId === sid);
      const sameSession = touchActionsVisible
        ? sameRow?.querySelector<HTMLButtonElement>(".session-touch-trigger")
        : sameRow?.querySelector<HTMLButtonElement>(".project-session");
      const firstSession = touchActionsVisible
        ? document.querySelector<HTMLButtonElement>(
          ".sidebar .session-touch-trigger",
        )
        : document.querySelector<HTMLButtonElement>(
          ".sidebar .project-session",
        );
      (
        sameSession ||
        firstSession ||
        document.querySelector<HTMLButtonElement>(".sidebar .brand-main")
      )?.focus();
    });
  };

  const renderSessionActions = (
    sid: string,
    modalReturnFocus?: HTMLElement,
  ) => (
    <SidebarSessionActions
      title={displayTitle(renames, sid, sessions.find((session) => session.id === sid)?.title)}
      pinned={pinned.includes(sid)}
      unread={unread.includes(sid)}
      archived={archived.includes(sid)}
      onTogglePin={() => {
        togglePin(sid);
        restoreSessionActionFocusAfterMutation(sid);
      }}
      onRename={() => {
        const active =
          document.activeElement instanceof HTMLElement
            ? document.activeElement
            : null;
        const returnFocus =
          modalReturnFocus ??
          active
            ?.closest(".pop-wrap")
            ?.querySelector<HTMLElement>(".menu-trigger") ??
          undefined;
        // Menu's bubble-phase close queues a zero-delay trigger restore after
        // this handler returns. The inner timer is registered only when this
        // outer timer runs, so the already-queued restore runs first and the
        // modal FocusScope deterministically owns final focus.
        const openRename = store.getState().openModal;
        window.setTimeout(() => {
          window.setTimeout(
            () => openRename({
              kind: "rename",
              sid,
              returnFocus,
            }),
            0,
          );
        }, 0);
      }}
      onToggleRead={() => unread.includes(sid) ? markRead(sid) : markUnread(sid)}
      onToggleArchive={() => {
        toggleArchive(sid);
        restoreSessionActionFocusAfterMutation(sid);
      }}
    />
  );

  // The Projects section header's controls (INC-104): the ⋯ organize menu and
  // the + create button. Rendered in both by-project and one-list headings so
  // each mode can always reach the other.
  const renderOrganizeMenu = () => (
    <Menu label={<DotsThree size={16} />} ariaLabel="Organize sidebar" iconTrigger>
      <MenuLabel>Organize sidebar</MenuLabel>
      <MenuItem checked={organize === "by-project"} onClick={() => setOrganize("by-project")}>By project</MenuItem>
      <MenuItem checked={organize === "one-list"} onClick={() => setOrganize("one-list")}>In one list</MenuItem>
      <MenuLabel>Sort by</MenuLabel>
      <MenuItem checked={sort === "priority"} onClick={() => setSort("priority")}>Priority</MenuItem>
      <MenuItem checked={sort === "updated"} onClick={() => setSort("updated")}>Last updated</MenuItem>
      <MenuItem checked={sort === "manual"} onClick={() => setSort("manual")}>Manual order</MenuItem>
    </Menu>
  );
  const renderCreateProjectButton = () => (
    <IconButton
      size="sm"
      variant="ghost"
      className="project-quick-action"
      aria-label="Create project"
      title="Create project"
      onClick={() => openModal({ kind: "project", mode: "create" })}
    >
      <Plus size={16} />
    </IconButton>
  );
  // Show more/less for the flat session list — shared by the by-project
  // "Sessions" section and the one-list "Chats" section.
  const renderFlatOverflowToggle = () => {
    if (model.workspaceLessSessions.length === 0) return null;
    if (!showAllSessions && model.workspaceLessSessions.length > shownSessions.length) {
      return (
        <button
          className="show-more"
          onClick={() => setShowAllSessions(true)}
          aria-label={`Show all ${model.workspaceLessSessions.length} sessions`}
        >
          Show more
        </button>
      );
    }
    if (showAllSessions && model.workspaceLessSessions.length > flatCap) {
      return (
        <button
          className="show-more"
          onClick={() => setShowAllSessions(false)}
          aria-label={`Show only the ${flatCap} most recent sessions`}
        >
          Show less
        </button>
      );
    }
    return null;
  };

  // One source for the project group's actions: the desktop right-click
  // ContextMenu and the touch ⋯ Menu on the heading row (INC-87.2) render the
  // same items, so the two entrances can never drift apart.
  const renderProjectActions = (group: ProjectGroup, label: string) => {
    const key = group.key;
    const overlay = projects[key];
    // The primary folder carries the single-target actions (Reveal, worktree,
    // New chat); a multi-folder project's full list lives in the hover card.
    const workspace = group.workspace;
    const ids = group.sessions.map((session) => session.id);
    return (
      <SidebarProjectActions
        pinned={overlay?.pinned}
        removed={overlay?.removed}
        workspace={workspace}
        explicit={!!group.projectId}
        chatCount={ids.length}
        onTogglePin={() => void toggleProjectPinned(key, !overlay?.pinned)}
        onReveal={() => {
          if (workspace) openProjectIn(workspace, "finder");
        }}
        onCreateWorktree={() => {
          if (!workspace) return;
          openPrompt({
            title: "Create permanent worktree",
            label: "New branch name",
            placeholder: "feature/my-work",
            submitLabel: "Create",
            onSubmit: (branch) => {
              void api.makeWorktree(workspace, branch.trim())
                .then((result) => toast(`worktree created · ${result.path}`, "info"))
                .catch((error: any) => toast(error.message, "error", error.details));
            },
          });
        }}
        onEdit={() => openModal({
          kind: "project",
          // A derived group opens the same dialog in create mode, prefilled —
          // Save upgrades it into an explicit registry entry (INC-104).
          mode: group.projectId ? "edit" : "create",
          id: group.projectId,
          overlayKey: group.projectId ? undefined : key,
          initialName: label,
          initialFolders: group.folders ?? (workspace ? [workspace] : []),
        })}
        onArchiveChats={() => ids.filter((id) => !archived.includes(id)).forEach(toggleArchive)}
        onToggleRemoved={() => {
          if (group.projectId) {
            const id = group.projectId;
            openModal({
              kind: "confirm",
              title: "Remove project?",
              body: `"${label}" is only a grouping — its chats, journals, and files stay exactly where they are. The folders go back to grouping on their own.`,
              confirmLabel: "Remove",
              danger: true,
              onConfirm: async () => {
                try {
                  await deleteProject(id);
                } catch (error: any) {
                  toast(error.message, "error", error.details);
                }
              },
            });
            return;
          }
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
      />
    );
  };

  const renderSession = (session: (typeof sessions)[number], nested = false) => {
    const active = session.id === currentSid;
    const isUnread = unread.includes(session.id);
    const title = displayTitle(renames, session.id, session.title);
    const when = relTimeAgo(sessionUpdatedDate(session));
    const openContext = (
      x: number,
      y: number,
      returnFocus: HTMLElement,
    ) => {
      // Opening a context menu instantly dismisses any hover preview so the
      // two floating layers stay mutually exclusive (R3-1).
      clearHoverPreview();
      setCtx({ kind: "session", x, y, sid: session.id, returnFocus });
    };
    return (
      <SidebarSessionItem
        key={session.id}
        session={session}
        title={title}
        when={when}
        nested={nested}
        active={active}
        unread={isUnread}
        archived={archived.includes(session.id)}
        pinned={pinned.includes(session.id)}
        actions={renderSessionActions(session.id)}
        onSelect={() => {
          select(session.id);
          onNavigate?.();
        }}
        onOpenContext={openContext}
        onPreview={(top) => previewSession(session, top)}
        onPreviewEnd={scheduleHoverPreviewClose}
        onDismissPreview={clearHoverPreview}
        onTogglePin={() => {
          togglePin(session.id);
          restoreSessionActionFocusAfterMutation(session.id);
        }}
        onToggleArchive={() => {
          toggleArchive(session.id);
          restoreSessionActionFocusAfterMutation(session.id);
        }}
      />
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
      <div className="flex h-[46px] shrink-0 items-center justify-between px-4">
        <button className="brand-main" onClick={() => { showPage("home"); onNavigate?.(); }} aria-label="Orca home">
          <span className="text-[16px] leading-6 font-semibold tracking-[-0.18px] text-[#222222] dark:text-ink">Orca</span>
        </button>
        <div className="flex items-center gap-[2px]">
          <IconButton
            size="md"
            variant="ghost"
            className="max-[900px]:w-[44px]! max-[900px]:h-[44px]!"
            onClick={onOpenPalette}
            title={`Search sessions (${keyLabel("mod")}K)`}
            aria-label="Search sessions"
          >
            <MagnifyingGlass size={17} weight="regular" />
          </IconButton>
          <IconButton
            variant="ghost"
            className="sidebar-close max-[900px]:w-[44px]! max-[900px]:h-[44px]!"
            onClick={onHide}
            title="Close sidebar"
            aria-label="Close sidebar"
          >
            <X size={17} weight="regular" />
          </IconButton>
        </div>
      </div>

      <nav className="primary-nav" aria-label="Primary">
        {NAV_DESTINATIONS.map(({ key, label, icon: DestIcon, keys }) => (
          <button
            key={key}
            className={`max-[900px]:h-11 [@media(any-pointer:coarse)]:h-11${
              key !== "home" && !currentSid && currentPage === key
                ? " active"
                : ""
            }`}
            aria-current={!currentSid && currentPage === key ? "page" : undefined}
            onClick={() => { showPage(key); onNavigate?.(); }}
            title={keys ? `${label} (${keys.map(keyLabel).join("")})` : label}
          >
            <span className="inline-grid h-4 w-4 shrink-0 place-items-center">
              <DestIcon size={16} weight="regular" />
            </span>
            <span className="leading-5">{label}</span>
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
              <CaretRight size={11} aria-hidden="true" className={`section-caret${foldedSections.has("pinned") ? "" : " open"}`} />
              Pinned
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
        ) : model.projects.length === 0 && model.workspaceLessSessions.length === 0 && model.pinned.length === 0 && projectDefs.length === 0 ? (
          <div className="sidebar-empty">
            <Tray size={22} />
            <b>No sessions yet</b>
            <span>Start a session to see it here.</span>
          </div>
        ) : null}

        {/* The heading renders even with zero groups — the + button is the
            only way to create the FIRST project, and the ⋯ menu is the only
            way back out of one-list mode (INC-104). */}
        {organize === "by-project" && sessionsReady && (
        <section className="sidebar-section projects-section">
          <div className="section-heading-row">
            <button
              className="section-label section-toggle"
              onClick={() => toggleSection("projects")}
              aria-expanded={!foldedSections.has("projects")}
            >
              <CaretRight size={11} aria-hidden="true" className={`section-caret${foldedSections.has("projects") ? "" : " open"}`} />
              Projects
            </button>
            <span className="section-heading-actions">
              {renderOrganizeMenu()}
              {renderCreateProjectButton()}
            </span>
          </div>
          {!foldedSections.has("projects") && shownProjects.length === 0 && (
            model.workspaceLessSessions.length > 0 || model.pinned.length > 0 ? (
              <div className="projects-none">No projects yet</div>
            ) : null
          )}
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
            const openMenu = (
              x: number,
              y: number,
              returnFocus: HTMLElement,
            ) => {
              clearHoverPreview();
              setCtx({
                kind: "project",
                x,
                y,
                key: project.key,
                label: name,
                workspace: project.workspace,
                ids: project.sessions.map((session) => session.id),
                returnFocus,
              });
            };
            return (
              <SidebarProjectItem
                key={project.key}
                name={name}
                folded={folded}
                removed={overlay?.removed}
                emptyLabel={project.sessions.length === 0 ? "No chats" : undefined}
                actions={renderProjectActions(project, name)}
                overflow={
                  !folded && !showAll && project.sessions.length > shown.length
                    ? "more"
                    : !folded && showAll && project.sessions.length > 6
                      ? "less"
                      : null
                }
                onToggle={() => setProjectCollapsed(project.key, !persistedFold)}
                onOpenContext={openMenu}
                onPreview={(top) => {
                  if (ctx) return;
                  cancelHoverPreviewClose();
                  setHoverPreview({
                    kind: "project",
                    key: project.key,
                    // The path wraps to at most four lines, so reserve enough
                    // room for the tallest card when clamping near the bottom.
                    top: Math.max(10, Math.min(top - 6, window.innerHeight - 200)),
                  });
                }}
                onPreviewEnd={scheduleHoverPreviewClose}
                onDismissPreview={clearHoverPreview}
                onNewChat={() => {
                  if (!project.workspace) return;
                  newSessionForProject(project.workspace);
                  onNavigate?.();
                }}
                onToggleOverflow={() => setExpanded((current) => {
                  const next = new Set(current);
                  if (showAll) next.delete(project.key);
                  else next.add(project.key);
                  return next;
                })}
              >
                {shown.map((session) => renderSession(session, true))}
              </SidebarProjectItem>
            );
          })}
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

        {/* One-list mode (INC-104): the flat list IS the rail. The heading
            keeps the ⋯/+ controls so the organize menu stays reachable. */}
        {organize === "one-list" && sessionsReady && (
          <section className="sidebar-section sessions-section">
            <div className="section-heading-row">
              <div className="section-label">Chats</div>
              <span className="section-heading-actions">
                {renderOrganizeMenu()}
                {renderCreateProjectButton()}
              </span>
            </div>
            {shownSessions.map((session) => renderSession(session))}
            {renderFlatOverflowToggle()}
          </section>
        )}

        {/* SB-13 · Sessions — the ones that belong to no project. Flat rows at the
            Pinned indent: no folder, no caret, nothing claiming a directory
            these sessions do not have. Renders only when it has something to say;
            an empty heading is worse than no heading. In one-list mode the
            block above already renders these rows under "Chats". */}
        {organize === "by-project" && model.workspaceLessSessions.length > 0 && (
          <section className="sidebar-section sessions-section">
            <div className="section-label">Sessions</div>
            {shownSessions.map((session) => renderSession(session))}
            {renderFlatOverflowToggle()}
          </section>
        )}

        {sessionsLoadingOlder && (
          <Spinner className="sidebar-history-loading" label="Loading older sessions…" />
        )}
        {archivedCount > 0 && (
          <button className="archive-toggle" onClick={toggleShowArchived}>
            <ArchiveBox size={14} /> {showArchived ? "Hide" : "Show"} archived · {archivedCount}
          </button>
        )}
      </div>

      <div className={`side-foot${health?.daemonUp === false ? " side-foot-connection" : " side-foot-quiet"}`}>
        {/* INC-41 L3 · Three states, not two. `health === null` means the first
            /health call hasn't answered yet — rendering that as a red "Daemon
            offline" made every cold load flash a fake outage (and armed a
            restart click). Unknown is neutral and inert; only a health record
            that actually says daemonUp:false is an outage. */}
        <SidebarConnectionStatus
          state={!health ? "checking" : health.daemonUp ? "connected" : "offline"}
          version={health?.version}
          onRestart={restartDaemon}
        />
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
            <SidebarPreviewCard
              kind="project"
              top={hoverPreview.top}
              name={name}
              pinned={overlay?.pinned}
              chats={project.sessions.length}
              workspace={project.workspace}
              folders={project.folders}
              missing={project.missing}
              onHoverStart={cancelHoverPreviewClose}
              onHoverEnd={scheduleHoverPreviewClose}
            />
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
          <SidebarPreviewCard
            kind="session"
            top={hoverPreview.top}
            title={title}
            when={when}
            project={projectNameForWorkspace(workspace, projectDefs, projects)}
            branch={branch}
            status={status}
            onHoverStart={cancelHoverPreviewClose}
            onHoverEnd={scheduleHoverPreviewClose}
          />
        );
      })()}

      {ctx?.kind === "session" && (
        <ContextMenu
          x={ctx.x}
          y={ctx.y}
          ariaLabel={`${displayTitle(
            renames,
            ctx.sid,
            sessions.find((session) => session.id === ctx.sid)?.title,
          )} actions`}
          returnFocus={ctx.returnFocus}
          onClose={() => setCtx(null)}
        >
          {renderSessionActions(ctx.sid, ctx.returnFocus)}
        </ContextMenu>
      )}
      {ctx?.kind === "project" && (() => {
        // Resolve the live group so the menu reflects the model at render
        // time; the ctx snapshot only anchors position and labels.
        const group = model.projects.find((item) => item.key === ctx.key);
        if (!group) return null;
        return (
          <ContextMenu
            x={ctx.x}
            y={ctx.y}
            ariaLabel={`${ctx.label} actions`}
            returnFocus={ctx.returnFocus}
            onClose={() => setCtx(null)}
          >
            {renderProjectActions(group, ctx.label)}
          </ContextMenu>
        );
      })()}
    </aside>
  );
}
