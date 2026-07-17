import { useState } from "react";
import { ArrowClockwise, WarningCircle } from "@phosphor-icons/react";
import { AR } from "../api";
import { useStore } from "../store";

// Shared J5 notification strip. It is intentionally driven by the same health
// record as the sidebar footer, so Home and an open task never disagree about
// whether actions are currently available.
// `home` swaps the session-view margins (old `.daemon-alert` base rule) for the
// Home hero's flush left-aligned placement (old `.home .daemon-alert` rule).
export function DaemonAlert({ home = false }: { home?: boolean }) {
  const health = useStore((state) => state.health);
  const refreshHealth = useStore((state) => state.refreshHealth);
  const toast = useStore((state) => state.toast);
  const [retrying, setRetrying] = useState(false);

  if (!health || health.daemonUp) return null;

  const retry = async () => {
    setRetrying(true);
    try {
      await AR.daemonStart();
      toast("daemon start requested", "info");
    } catch (error: any) {
      toast(error.message);
    }
    window.setTimeout(() => {
      void refreshHealth();
      setRetrying(false);
    }, 800);
  };

  return (
    <div className="daemon-alert" role="alert">
      <span className="daemon-alert-ic"><WarningCircle size={17} weight="fill" /></span>
      <div className="daemon-alert-text">
        <b>Daemon offline</b>
        <span>AgentRunner can’t reach the daemon. Live updates and actions are paused.</span>
      </div>
      <button type="button" className="daemon-alert-retry" onClick={retry} disabled={retrying}>
        <ArrowClockwise size={14} /> {retrying ? "Retrying…" : "Retry"}
      </button>
    </div>
  );
}
