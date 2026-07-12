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
    <div className="rs-panel max-w-[660px] mx-auto">
      <h2 className="rs-panel-title m-0 mb-[4px] text-[19px] font-[650]">Configuration</h2>
      <p className="rs-panel-sub m-0 mb-[22px] text-dim text-[13px] leading-[1.5]">Live daemon and runtime details reported by AgentRunner.</p>

      {rows.length === 0 && !showPolicy && <div className="rs-noresults text-dim text-[13px] py-[8px]">No configuration matches “{query}”.</div>}

      {rows.length > 0 && (
        <dl className="rs-kv m-0 border border-line rounded-[12px] overflow-hidden">
          {rows.map((r) => (
            <div className="rs-kv-row grid grid-cols-[160px_1fr] gap-[14px] px-[14px] py-[11px] border-t border-line-2 first:border-t-0" key={r.label}>
              <dt className="text-dim text-[12.5px]">{r.label}</dt>
              <dd className="mono m-0 text-[12.5px] text-ink overflow-hidden text-ellipsis break-all" title={r.value}>
                {r.value}
              </dd>
            </div>
          ))}
        </dl>
      )}

      {showPolicy && (
        <section className="rs-row rs-row-block flex flex-col items-stretch justify-between gap-[12px] py-[16px] border-t border-line-2 first-of-type:border-t-0">
          <div className="rs-row-head min-w-0">
            <div className="rs-row-label flex items-center gap-[8px] text-[14px] text-ink">
              Approval policy &amp; sandbox{" "}
              <span className="rs-todo text-[10px] font-semibold uppercase tracking-[0.4px] px-[7px] py-[2px] rounded-full text-dim bg-panel-2 border border-line">
                Not surfaced
              </span>
            </div>
            <div className="rs-row-desc mt-[3px] text-[12.5px] text-dim leading-[1.5]">
              Per-task approval mode is chosen when starting a task; the daemon doesn’t expose a global policy to read here yet.
            </div>
          </div>
        </section>
      )}
    </div>
  );
}
