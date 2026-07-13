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

  return (
    <div className="max-w-[660px] mx-auto">
      <h2 className="text-[19px] font-[650] m-0 mb-[4px]">General</h2>
      <p className="m-0 mb-[22px] text-dim text-[13px] leading-[1.5]">AgentRunner settings for this browser.</p>

      {!any && <div className="text-dim text-[13px] py-[8px]">No general settings match “{query}”.</div>}

      {show("status daemon connection sessions") && (
        <section className="flex items-center justify-between gap-[22px] py-[16px] border-t border-line-2 first-of-type:border-t-0 max-[500px]:flex-col max-[500px]:items-stretch max-[500px]:gap-[10px]">
          <div className="min-w-0">
            <div className="flex items-center gap-[8px] text-[14px] text-ink">Status</div>
            <div className="mt-[3px] text-[12.5px] text-dim leading-[1.5]">
              {health?.daemonUp ? "Connected to the daemon." : "Daemon unavailable."} {sessions.length} session{sessions.length === 1 ? "" : "s"} loaded.
            </div>
          </div>
          <span className={"w-[9px] h-[9px] rounded-full shrink-0 max-[500px]:self-start " + (health?.daemonUp ? "bg-green" : "bg-red")} aria-hidden />
        </section>
      )}

      {show("reset settings defaults appearance") && (
        <section className="flex items-center justify-between gap-[22px] py-[16px] border-t border-line-2 first-of-type:border-t-0 max-[500px]:flex-col max-[500px]:items-stretch max-[500px]:gap-[12px]">
          <div className="min-w-0">
            <div className="flex items-center gap-[8px] text-[14px] text-ink">Reset settings</div>
            <div className="mt-[3px] text-[12.5px] text-dim leading-[1.5]">Restore appearance and Git defaults. Doesn’t touch your sessions or workspaces.</div>
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
        </section>
      )}
    </div>
  );
}
