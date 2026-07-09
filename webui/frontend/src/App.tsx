import { useEffect, useState } from "react";
import { useStore } from "./store";
import { Sidebar } from "./components/Sidebar";
import { SessionView } from "./components/SessionView";
import { RunView } from "./components/RunView";
import { Home } from "./components/Home";
import { Modals } from "./components/Modals";
import { Toasts } from "./components/Toasts";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { CommandPalette } from "./components/CommandPalette";
import { Shortcuts } from "./components/Shortcuts";
import { requestNotifyPermission } from "./notify";

export function App() {
  const { currentSid, currentRunId, refreshHealth, refreshSessions, refreshRuns, select, selectRun } =
    useStore();
  const helpOpen = useStore((s) => s.helpOpen);
  const openHelp = useStore((s) => s.openHelp);
  const closeHelp = useStore((s) => s.closeHelp);
  const [palette, setPalette] = useState(false);

  // Global keys: ⌘K/Ctrl-K toggles the command palette; "?" opens the
  // keyboard-shortcuts reference (unless the user is typing into a field).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette((p) => !p);
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
      if (raw.startsWith("run:")) {
        const rid = raw.slice(4);
        if (rid && rid !== useStore.getState().currentRunId) selectRun(rid);
      } else if (raw && raw !== useStore.getState().currentSid) {
        select(raw);
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

  return (
    <div className="app">
      <Sidebar />
      <div className="main">
        <ErrorBoundary resetKey={currentRunId || currentSid || "home"}>
          {currentRunId ? (
            <RunView runId={currentRunId} />
          ) : currentSid ? (
            <SessionView sid={currentSid} key={currentSid} />
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
