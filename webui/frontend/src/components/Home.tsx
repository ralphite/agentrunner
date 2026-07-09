import { useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";
import { DEFAULT_SPEC, DEFAULT_WORKER } from "../specs";
import { pillClass } from "./pill";
import { relTime, sessionDate } from "../time";

// Home mirrors Codex's landing: a heading + one rounded composer card with a
// submit arrow, an inline mode/workspace control row, and a grid of task cards.
export function Home() {
  const { sessions, runs, select, selectRun, openModal, refreshSessions, refreshRuns, toast } =
    useStore();
  const [text, setText] = useState("");
  const [ws, setWs] = useState("");
  const [mode, setMode] = useState<"chat" | "submit">("chat");
  const [busy, setBusy] = useState(false);

  const ensureWs = async (): Promise<string> => {
    if (ws.trim()) return ws.trim();
    const p = (await AR.makeWorkspace()).path;
    setWs(p);
    return p;
  };

  const submit = async () => {
    const t = text.trim();
    if (!t || busy) return;
    setBusy(true);
    try {
      const workspace = await ensureWs();
      if (mode === "chat") {
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
      } else {
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
      }
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  const wsShort = ws ? ws.split("/").filter(Boolean).slice(-1)[0] : "自动创建";

  const cards = [
    ...sessions.map((s) => ({
      key: "s" + s.id,
      title: s.title || s.id,
      time: relTime(sessionDate(s.id)),
      cls: pillClass(s.status),
      badge: s.status,
      onClick: () => select(s.id),
    })),
    ...runs.map((r) => ({
      key: "r" + r.id,
      title: r.label || r.id,
      time: r.kind,
      cls: r.status,
      badge: r.status,
      onClick: () => selectRun(r.id),
    })),
  ];

  return (
    <div className="home">
      <div className="hero">
        <h2>接下来做点什么？</h2>
        <div className="cx-composer">
          <textarea
            value={text}
            placeholder="随便描述一个任务，或问一个问题…"
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit();
            }}
          />
          <div className="cx-controls">
            <button className="cx-plus" title="自定义 spec" onClick={() => openModal({ kind: "new", message: text })}>
              ＋
            </button>
            <select className="cx-chip" value={mode} onChange={(e) => setMode(e.target.value as any)}>
              <option value="chat">对话</option>
              <option value="submit">后台任务</option>
            </select>
            <span className="spacer" />
            <span className="cx-model">gemini-flash</span>
            <button className="cx-send" disabled={busy || !text.trim()} onClick={submit} title="提交 (⌘↵)">
              ↑
            </button>
          </div>
          <div className="cx-context">
            <span className="cx-ctx-item" title={ws}>
              🗂 {wsShort}
            </span>
            <button className="cx-ctx-btn" onClick={async () => setWs((await AR.makeWorkspace()).path)}>
              造空 workspace
            </button>
            <input
              className="cx-ws-input"
              value={ws}
              placeholder="或填 workspace 绝对路径…"
              onChange={(e) => setWs(e.target.value)}
            />
          </div>
        </div>
      </div>

      <div className="tasklist">
        <div className="grp-label">任务 · {cards.length}</div>
        {cards.length === 0 ? (
          <div className="dim">还没有任务。用上面的 composer 开一个。</div>
        ) : (
          <div className="task-grid">
            {cards.map((c) => (
              <div className="task-card" key={c.key} onClick={c.onClick}>
                <div className="tc-title">{c.title}</div>
                <div className="tc-sub">
                  <span className={"nr-dot " + c.cls} />
                  <span className="dim">{c.badge}</span>
                  <span className="spacer" />
                  <span className="dim">{c.time}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
