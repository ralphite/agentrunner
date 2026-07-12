import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useStore } from "./store";
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
import { requestNotifyPermission } from "./notify";
import { quickSwitchTasks } from "./viewModels";
import { applyAppearance, loadAppearance } from "./theme";
import { SidebarSimple } from "@phosphor-icons/react";
import "./styles.rs.css";

export function App() {
  const { currentSid, currentRunId, currentPage, refreshHealth, refreshSessions, refreshRuns, refreshProjects, select, selectRun, showPage } =
    useStore();
  const helpOpen = useStore((s) => s.helpOpen);
  const openHelp = useStore((s) => s.openHelp);
  const closeHelp = useStore((s) => s.closeHelp);
  const sidebarCollapsed = useStore((s) => s.sidebarCollapsed);
  const toggleSidebar = useStore((s) => s.toggleSidebar);
  const unread = useStore((s) => s.unread);
  const [palette, setPalette] = useState(false);
  const paletteOpenRef = useRef(false);
  const paletteReturnFocusRef = useRef<HTMLElement | null>(null);
  paletteOpenRef.current = palette;
  const [settingsOpen, setSettingsOpen] = useState(false);
  const settingsOpenRef = useRef(false);
  const settingsReturnFocusRef = useRef<HTMLElement | null>(null);
  settingsOpenRef.current = settingsOpen;
  const [isMobile, setIsMobile] = useState(() => window.matchMedia("(max-width: 680px)").matches);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);

  const openPalette = () => {
    const active = document.activeElement;
    paletteReturnFocusRef.current = active instanceof HTMLElement ? active : null;
    setPalette(true);
  };
  const closePalette = (restoreFocus = true) => {
    const returnTarget = paletteReturnFocusRef.current;
    setPalette(false);
    if (!restoreFocus) return;
    requestAnimationFrame(() => {
      if (returnTarget?.isConnected && returnTarget.getClientRects().length > 0) returnTarget.focus();
    });
  };

  const openSettings = () => {
    const active = document.activeElement;
    settingsReturnFocusRef.current = active instanceof HTMLElement ? active : null;
    if (window.matchMedia("(max-width: 680px)").matches) setMobileSidebarOpen(false);
    setSettingsOpen(true);
  };
  const closeSettings = () => {
    const returnTarget = settingsReturnFocusRef.current;
    setSettingsOpen(false);
    requestAnimationFrame(() => {
      if (returnTarget?.isConnected && returnTarget.getClientRects().length > 0) returnTarget.focus();
      else document.querySelector<HTMLElement>(".sidebar-show")?.focus();
    });
  };

  // Apply the full appearance record (fonts, contrast, diff markers, motion,
  // syntax) once styles are mounted — main.tsx only restores the theme, so this
  // completes the picture with a minimal, one-frame settle.
  useLayoutEffect(() => {
    applyAppearance(loadAppearance());
  }, []);

  useEffect(() => {
    const query = window.matchMedia("(max-width: 680px)");
    const sync = () => {
      setIsMobile(query.matches);
      if (query.matches) setMobileSidebarOpen(false);
    };
    query.addEventListener("change", sync);
    return () => query.removeEventListener("change", sync);
  }, []);

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
      // ⌥⌘↑ / ⌥⌘↓ moves to the previous / next task in the sidebar order.
      if ((e.metaKey || e.ctrlKey) && e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
        e.preventDefault();
        useStore.getState().selectAdjacent(e.key === "ArrowDown" ? 1 : -1);
        return;
      }
      // ⌘1..⌘9 (Ctrl elsewhere) jump straight to a recent task — the same list
      // and order as the command palette's ⌘-digit badges (INC-41 W8). Works
      // globally and while the palette is open; closes the palette on jump.
      if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key >= "1" && e.key <= "9") {
        const state = useStore.getState();
        const target = quickSwitchTasks(state.sessions, { archived: state.archived })[Number(e.key) - 1];
        if (target) {
          e.preventDefault();
          setPalette(false);
          state.select(target.id);
        }
        return;
      }
      // New task (INC-41 RH-4). Codex-the-desktop-app binds plain ⌘N, but we
      // live in a browser tab: Chrome/Safari reserve ⌘N (new window) and ⇧⌘N
      // (incognito) at the browser level — the keydown never reaches the page,
      // so a "⌘N" badge would be exactly the lie RH-3 was about. We bind the
      // ⌥⌘ family the app already uses for task navigation (⌥⌘↑/↓) and that is
      // what shortcuts.ts and the sidebar badge advertise. The plain-⌘N shape
      // is accepted by the same branch (altKey optional) so a wrapped context
      // that *does* deliver it (Electron / standalone PWA) gets Codex's key for
      // free. e.code, not e.key: on macOS ⌥+N is a dead key ("Dead"/"˜").
      if ((e.metaKey || e.ctrlKey) && !e.shiftKey && (e.code === "KeyN" || e.key.toLowerCase() === "n")) {
        const t = e.target as HTMLElement | null;
        // Never steal a keystroke out of a field the user is typing in.
        if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) return;
        e.preventDefault();
        setPalette(false);
        useStore.getState().showPage("home");
        return;
      }
      // ⌘B / Ctrl-B shows or hides the sidebar (Codex's Toggle sidebar).
      if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "b") {
        e.preventDefault();
        if (window.matchMedia("(max-width: 680px)").matches) setMobileSidebarOpen((open) => !open);
        else useStore.getState().toggleSidebar();
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
    // Notification permission needs a user gesture — ask on the first click.
    const askOnce = () => requestNotifyPermission();
    window.addEventListener("pointerdown", askOnce, { once: true });
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("pointerdown", askOnce);
    };
  }, []);

  useEffect(() => {
    refreshHealth();
    refreshSessions();
    refreshRuns();
    refreshProjects();
    const h = setInterval(refreshHealth, 5000);
    const s = setInterval(refreshSessions, 4000);
    const r = setInterval(refreshRuns, 4000);
    const p = setInterval(refreshProjects, 8000);
    // hash routing: "run:<id>" → a background run; anything else → a session.
    const route = (raw0: string) => {
      // Tolerate a "/s/<sid>" (or "s/<sid>") deep-link prefix: the app itself
      // emits bare "#<sid>", but a shared/typed link using the "/s/" form must
      // resolve to the same session rather than leaking the raw route string
      // into the title and rendering an empty page.
      const raw = raw0.replace(/^\/?s\//, "");
      if (raw === "scheduled") {
        showPage(raw);
      } else if (raw.startsWith("run:")) {
        const rid = raw.slice(4);
        if (rid && rid !== useStore.getState().currentRunId) selectRun(rid);
      } else if (raw && raw !== useStore.getState().currentSid) {
        select(raw);
      } else if (!raw && (useStore.getState().currentSid || useStore.getState().currentRunId || useStore.getState().currentPage !== "home")) {
        showPage("home");
      }
    };
    if (location.hash.length > 1) route(location.hash.slice(1));
    const onHash = () => route(location.hash.slice(1));
    window.addEventListener("hashchange", onHash);
    return () => {
      clearInterval(h);
      clearInterval(s);
      clearInterval(r);
      clearInterval(p);
      window.removeEventListener("hashchange", onHash);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const effectiveCollapsed = isMobile ? !mobileSidebarOpen : sidebarCollapsed;
  const hideSidebar = () => isMobile ? setMobileSidebarOpen(false) : toggleSidebar();
  const showSidebar = () => isMobile ? setMobileSidebarOpen(true) : toggleSidebar();
  const closeAfterNavigate = () => {
    if (isMobile) setMobileSidebarOpen(false);
  };

  return (
    <div className={"app" + (effectiveCollapsed ? " collapsed" : "")}>
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
          closeAfterNavigate();
          openPalette();
        }}
        onOpenSettings={() => {
          closeAfterNavigate();
          openSettings();
        }}
      />
      {isMobile && mobileSidebarOpen && <button className="sidebar-scrim" aria-label="Close sidebar" onClick={hideSidebar} />}
      {/* tabIndex -1 so the skip link can land focus here (not just scroll):
          the next Tab then continues into the conversation / composer. */}
      <div className="main" id="main" tabIndex={-1}>
        {effectiveCollapsed && (
          <button
            className="sidebar-show"
            onClick={showSidebar}
            title="Show sidebar (⌘B)"
            aria-label="Show sidebar"
          >
            <SidebarSimple size={17} />
          </button>
        )}
        <ErrorBoundary resetKey={currentRunId || currentSid || "home"}>
          {currentRunId ? (
            <RunView runId={currentRunId} />
          ) : currentSid ? (
            <SessionView sid={currentSid} key={currentSid} />
          ) : currentPage === "scheduled" ? (
            <Scheduled />
          ) : (
            <Home />
          )}
        </ErrorBoundary>
      </div>
      <Modals />
      {palette && <CommandPalette onClose={closePalette} />}
      {helpOpen && <Shortcuts onClose={closeHelp} />}
      {settingsOpen && <Settings onClose={closeSettings} />}
      <Toasts />
    </div>
  );
}
