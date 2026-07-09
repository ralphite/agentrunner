import { useMemo, useState } from "react";
import { useStore } from "../store";
import { AR } from "../api";
import { pillClass } from "./pill";
import { relTime, sessionDate } from "../time";

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
      toast("daemon 启动请求已发", "info");
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

  return (
    <div className="sidebar">
      <div className="brand" onClick={() => select(null)}>
        <div className="logo">◆</div>
        <h1>AgentRunner</h1>
      </div>

      <nav className="side-nav">
        <button className="nav-item" onClick={() => select(null)}>
          <span className="ni-ico">✎</span> 新建任务
        </button>
        <button className="nav-item" onClick={() => setSearching((v) => !v)}>
          <span className="ni-ico">⌕</span> 搜索
        </button>
        <button className="nav-item" onClick={() => openModal({ kind: "run" })}>
          <span className="ni-ico">⧉</span> 后台运行…
        </button>
        <button className="nav-item" onClick={() => openModal({ kind: "trust" })}>
          <span className="ni-ico">⛨</span> 信任目录
        </button>
      </nav>

      {searching && (
        <div className="side-search">
          <input
            autoFocus
            value={q}
            placeholder="搜索任务…"
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
      )}

      <div className="list">
        <div className="grp-label">会话</div>
        {shownSessions.length === 0 ? (
          <div className="grp-empty">无</div>
        ) : (
          shownSessions.map((s) => (
            <div
              key={s.id}
              className={"nav-row" + (s.id === currentSid ? " cur" : "")}
              onClick={() => select(s.id)}
              title={s.id}
            >
              <span className="nr-title">{s.title || s.id}</span>
              <span className="nr-time">{relTime(sessionDate(s.id))}</span>
              <span className={"nr-dot " + pillClass(s.status)} title={s.status} />
            </div>
          ))
        )}

        <div className="grp-label" style={{ marginTop: 14 }}>
          后台运行
        </div>
        {shownRuns.length === 0 ? (
          <div className="grp-empty">无</div>
        ) : (
          shownRuns.map((r) => (
            <div
              key={r.id}
              className={"nav-row" + (r.id === currentRunId ? " cur" : "")}
              onClick={() => selectRun(r.id)}
              title={r.kind}
            >
              <span className="nr-title">{r.label || r.id}</span>
              <span className="nr-time">{r.kind}</span>
              <span className={"nr-dot " + r.status} title={r.status} />
            </div>
          ))
        )}
      </div>

      <div className="side-foot">
        <span className={"dot" + (health?.daemonUp ? " up" : "")} />
        <span className="sf-text">
          {health ? (
            <>
              daemon {health.daemonUp ? "在线" : "不可达"}
              {health.daemonManaged ? "(托管)" : ""} · {health.version.replace("agentrunner ", "")}
            </>
          ) : (
            "arwebui 不可达"
          )}
        </span>
        {health && !health.daemonUp && (
          <button className="sm danger" onClick={restartDaemon}>
            重启
          </button>
        )}
      </div>
    </div>
  );
}
