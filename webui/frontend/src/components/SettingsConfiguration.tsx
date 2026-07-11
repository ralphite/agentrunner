import { useStore } from "../store";
import { matchesQuery } from "./SettingsSearch";

// SettingsConfiguration is Codex's Settings → Configuration (INC-41 H5),
// read-only. Everything here comes from the /health payload we already poll —
// version, runtime dir, daemon topology, log path. Approval policy / sandbox
// aren't surfaced by the backend, so they're noted, not invented.
export function SettingsConfiguration({ query }: { query: string }) {
  const health = useStore((s) => s.health);

  const daemonMode = !health
    ? "—"
    : !health.daemonUp
      ? "Unavailable"
      : health.daemonExternal
        ? "External (shared)"
        : health.daemonManaged
          ? "Managed by this UI"
          : "Running";

  const rows: { label: string; value: string; kw?: string }[] = [
    { label: "Version", value: health?.version || "unknown", kw: "build" },
    { label: "Daemon", value: daemonMode, kw: "server status connection" },
    { label: "Runtime directory", value: health?.runtimeDir || "—", kw: "path store data" },
    { label: "Daemon log", value: health?.daemonLogPath || "—", kw: "logs path" },
  ].filter((r) => matchesQuery(query, r.label + " " + r.value + " " + (r.kw || "")));

  const showPolicy = matchesQuery(query, "approval policy sandbox permissions");

  return (
    <div className="rs-panel">
      <h2 className="rs-panel-title">Configuration</h2>
      <p className="rs-panel-sub">Live daemon and runtime details reported by AgentRunner.</p>

      {rows.length === 0 && !showPolicy && <div className="rs-noresults">No configuration matches “{query}”.</div>}

      {rows.length > 0 && (
        <dl className="rs-kv">
          {rows.map((r) => (
            <div className="rs-kv-row" key={r.label}>
              <dt>{r.label}</dt>
              <dd className="mono" title={r.value}>
                {r.value}
              </dd>
            </div>
          ))}
        </dl>
      )}

      {showPolicy && (
        <section className="rs-row rs-row-block">
          <div className="rs-row-head">
            <div className="rs-row-label">
              Approval policy &amp; sandbox <span className="rs-todo">Not surfaced</span>
            </div>
            <div className="rs-row-desc">
              Per-task approval mode is chosen when starting a task; the daemon doesn’t expose a global policy to read here yet.
            </div>
          </div>
        </section>
      )}
    </div>
  );
}
