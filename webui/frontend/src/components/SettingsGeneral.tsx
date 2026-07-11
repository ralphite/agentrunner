import { useState } from "react";
import { useStore } from "../store";
import { resetAll } from "../theme";
import { matchesQuery } from "./SettingsSearch";

// SettingsGeneral is the Settings landing (Codex's Settings → General). Kept to
// things this slice can honestly own: an at-a-glance status line and a real
// "reset all settings" that clears the appearance + git prefs it persists.
export function SettingsGeneral({ query, onReset }: { query: string; onReset: () => void }) {
  const health = useStore((s) => s.health);
  const sessions = useStore((s) => s.sessions);
  const [confirming, setConfirming] = useState(false);

  const show = (s: string) => matchesQuery(query, s);
  const any = show("status daemon connection tasks") || show("reset settings defaults appearance");

  const doReset = () => {
    resetAll();
    setConfirming(false);
    onReset();
  };

  return (
    <div className="rs-panel">
      <h2 className="rs-panel-title">General</h2>
      <p className="rs-panel-sub">AgentRunner settings for this browser.</p>

      {!any && <div className="rs-noresults">No general settings match “{query}”.</div>}

      {show("status daemon connection tasks") && (
        <section className="rs-row">
          <div className="rs-row-head">
            <div className="rs-row-label">Status</div>
            <div className="rs-row-desc">
              {health?.daemonUp ? "Connected to the daemon." : "Daemon unavailable."} {sessions.length} task{sessions.length === 1 ? "" : "s"} loaded.
            </div>
          </div>
          <span className={"rs-statusdot" + (health?.daemonUp ? " on" : "")} aria-hidden />
        </section>
      )}

      {show("reset settings defaults appearance") && (
        <section className="rs-row rs-row-block">
          <div className="rs-row-head">
            <div className="rs-row-label">Reset settings</div>
            <div className="rs-row-desc">Restore appearance and Git defaults. Doesn’t touch your tasks or workspaces.</div>
          </div>
          {confirming ? (
            <div className="rs-confirm">
              <span>Reset all settings to defaults?</span>
              <button className="rs-btn danger" onClick={doReset}>
                Reset
              </button>
              <button className="rs-btn" onClick={() => setConfirming(false)}>
                Cancel
              </button>
            </div>
          ) : (
            <button className="rs-btn" onClick={() => setConfirming(true)}>
              Reset to defaults
            </button>
          )}
        </section>
      )}
    </div>
  );
}
