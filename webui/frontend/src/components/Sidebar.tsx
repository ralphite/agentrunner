import { useState } from "react";
import { useStore } from "../store";
import { AR } from "../api";
import { pillClass } from "./pill";

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
  const [tab, setTab] = useState<"sessions" | "runs">("sessions");

  const restartDaemon = async () => {
    try {
      await AR.daemonStart();
      toast("daemon 启动请求已发", "info");
      setTimeout(refreshHealth, 800);
    } catch (e: any) {
      toast(e.message);
    }
  };

  return (
    <div className="sidebar">
      <div className="brand" onClick={() => select(null)} style={{ cursor: "pointer" }}>
        <div className="logo">◆</div>
        <h1>AgentRunner</h1>
      </div>
      <div className="health">
        <span className={"dot" + (health?.daemonUp ? " up" : "")} />
        {health ? (
          <span>
            {health.version} · daemon {health.daemonUp ? "在线" : "不可达"}
            {health.daemonManaged ? "(托管)" : ""}
          </span>
        ) : (
          <span>arwebui 不可达</span>
        )}
        {health && !health.daemonUp && (
          <button className="sm danger" style={{ marginLeft: "auto" }} onClick={restartDaemon}>
            重启
          </button>
        )}
      </div>

      <div className="composer-cta">
        <button onClick={() => openModal({ kind: "new" })}>
          <span>＋</span> 新任务 / 会话
        </button>
      </div>

      <div className="side-tabs">
        <button className={tab === "sessions" ? "on" : ""} onClick={() => setTab("sessions")}>
          会话 {sessions.length ? `· ${sessions.length}` : ""}
        </button>
        <button className={tab === "runs" ? "on" : ""} onClick={() => setTab("runs")}>
          后台运行 {runs.length ? `· ${runs.length}` : ""}
        </button>
      </div>

      <div className="list">
        {tab === "sessions" &&
          (sessions.length === 0 ? (
            <div className="list-empty">还没有会话</div>
          ) : (
            sessions.map((s) => (
              <div
                key={s.id}
                className={"card-row" + (s.id === currentSid ? " cur" : "")}
                onClick={() => select(s.id)}
                title={s.id}
              >
                <div className="title">{s.title || s.id}</div>
                <div className="sub">
                  <span className={"pill " + pillClass(s.status)}>{s.status}</span>
                  <span>· {s.turns} 轮</span>
                </div>
              </div>
            ))
          ))}

        {tab === "runs" &&
          (runs.length === 0 ? (
            <div className="list-empty">还没有后台运行 (submit / drive)</div>
          ) : (
            runs.map((r) => (
              <div
                key={r.id}
                className={"card-row" + (r.id === currentRunId ? " cur" : "")}
                onClick={() => selectRun(r.id)}
              >
                <div className="title">{r.label || r.id}</div>
                <div className="sub">
                  <span className={"pill " + r.status}>{r.status}</span>
                  <span>· {r.kind}</span>
                </div>
              </div>
            ))
          ))}
      </div>

      <div className="composer-cta" style={{ borderTop: "1px solid var(--line)" }}>
        <button onClick={() => openModal({ kind: "trust" })}>信任 workspace 目录…</button>
      </div>
    </div>
  );
}
