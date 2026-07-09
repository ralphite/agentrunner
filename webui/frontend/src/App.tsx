import { useEffect } from "react";
import { useStore } from "./store";
import { Sidebar } from "./components/Sidebar";
import { SessionView } from "./components/SessionView";
import { RunView } from "./components/RunView";
import { Home } from "./components/Home";
import { Modals } from "./components/Modals";
import { Toasts } from "./components/Toasts";
import { ErrorBoundary } from "./components/ErrorBoundary";

export function App() {
  const { currentSid, currentRunId, refreshHealth, refreshSessions, refreshRuns, select } =
    useStore();

  useEffect(() => {
    refreshHealth();
    refreshSessions();
    refreshRuns();
    const h = setInterval(refreshHealth, 5000);
    const s = setInterval(refreshSessions, 4000);
    const r = setInterval(refreshRuns, 4000);
    if (location.hash.length > 1) select(location.hash.slice(1));
    const onHash = () => {
      const id = location.hash.slice(1);
      if (id && id !== useStore.getState().currentSid) select(id);
    };
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
      <Toasts />
    </div>
  );
}
