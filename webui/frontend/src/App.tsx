import { useCallback, useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { useAppStoreApi, useStore } from "./store";
import { useBreakpoint } from "./hooks/useBreakpoint";
import { Sidebar } from "./components/Sidebar";
import { SessionView } from "./components/SessionView";
import { RunView } from "./components/RunView";
import { Home } from "./components/Home";
import { Scheduled } from "./components/Scheduled";
import { Modals } from "./components/Modals";
import { Toasts } from "./components/Toasts";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { CommandPalette } from "./components/CommandPalette";
import { Shortcuts } from "./components/Shortcuts";
import { Settings } from "./components/Settings";
import { quickSwitchSessions } from "./viewModels";
import { applyAppearance, loadAppearance } from "./theme";
import { SidebarSimple } from "@phosphor-icons/react";
import { useAppServices } from "./app/appServices";
import { useFocusScope } from "./ui/FocusScope";
import { IconButton } from "./ui/IconButton";

export function AppShell() {
  const { storage } = useAppServices();
  const store = useAppStoreApi();
  const { currentSid, currentRunId, currentPage, scheduledDetailSid } = useStore();
  const helpOpen = useStore((s) => s.helpOpen);
  const openHelp = useStore((s) => s.openHelp);
  const closeHelp = useStore((s) => s.closeHelp);
  const sidebarCollapsed = useStore((s) => s.sidebarCollapsed);
  const toggleSidebar = useStore((s) => s.toggleSidebar);
  const sidebarWidth = useStore((s) => s.sidebarWidth);
  const unread = useStore((s) => s.unread);
  const [palette, setPalette] = useState(false);
  const paletteOpenRef = useRef(false);
  const paletteRestoreFocusRef = useRef(true);
  paletteOpenRef.current = palette;
  const [settingsOpen, setSettingsOpen] = useState(false);
  const settingsOpenRef = useRef(false);
  const settingsReturnFocusRef = useRef<HTMLElement | null>(null);
  settingsOpenRef.current = settingsOpen;
  const bp = useBreakpoint();
  const isMobile = bp.compact || bp.tablet;
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const mobileSidebarRestoreFocusRef = useRef(true);
  const mobileSidebarReturnFocusRef = useRef<HTMLElement | null>(null);
  const mobileSidebarRoot = useCallback(
    () => document.querySelector<HTMLElement>(".app > .sidebar"),
    [],
  );
  const openMobileSidebar = useCallback(() => {
    mobileSidebarReturnFocusRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    mobileSidebarRestoreFocusRef.current = true;
    setMobileSidebarOpen(true);
  }, []);
  const closeMobileSidebar = useCallback((restoreFocus = true) => {
    mobileSidebarRestoreFocusRef.current = restoreFocus;
    setMobileSidebarOpen(false);
  }, []);

  useFocusScope(mobileSidebarRoot, {
    enabled: isMobile && mobileSidebarOpen,
    initialFocus: '[aria-label="Search sessions"]',
    restoreFocus: () => {
      const target = mobileSidebarReturnFocusRef.current;
      if (target?.isConnected && target.getClientRects().length > 0) return target;
      return document.querySelector<HTMLElement>("[data-focus-restore-fallback]");
    },
    shouldRestoreFocus: () => mobileSidebarRestoreFocusRef.current,
    onEscape: () => closeMobileSidebar(true),
  });

  const openPalette = () => {
    paletteRestoreFocusRef.current = true;
    setPalette(true);
  };
  const closePalette = (restoreFocus = true) => {
    paletteRestoreFocusRef.current = restoreFocus;
    setPalette(false);
  };

  const openSettings = () => {
    const active = document.activeElement;
    settingsReturnFocusRef.current = active instanceof HTMLElement && active.closest('[role="menu"]')
      ? document.querySelector<HTMLElement>('button[aria-label="More options"]')
      : active instanceof HTMLElement ? active : null;
    if (isMobile) setMobileSidebarOpen(false);
    setSettingsOpen(true);
  };
  const closeSettings = () => {
    setSettingsOpen(false);
  };

  // Apply the full appearance record (fonts, contrast, diff markers, motion,
  // syntax) once styles are mounted — main.tsx only restores the theme, so this
  // completes the picture with a minimal, one-frame settle.
  useLayoutEffect(() => {
    applyAppearance(loadAppearance(storage.local));
  }, []);

  useEffect(() => {
    if (isMobile) setMobileSidebarOpen(false);
  }, [isMobile]);

  // Help is opened by Sidebar's store action rather than an App callback. Close
  // the drawer before paint and mark this as a focus transfer so its cleanup
  // cannot steal focus back from Shortcuts.
  useLayoutEffect(() => {
    if (!helpOpen || !isMobile || !mobileSidebarOpen) return;
    closeMobileSidebar(false);
  }, [closeMobileSidebar, helpOpen, isMobile, mobileSidebarOpen]);

  useEffect(() => {
    document.title = unread.length > 0 ? `(${unread.length}) AgentRunner` : "AgentRunner";
  }, [unread.length]);

  // Global keys: ⌘K/Ctrl-K toggles the command palette; "?" opens the
  // keyboard-shortcuts reference (unless the user is typing into a field).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // ⌘, / Ctrl-, opens (or closes) the full-window Settings surface (Codex).
      if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key === ",") {
        e.preventDefault();
        if (settingsOpenRef.current) closeSettings();
        else openSettings();
        return;
      }
      // While Settings owns the window, let it handle its own keys (Escape to
      // close) and mute the app-level shortcuts behind it.
      if (settingsOpenRef.current) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        if (paletteOpenRef.current) closePalette();
        else openPalette();
        return;
      }
      // ⌥⌘↑ / ⌥⌘↓ moves to the previous / next session in the sidebar order.
      if ((e.metaKey || e.ctrlKey) && e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
        e.preventDefault();
        store.getState().selectAdjacent(e.key === "ArrowDown" ? 1 : -1);
        return;
      }
      // ⌘1..⌘9 (Ctrl elsewhere) jump straight to a recent session — the same list
      // and order as the command palette's ⌘-digit badges (INC-41 W8). Works
      // globally and while the palette is open; closes the palette on jump.
      if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key >= "1" && e.key <= "9") {
        const state = store.getState();
        const target = quickSwitchSessions(state.sessions, { archived: state.archived })[Number(e.key) - 1];
        if (target) {
          e.preventDefault();
          closePalette(false);
          state.select(target.id);
        }
        return;
      }
      // New session (INC-41 RH-4). Codex-the-desktop-app binds plain ⌘N, but we
      // live in a browser tab: Chrome/Safari reserve ⌘N (new window) and ⇧⌘N
      // (incognito) at the browser level — the keydown never reaches the page,
      // so a "⌘N" badge would be exactly the lie RH-3 was about. We bind the
      // ⌥⌘ family the app already uses for session navigation (⌥⌘↑/↓) and that is
      // what shortcuts.ts and the sidebar badge advertise. The plain-⌘N shape
      // is accepted by the same branch (altKey optional) so a wrapped context
      // that *does* deliver it (Electron / standalone PWA) gets Codex's key for
      // free. e.code, not e.key: on macOS ⌥+N is a dead key ("Dead"/"˜").
      if ((e.metaKey || e.ctrlKey) && !e.shiftKey && (e.code === "KeyN" || e.key.toLowerCase() === "n")) {
        const t = e.target as HTMLElement | null;
        // Never steal a keystroke out of a field the user is typing in.
        if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) return;
        e.preventDefault();
        closePalette(false);
        store.getState().showPage("home");
        // …and land ready to type (INC-41 HM-1). A fresh mount focuses itself
        // (Composer's home effect), but when we were ALREADY on home nothing
        // remounts, so the caret would stay wherever it was. Focus the live node
        // after React paints; querying the DOM (like Home's prefillComposer)
        // keeps App from having to own a ref into the composer.
        requestAnimationFrame(() => {
          document.querySelector<HTMLTextAreaElement>(".home-empty-state .cx-home textarea")?.focus();
        });
        return;
      }
      // ⌘B / Ctrl-B shows or hides the sidebar (Codex's Toggle sidebar).
      if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "b") {
        e.preventDefault();
        if (isMobile) setMobileSidebarOpen((open) => !open);
        else store.getState().toggleSidebar();
        return;
      }
      if (e.key === "?" && !e.metaKey && !e.ctrlKey && !e.altKey) {
        const t = e.target as HTMLElement | null;
        const typing = !!t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable);
        if (!typing) {
          e.preventDefault();
          openHelp();
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
    };
  }, []);

  const effectiveCollapsed = isMobile ? !mobileSidebarOpen : sidebarCollapsed;
  const hideSidebar = () => isMobile ? closeMobileSidebar(true) : toggleSidebar();
  const showSidebar = () => isMobile ? openMobileSidebar() : toggleSidebar();
  const closeAfterNavigate = () => {
    if (isMobile) closeMobileSidebar(true);
  };

  return (
    <div
      className={"app" + (effectiveCollapsed ? " collapsed" : "")}
      style={{ "--sidebar-width": `${sidebarWidth}px` } as CSSProperties}
    >
      {/* A11Y-1 · Must be the first focusable node in the DOM: the sidebar holds
          ~94% of the app's tab stops, so without this a keyboard user never
          reaches the conversation or composer. */}
      <a
        className="skip-link"
        href="#main"
        onClick={(e) => {
          // location.hash is the router's ("#<sid>" / "#scheduled"), so the
          // fragment must never actually land there — move focus ourselves.
          e.preventDefault();
          document.getElementById("main")?.focus();
        }}
      >
        Skip to conversation
      </a>
      <Sidebar
        onHide={hideSidebar}
        onNavigate={closeAfterNavigate}
        // RH-5: the sidebar's magnifier *is* ⌘K — one search entry point.
        onOpenPalette={() => {
          if (isMobile) closeMobileSidebar(false);
          openPalette();
        }}
        onOpenSettings={() => {
          if (isMobile) closeMobileSidebar(false);
          openSettings();
        }}
      />
      {isMobile && mobileSidebarOpen && <button className="sidebar-scrim" aria-label="Close sidebar" onClick={hideSidebar} />}
      {/* tabIndex -1 so the skip link can land focus here (not just scroll):
          the next Tab then continues into the conversation / composer. */}
      <div
        className="main"
        id="main"
        tabIndex={-1}
        inert={isMobile && mobileSidebarOpen ? true : undefined}
        aria-hidden={isMobile && mobileSidebarOpen ? "true" : undefined}
      >
        {effectiveCollapsed && !(isMobile && currentPage === "scheduled" && scheduledDetailSid) && (
          <IconButton
            className="sidebar-show"
            data-focus-restore-fallback
            size="md"
            variant="outline"
            onClick={showSidebar}
            title="Show sidebar (⌘B)"
            aria-label="Show sidebar"
          >
            <SidebarSimple size={17} />
          </IconButton>
        )}
        <ErrorBoundary resetKey={currentRunId || currentSid || "home"}>
          {currentRunId ? (
            <RunView runId={currentRunId} />
          ) : currentSid ? (
            <SessionView
              sid={currentSid}
              key={currentSid}
              mobileNavigationOpen={isMobile && mobileSidebarOpen}
            />
          ) : currentPage === "scheduled" ? (
            <Scheduled />
          ) : (
            <Home />
          )}
        </ErrorBoundary>
      </div>
      <Modals />
      {/* The palette's `Open settings` row reuses the gear's / ⌘,'s own opener
          (CP-8) rather than a second, drifting copy of it. */}
      {palette && (
        <CommandPalette
          onClose={closePalette}
          onOpenSettings={openSettings}
          shouldRestoreFocus={() => paletteRestoreFocusRef.current}
        />
      )}
      {helpOpen && <Shortcuts onClose={closeHelp} />}
      {settingsOpen && (
        <Settings
          onClose={closeSettings}
          restoreFocus={() => {
            const returnTarget = settingsReturnFocusRef.current;
            if (returnTarget?.isConnected && returnTarget.getClientRects().length > 0) {
              return returnTarget;
            }
            return document.querySelector<HTMLElement>(".sidebar-show");
          }}
        />
      )}
      <Toasts />
    </div>
  );
}
