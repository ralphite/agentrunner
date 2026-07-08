import { useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";
import { DEFAULT_SPEC, DEFAULT_WORKER } from "../specs";
import { pillClass } from "./pill";

// Home is the Codex-style landing: a big centred composer with Ask / Code and
// a grid of recent task cards. "Ask" starts a conversational session; "Code"
// hands the task to the daemon as a background run — mirroring Codex's split.
export function Home() {
  const { sessions, runs, select, selectRun, openModal, refreshSessions, refreshRuns, toast } =
    useStore();
  const [text, setText] = useState("");
  const [ws, setWs] = useState("");
  const [busy, setBusy] = useState(false);

  const ensureWs = async (): Promise<string> => {
    if (ws.trim()) return ws.trim();
    const p = (await AR.makeWorkspace()).path;
    setWs(p);
    return p;
  };

  const ask = async () => {
    const t = text.trim();
    if (!t) return;
    setBusy(true);
    try {
      const workspace = await ensureWs();
      const r = await AR.newSession({
        spec: DEFAULT_SPEC,
        extraSpecs: [{ name: "worker.yaml", content: DEFAULT_WORKER }],
        workspace,
        message: t,
        mode: "",
      });
      setText("");
      await refreshSessions();
      select(r.sid);
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  const code = async () => {
    const t = text.trim();
    if (!t) return;
    setBusy(true);
    try {
      const workspace = await ensureWs();
      const r = await AR.startRun({
        kind: "submit",
        spec: DEFAULT_SPEC,
        extraSpecs: [],
        task: t,
        workspace,
        mode: "",
        idem: "",
      });
      setText("");
      await refreshRuns();
      selectRun(r.runId);
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  const cards = [
    ...sessions.map((s) => ({
      key: "s" + s.id,
      title: s.title || s.id,
      sub: `${s.status} · ${s.turns} 轮`,
      cls: pillClass(s.status),
      badge: s.status,
      onClick: () => select(s.id),
    })),
    ...runs.map((r) => ({
      key: "r" + r.id,
      title: r.label || r.id,
      sub: `${r.kind} · 后台运行`,
      cls: r.status,
      badge: r.status,
      onClick: () => selectRun(r.id),
    })),
  ];

  return (
    <div className="home">
      <div className="hero">
        <h2>接下来做什么？</h2>
        <div className="hero-composer">
          <textarea
            value={text}
            placeholder="描述一个任务，或问一个问题…"
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) code();
            }}
          />
          <div className="hero-controls">
            <input
              type="text"
              className="ws"
              value={ws}
              placeholder="workspace(留空自动创建)"
              onChange={(e) => setWs(e.target.value)}
            />
            <span className="spacer" />
            <a onClick={() => openModal({ kind: "new", message: text })}>自定义 spec…</a>
            <button disabled={busy || !text.trim()} onClick={ask}>
              Ask
            </button>
            <button className="primary" disabled={busy || !text.trim()} onClick={code}>
              Code ↵
            </button>
          </div>
        </div>
      </div>

      <div className="tasklist">
        <h3>任务 · {cards.length}</h3>
        {cards.length === 0 ? (
          <div className="dim">还没有任务。用上面的 composer 开一个。</div>
        ) : (
          <div className="task-grid">
            {cards.map((c) => (
              <div className="task-card" key={c.key} onClick={c.onClick}>
                <div className="tc-title">{c.title}</div>
                <div className="tc-sub">
                  <span className={"pill " + c.cls}>{c.badge}</span>
                  <span className="dim">{c.sub}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
