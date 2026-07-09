import { useMemo, useState } from "react";
import { useStore } from "../store";
import { AR } from "../api";
import { pillClass } from "./pill";
import { bucketOf, relTime, sessionDate } from "../time";

export function Sidebar() {
  const {
    health,
    sessions,
    runs,
    currentSid,
    currentRunId,
    select,
    selectRun,
    openModal,
    refreshHealth,
    toast,
  } = useStore();
  const [q, setQ] = useState("");
  const [searching, setSearching] = useState(false);

  const restartDaemon = async () => {
    try {
      await AR.daemonStart();
      toast("daemon start requested", "info");
      setTimeout(refreshHealth, 800);
    } catch (e: any) {
      toast(e.message);
    }
  };

  const ql = q.trim().toLowerCase();
  const shownSessions = useMemo(
    () =>
      sessions.filter(
        (s) => !ql || (s.title || s.id).toLowerCase().includes(ql) || s.id.toLowerCase().includes(ql),
      ),
    [sessions, ql],
  );
  const shownRuns = useMemo(
    () => runs.filter((r) => !ql || (r.label || r.id).toLowerCase().includes(ql)),
    [runs, ql],
  );

  // Codex groups tasks by recency, and floats anything in-progress to the top.
  const groups = useMemo(() => {
    const rows = shownSessions.map((s) => ({ s, d: sessionDate(s.id), cls: pillClass(s.status) }));
    const isActive = (cls: string) => cls === "run" || cls === "appr";
    const byDate = (a: { d: Date | null }, b: { d: Date | null }) =>
      (b.d?.getTime() || 0) - (a.d?.getTime() || 0);
    const active = rows.filter((x) => isActive(x.cls)).sort(byDate);
    const rest = rows.filter((x) => !isActive(x.cls));
    const buckets = new Map<string, { rank: number; items: typeof rest }>();
    for (const x of rest) {
      const b = bucketOf(x.d);
      if (!buckets.has(b.label)) buckets.set(b.label, { rank: b.rank, items: [] });
      buckets.get(b.label)!.items.push(x);
    }
    const out: { label: string; items: typeof rest }[] = [];
    if (active.length) out.push({ label: "Active", items: active });
    for (const [label, v] of [...buckets.entries()].sort((a, b) => a[1].rank - b[1].rank)) {
      out.push({ label, items: v.items.sort(byDate) });
    }
    return out;
  }, [shownSessions]);

  return (
    <div className="sidebar">
      <div className="brand" onClick={() => select(null)}>
        <div className="logo">◆</div>
        <h1>AgentRunner</h1>
      </div>

      <nav className="side-nav">
        <button className="nav-item" onClick={() => select(null)}>
          <span className="ni-ico">✎</span> New task
        </button>
        <button className={"nav-item" + (searching ? " on" : "")} onClick={() => setSearching((v) => !v)}>
          <span className="ni-ico">⌕</span> Search
        </button>
        <button className="nav-item" onClick={() => openModal({ kind: "run" })}>
          <span className="ni-ico">⧉</span> Background run…
        </button>
        <button className="nav-item" onClick={() => openModal({ kind: "trust" })}>
          <span className="ni-ico">⛨</span> Trust directory
        </button>
      </nav>

      {searching && (
        <div className="side-search">
          <input
            autoFocus
            value={q}
            placeholder="Search tasks…"
            onChange={(e) => setQ(e.target.value)}
          />
          {q && (
            <button className="ss-clear" onClick={() => setQ("")} title="clear">
              ✕
            </button>
          )}
        </div>
      )}

      <div className="list">
        {groups.length === 0 && shownRuns.length === 0 && (
          <div className="grp-empty">{ql ? "no matches" : "no tasks yet"}</div>
        )}

        {groups.map((g) => (
          <div key={g.label}>
            <div className={"grp-label" + (g.label === "Active" ? " active" : "")}>{g.label}</div>
            {g.items.map(({ s }) => (
              <div
                key={s.id}
                className={"nav-row" + (s.id === currentSid ? " cur" : "")}
                onClick={() => select(s.id)}
                title={s.id}
              >
                <span className={"nr-dot " + pillClass(s.status)} title={s.status} />
                <span className="nr-title">{s.title || s.id}</span>
                <span className="nr-time">{relTime(sessionDate(s.id))}</span>
              </div>
            ))}
          </div>
        ))}

        {shownRuns.length > 0 && (
          <>
            <div className="grp-label" style={{ marginTop: 10 }}>
              Background runs · {shownRuns.length}
            </div>
            {shownRuns.map((r) => (
              <div
                key={r.id}
                className={"nav-row" + (r.id === currentRunId ? " cur" : "")}
                onClick={() => selectRun(r.id)}
                title={r.kind}
              >
                <span className={"nr-dot " + pillClass(r.status)} title={r.status} />
                <span className="nr-title">{r.label || r.id}</span>
                <span className="nr-time">{r.kind}</span>
              </div>
            ))}
          </>
        )}
      </div>

      <div className="side-foot">
        <span className={"dot" + (health?.daemonUp ? " up" : "")} />
        <span className="sf-text">
          {health ? (
            <>
              daemon {health.daemonUp ? "up" : "unreachable"}
              {health.daemonManaged ? " (managed)" : health.daemonExternal ? " (external)" : ""} · {health.version.replace("agentrunner ", "")}
            </>
          ) : (
            "arwebui unreachable"
          )}
        </span>
        {health && !health.daemonUp && (
          <button className="sm danger" onClick={restartDaemon}>
            restart
          </button>
        )}
      </div>
    </div>
  );
}
