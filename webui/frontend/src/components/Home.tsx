import { useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";
import { DEFAULT_SPEC, DEFAULT_WORKER } from "../specs";
import { friendlyStatus } from "./pill";
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

  const wsShort = ws ? ws.split("/").filter(Boolean).slice(-1)[0] : "auto-created";

  const cards = [
    ...sessions.map((s) => ({
      key: "s" + s.id,
      title: s.title || s.id,
      time: relTime(sessionDate(s.id)),
      cls: friendlyStatus(s.status).cls,
      badge: friendlyStatus(s.status).text,
      onClick: () => select(s.id),
    })),
    ...runs.map((r) => ({
      key: "r" + r.id,
      title: r.label || r.id,
      time: r.kind,
      cls: friendlyStatus(r.status).cls,
      badge: friendlyStatus(r.status).text,
      onClick: () => selectRun(r.id),
    })),
  ];

  return (
    <div className="home">
      <div className="hero">
        <h2>What should we do next?</h2>
        <div className="cx-composer">
          <textarea
            value={text}
            placeholder="Describe a task, or ask a question…"
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit();
            }}
          />
          <div className="cx-controls">
            <button className="cx-plus" title="advanced: edit the agent spec YAML before starting" onClick={() => openModal({ kind: "new", message: text })}>
              ＋
            </button>
            <select className="cx-chip" value={mode} onChange={(e) => setMode(e.target.value as any)}>
              <option value="chat">Chat</option>
              <option value="submit">Background task</option>
            </select>
            <span className="spacer" />
            <span className="cx-model">gemini-flash</span>
            <button className="cx-send" disabled={busy || !text.trim()} onClick={submit} title="submit (⌘↵)">
              ↑
            </button>
          </div>
          <div className="cx-context">
            <span className="cx-ctx-item" title={ws}>
              🗂 {wsShort}
            </span>
            <button className="cx-ctx-btn" onClick={async () => setWs((await AR.makeWorkspace()).path)}>
              make empty workspace
            </button>
            <input
              className="cx-ws-input"
              value={ws}
              placeholder="or an absolute workspace path…"
              onChange={(e) => setWs(e.target.value)}
            />
          </div>
        </div>
      </div>

      <div className="tasklist">
        <div className="grp-label">Tasks · {cards.length}</div>
        {cards.length === 0 ? (
          <div className="dim">No tasks yet. Start one with the composer above.</div>
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
