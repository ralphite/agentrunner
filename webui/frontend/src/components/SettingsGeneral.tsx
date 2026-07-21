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
  const any = show("status daemon connection sessions") || show("reset settings defaults appearance");

  const doReset = () => {
    resetAll();
    setConfirming(false);
    onReset();
  };

  // SETTINGS-GENERAL-CHROME (R81): the landing panel used to hand-roll a 19px
  // heading and borderless `border-t` list rows, so the first Settings page a
  // user sees looked like a different, earlier surface than the six sibling
  // panels — which all share `rs-panel-title` (22px) + bordered `rs-row` cards.
  // Codex runs one settings design system across every section; General now
  // joins it (rs-panel / rs-panel-title / rs-panel-sub / rs-row-block) so the
  // left rail never changes the visual language of the content pane.
  return (
    <div className="rs-panel min-w-0">
      <h2 className="rs-panel-title">General</h2>
      <p className="rs-panel-sub leading-[1.5]">AgentRunner settings for this browser.</p>

      {!any && <div className="rs-noresults">No general settings match “{query}”.</div>}

      {show("status daemon connection sessions") && (
        <section className="rs-row rs-row-block min-w-0 max-[500px]:rounded-[8px] max-[500px]:p-2.5">
          <div className="rs-row-head min-w-0 max-[500px]:flex-col max-[500px]:items-stretch max-[500px]:gap-1">
            <div className="min-w-0">
              <div className="rs-row-label">Status</div>
              <div className="rs-row-desc mt-[3px] leading-[1.5]">
                {health?.daemonUp ? "Connected to the daemon." : "Daemon unavailable."} {sessions.length} session{sessions.length === 1 ? "" : "s"} loaded.
              </div>
            </div>
            <span className={"w-[9px] h-[9px] rounded-full shrink-0 max-[500px]:self-start " + (health?.daemonUp ? "bg-green" : "bg-red")} aria-hidden />
          </div>
        </section>
      )}

      {show("reset settings defaults appearance") && (
        <section className="rs-row rs-row-block min-w-0 max-[500px]:rounded-[8px] max-[500px]:p-2.5">
          <div className="rs-row-head min-w-0 max-[500px]:flex-col max-[500px]:items-stretch max-[500px]:gap-2">
            <div className="min-w-0">
              <div className="rs-row-label">Reset settings</div>
              <div className="rs-row-desc mt-[3px] leading-[1.5]">Restore appearance and Git defaults. Doesn’t touch your sessions or workspaces.</div>
            </div>
            {confirming ? (
              <div className="flex shrink-0 flex-col items-end gap-[8px] text-[12.5px] text-dim max-[500px]:items-stretch">
                <span className="max-[500px]:leading-[1.5]">Reset all settings to defaults?</span>
                <div className="flex items-center justify-end gap-[8px] max-[500px]:grid max-[500px]:grid-cols-2">
                  <button className="px-[15px] py-[7px] border rounded-full bg-panel text-red border-[color-mix(in_srgb,var(--red)_40%,var(--line))] text-[13px] shrink-0 hover:bg-panel-2" onClick={doReset}>
                    Reset
                  </button>
                  <button className="px-[15px] py-[7px] border border-line rounded-full bg-panel text-ink text-[13px] shrink-0 hover:bg-panel-2" onClick={() => setConfirming(false)}>
                    Cancel
                  </button>
                </div>
              </div>
            ) : (
              <button className="px-[15px] py-[7px] border border-line rounded-full bg-panel text-ink text-[13px] shrink-0 hover:bg-panel-2 max-[500px]:self-start" onClick={() => setConfirming(true)}>
                Reset to defaults
              </button>
            )}
          </div>
        </section>
      )}
    </div>
  );
}
