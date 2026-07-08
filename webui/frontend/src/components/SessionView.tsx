import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";
import type { Envelope, Task } from "../types";
import { foldEvents, type ApprovalRef } from "../timeline";
import { TimelineView } from "./Timeline";
import { ApprovalCard } from "./ApprovalCard";
import { Composer } from "./Composer";

interface SSEApproval {
  id: string;
  tool: string;
  args: any;
  agent?: string;
}

export function SessionView({ sid }: { sid: string }) {
  const { select, openModal, toast, refreshSessions, showSys, toggleSys } = useStore();
  const isSub = sid.includes("-sub-");

  const [events, setEvents] = useState<Envelope[]>([]);
  const [pending, setPending] = useState<{ id: number; text: string; images: number }[]>([]);
  const [typing, setTyping] = useState<string>("");
  const [sseApprovals, setSseApprovals] = useState<Map<string, SSEApproval>>(new Map());
  const [resolvedLocal, setResolvedLocal] = useState<Set<string>>(new Set());
  const [tasks, setTasks] = useState<Task[]>([]);
  const [closeArmed, setCloseArmed] = useState(false);

  const cursor = useRef(0);
  const pollBusy = useRef(false);
  const pendSeq = useRef(0);

  // ---- incremental journal poll (the realtime source of truth) ----
  const poll = useCallback(async () => {
    if (pollBusy.current) return;
    pollBusy.current = true;
    try {
      const evs = await AR.events(sid, cursor.current);
      if (evs.length) {
        setPending((prev) => {
          let next = prev;
          for (const e of evs) {
            if (e.type === "input_received") {
              const t = e.payload?.text;
              const i = next.findIndex((x) => x.text === t);
              if (i >= 0) next = next.filter((_, j) => j !== i);
            }
            if (e.type === "assistant_message") setTyping("");
          }
          return next;
        });
        setEvents((prev) => [...prev, ...evs]);
        cursor.current = evs.reduce((m, e) => Math.max(m, e.seq || 0), cursor.current);
      }
    } catch {
      /* daemon down / transient: health dot tells the story */
    } finally {
      pollBusy.current = false;
    }
  }, [sid]);

  const pollTasks = useCallback(async () => {
    try {
      setTasks(await AR.ps(sid));
    } catch {
      /* ignore */
    }
  }, [sid]);

  useEffect(() => {
    cursor.current = 0;
    setEvents([]);
    setPending([]);
    setTyping("");
    setSseApprovals(new Map());
    setResolvedLocal(new Set());
    poll();
    const e = setInterval(poll, 1000);
    const t = setInterval(pollTasks, 2500);
    pollTasks();
    let es: EventSource | null = null;
    if (!isSub) {
      es = new EventSource(`/api/sessions/${sid}/stream`);
      es.onmessage = (m) => {
        let ev: any;
        try {
          ev = JSON.parse(m.data);
        } catch {
          return;
        }
        if (ev.kind === "text_delta" && ev.text) setTyping((prev) => prev + ev.text);
        if (ev.kind === "discard") setTyping("");
        // Child asks exist ONLY on this stream (they never touch the parent
        // journal). e.text carries the requesting agent's name.
        if (ev.kind === "approval_request" && ev.approval_id) {
          setSseApprovals((prev) => {
            const next = new Map(prev);
            next.set(ev.approval_id, {
              id: ev.approval_id,
              tool: ev.tool,
              args: ev.args,
              agent: ev.text,
            });
            return next;
          });
        }
      };
      es.addEventListener("end", () => es?.close());
    }
    return () => {
      clearInterval(e);
      clearInterval(t);
      es?.close();
    };
  }, [sid, isSub, poll, pollTasks]);

  const folded = useMemo(() => foldEvents(events), [events]);

  // Open approvals = journal asks not yet resolved + SSE-only child asks.
  const openApprovals: (ApprovalRef & { agent?: string; viaSSE?: boolean })[] = [];
  for (const a of folded.approvals.values()) {
    if (!a.resolved && !resolvedLocal.has(a.id)) openApprovals.push(a);
  }
  for (const s of sseApprovals.values()) {
    if (folded.approvals.has(s.id) || resolvedLocal.has(s.id)) continue;
    openApprovals.push({ id: s.id, tool: s.tool, args: s.args, gates: [], agent: s.agent, viaSSE: true });
  }

  const status = folded.status;

  const doSend = async (text: string, images: string[]) => {
    const id = ++pendSeq.current;
    setPending((p) => [...p, { id, text, images: images.length }]);
    try {
      await AR.send(sid, text, images);
    } catch (e: any) {
      toast(e.message);
      setPending((p) => p.filter((x) => x.id !== id));
    }
  };

  const decideApproval = async (id: string, decision: "approve" | "deny", reason: string) => {
    await AR.approve(sid, id, decision, reason);
    setResolvedLocal((s) => new Set(s).add(id));
  };

  const act = {
    interrupt: async () => {
      try {
        await AR.interrupt(sid);
        toast("已发 interrupt", "info");
      } catch (e: any) {
        toast(e.message);
      }
    },
    resume: async () => {
      try {
        await AR.resume(sid);
        toast("已发 resume", "info");
      } catch (e: any) {
        toast(e.message);
      }
    },
    close: async () => {
      if (!closeArmed) {
        setCloseArmed(true);
        setTimeout(() => setCloseArmed(false), 3000);
        return;
      }
      setCloseArmed(false);
      try {
        await AR.close(sid);
        refreshSessions();
      } catch (e: any) {
        toast(e.message);
      }
    },
    kill: async (handle: string) => {
      try {
        await AR.kill(sid, handle);
      } catch (e: any) {
        toast(e.message);
      }
    },
    view: async (title: string, loader: () => Promise<any>) => {
      try {
        const data = await loader();
        openModal({ kind: "viewer", title: `${title} · ${sid}`, body: JSON.stringify(data, null, 2) });
      } catch (e: any) {
        toast(e.message);
      }
    },
  };

  return (
    <>
      <div className="topbar">
        {isSub && (
          <a onClick={() => select(sid.slice(0, sid.lastIndexOf("-sub-")))}>← 父会话</a>
        )}
        <span className="sid">{sid}</span>
        <span className={"pill " + status.cls}>{status.text}</span>
        {isSub && <span className="readonly-tag">只读子会话</span>}
        <span className="spacer" />
        <label className="dim" style={{ fontSize: 12, display: "flex", alignItems: "center", gap: 4 }}>
          <input type="checkbox" checked={showSys} onChange={toggleSys} /> 系统事件
        </label>
        <div className="actions">
          <button className="sm" onClick={() => act.view("原始 journal", () => AR.rawEvents(sid))}>
            journal
          </button>
          <button className="sm" onClick={() => act.view("折叠状态", () => AR.state(sid))}>
            state
          </button>
          <button className="sm" onClick={() => act.view("inspect 树", () => AR.inspect(sid))}>
            inspect
          </button>
          {!isSub && (
            <>
              <button className="sm" onClick={() => openModal({ kind: "fork", sid })}>
                fork
              </button>
              <button className="sm" onClick={() => openModal({ kind: "agent", sid })}>
                换 agent
              </button>
              <button className="sm" onClick={act.resume}>
                resume
              </button>
              <button className="sm danger" onClick={act.interrupt}>
                interrupt
              </button>
              <button className="sm danger" onClick={act.close}>
                {closeArmed ? "确认关闭?" : "关闭"}
              </button>
            </>
          )}
        </div>
      </div>

      <TimelineView items={folded.items} pending={pending} typing={typing} showSys={showSys} />

      {openApprovals.length > 0 && (
        <div className="approvals">
          {openApprovals.map((a) => (
            <ApprovalCard
              key={a.id}
              approval={a}
              readonly={isSub}
              onDecide={decideApproval}
              onError={(m) => toast(m)}
            />
          ))}
        </div>
      )}

      {tasks.length > 0 && (
        <div className="workpanel">
          <h4>在飞后台任务 · {tasks.length}</h4>
          {tasks.map((t) => (
            <div className="task-row" key={t.handle}>
              <span className="grow">
                {t.handle} · {t.tool} · {t.detail}
              </span>
              {!isSub && (
                <button className="sm danger" onClick={() => act.kill(t.handle)}>
                  kill
                </button>
              )}
            </div>
          ))}
        </div>
      )}

      {!isSub && <Composer statusText={status.text} onSend={doSend} onError={(m) => toast(m)} />}
    </>
  );
}
