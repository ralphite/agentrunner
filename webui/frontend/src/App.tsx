import { useEffect, useState } from "react";
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
import { requestNotifyPermission } from "./notify";
import { quickSwitchTasks } from "./viewModels";
import { SidebarSimple } from "@phosphor-icons/react";

export function App() {
  const { currentSid, currentRunId, currentPage, refreshHealth, refreshSessions, refreshRuns, select, selectRun, showPage } =
    useStore();
  const helpOpen = useStore((s) => s.helpOpen);
  const openHelp = useStore((s) => s.openHelp);
  const closeHelp = useStore((s) => s.closeHelp);
  const sidebarCollapsed = useStore((s) => s.sidebarCollapsed);
  const toggleSidebar = useStore((s) => s.toggleSidebar);
  const unread = useStore((s) => s.unread);
  const [palette, setPalette] = useState(false);
  const [isMobile, setIsMobile] = useState(() => window.matchMedia("(max-width: 680px)").matches);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);

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
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette((p) => !p);
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
    const h = setInterval(refreshHealth, 5000);
    const s = setInterval(refreshSessions, 4000);
    const r = setInterval(refreshRuns, 4000);
    // hash routing: "run:<id>" → a background run; anything else → a session.
    const route = (raw: string) => {
      if (raw === "scheduled") {
        showPage("scheduled");
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
      <Sidebar onHide={hideSidebar} onNavigate={closeAfterNavigate} />
      {isMobile && mobileSidebarOpen && <button className="sidebar-scrim" aria-label="Close sidebar" onClick={hideSidebar} />}
      <div className="main">
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
      {palette && <CommandPalette onClose={() => setPalette(false)} />}
      {helpOpen && <Shortcuts onClose={closeHelp} />}
      <Toasts />
    </div>
  );
}
