import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AR } from "../api";
import { useStore } from "../store";
import type { Envelope, Task } from "../types";
import { foldEvents, type ApprovalRef } from "../timeline";
import { TimelineView } from "./Timeline";
import { ApprovalCard } from "./ApprovalCard";
import { Composer } from "./Composer";
import { DiffView } from "./DiffView";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import { Subagents, type InspectNode } from "./Subagents";
import { FindBar } from "./FindBar";
import { friendlyStatus } from "./pill";
import { displayTitle } from "../title";

interface SSEApproval {
  id: string;
  tool: string;
  args: any;
  agent?: string;
}

// 1403 → "1.4k", 20 → "20" — a compact token count for the header badge.
function fmtTokens(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return (n / 1000).toFixed(n < 10_000 ? 1 : 0) + "k";
  return (n / 1_000_000).toFixed(1) + "M";
}

export function SessionView({ sid }: { sid: string }) {
  const { select, openModal, toast, showSys, toggleSys, sessions, archived, toggleArchive, pinned, togglePin, renames } =
    useStore();
  const isSub = sid.includes("-sub-");
  const title = displayTitle(renames, sid, sessions.find((s) => s.id === sid)?.title);

  const [events, setEvents] = useState<Envelope[]>([]);
  const [pending, setPending] = useState<{ id: number; text: string; images: number }[]>([]);
  const [typing, setTyping] = useState<string>("");
  const [sseApprovals, setSseApprovals] = useState<Map<string, SSEApproval>>(new Map());
  const [resolvedLocal, setResolvedLocal] = useState<Set<string>>(new Set());
  const [tasks, setTasks] = useState<Task[]>([]);
  const [usage, setUsage] = useState<{ billed: number; steps: number } | null>(null);
  const [children, setChildren] = useState<InspectNode[]>([]);
  const [goal, setGoal] = useState<{ goal: string; checks: number; max_checks?: number; paused?: boolean } | null>(null);
  const [view, setView] = useState<"chat" | "diff">("chat");
  const [findOpen, setFindOpen] = useState(false);

  const cursor = useRef(0);
  const pollBusy = useRef(false);
  const pendSeq = useRef(0);

  // ⌘F / Ctrl-F opens the in-chat Find bar (Codex's Search chat). We take over
  // the browser's native find since Find operates on the rendered timeline.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f") {
        e.preventDefault();
        setFindOpen(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Leaving chat view (e.g. to the diff) closes Find — it only searches the timeline.
  useEffect(() => {
    if (view !== "chat") setFindOpen(false);
  }, [view]);

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
    try {
      const ins = await AR.inspect(sid);
      const u = ins?.usage;
      if (u) setUsage({ billed: u.billed ?? (u.input_tokens || 0) + (u.output_tokens || 0), steps: ins.gen_steps || 0 });
      setChildren(Array.isArray(ins?.children) ? ins.children : []);
      setGoal(ins?.goal || null);
    } catch {
      /* ignore — usage badge / subagents are best-effort */
    }
  }, [sid]);

  useEffect(() => {
    cursor.current = 0;
    setEvents([]);
    setPending([]);
    setTyping("");
    setSseApprovals(new Map());
    setResolvedLocal(new Set());
    setUsage(null);
    setChildren([]);
    setGoal(null);
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

  // Status precedence: a live turn (tool running / step in flight / a pending
  // approval) wins — it's the most current. Otherwise the daemon's session
  // status (from `ar sessions list`) is authoritative and keeps the header in
  // sync with the sidebar (QA #8). A child session has no list entry, so a
  // dangling "running…" with nothing active means it finished (QA #6).
  const listStatus = sessions.find((s) => s.id === sid)?.status;
  const live = folded.active || openApprovals.length > 0;
  const status = live
    ? openApprovals.length > 0
      ? { text: "needs approval", cls: "appr" }
      : { text: "running…", cls: "run" }
    : folded.isDriver
      ? // a driver session's `sessions list` status is "unreadable"; its own
        // journal (driver_completed) is the authoritative status.
        folded.status
      : listStatus
        ? friendlyStatus(listStatus)
        : folded.status.cls === "run"
          ? { text: "completed", cls: "closed" }
          : folded.status;
  const isDriver = folded.isDriver;

  const doSend = async (text: string, images: string[], files: string[] = []) => {
    const id = ++pendSeq.current;
    setPending((p) => [...p, { id, text, images: images.length + files.length }]);
    try {
      await AR.send(sid, text, images, files);
    } catch (e: any) {
      toast(e.message);
      setPending((p) => p.filter((x) => x.id !== id));
    }
  };

  const decideApproval = async (id: string, decision: "approve" | "deny", reason: string) => {
    await AR.approve(sid, id, decision, reason);
    setResolvedLocal((s) => new Set(s).add(id));
  };

  // Approve/deny every pending request at once — drivers and parallel tool
  // calls can queue several, and clicking each is slow.
  const decideAll = async (decision: "approve" | "deny") => {
    for (const a of openApprovals) {
      try {
        await decideApproval(a.id, decision, "");
      } catch (e: any) {
        toast(e.message);
      }
    }
  };

  // ⌘↵ approves the top pending request, ⌘⌫ denies it (Codex's Approve request).
  // A ref keeps the latest first-id / handler without rebinding each render.
  const apprKb = useRef<{ firstId: string | null; decide: typeof decideApproval }>({
    firstId: null,
    decide: decideApproval,
  });
  apprKb.current = { firstId: !isSub && openApprovals[0] ? openApprovals[0].id : null, decide: decideApproval };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      const { firstId, decide } = apprKb.current;
      if (!firstId) return;
      if (e.key === "Enter") {
        e.preventDefault();
        decide(firstId, "approve", "").catch((err) => toast(err.message));
      } else if (e.key === "Backspace" || e.key === "Delete") {
        e.preventDefault();
        decide(firstId, "deny", "").catch((err) => toast(err.message));
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const act = {
    interrupt: async () => {
      try {
        await AR.interrupt(sid);
        toast("interrupt sent", "info");
      } catch (e: any) {
        toast(e.message);
      }
    },
    resume: async () => {
      try {
        await AR.resume(sid);
        toast("resume sent", "info");
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

  const running = status.cls === "run";
  const showSpin = status.cls === "run";

  return (
    <>
      <div className="task-topbar">
        <div className="tt-left">
          {isSub && (
            <a className="tt-back" onClick={() => select(sid.slice(0, sid.lastIndexOf("-sub-")))}>
              ←
            </a>
          )}
          <div className="tt-title" title={sid}>
            {title}
          </div>
          <span className={"status-chip " + status.cls}>
            {showSpin && <span className="spin" />}
            {status.text}
          </span>
          {usage && usage.billed > 0 && (
            <button
              className="usage-badge"
              title="context usage — billed tokens; click for the inspect tree (sub-agents, per-node usage)"
              onClick={() => act.view("inspect tree", () => AR.inspect(sid))}
            >
              ⛁ {fmtTokens(usage.billed)}
              {usage.steps ? ` · ${usage.steps} steps` : ""}
            </button>
          )}
          {isSub && <span className="readonly-tag" title="a sub-agent session — watch it here; only its parent can drive it">read-only sub-task</span>}
        </div>

        <div className="seg tabs">
          <button className={view === "chat" ? "on" : ""} onClick={() => setView("chat")} title="the conversation timeline, rendered from the journal">
            Activity
          </button>
          <button className={view === "diff" ? "on" : ""} onClick={() => setView("diff")} title="git diff of the session's workspace (changed + untracked files)">
            Diff
          </button>
        </div>

        <span className="spacer" />

        {!isSub && running && (
          <button className="stop-btn" onClick={act.interrupt} title="interrupt: cancel the in-flight turn so you can redirect the agent">
            ■ Stop
          </button>
        )}

        <Menu label="⋯">
          <MenuLabel>View</MenuLabel>
          <MenuItem
            title="the append-only event log (ar events --json) — the source of truth this timeline is rendered from"
            onClick={() => act.view("raw journal", () => AR.rawEvents(sid))}
          >
            Raw journal
          </MenuItem>
          <MenuItem
            title="the current session state folded from the journal (ar events --state): status, spec, usage"
            onClick={() => act.view("folded state", () => AR.state(sid))}
          >
            Folded state
          </MenuItem>
          <MenuItem
            title="the session tree (ar inspect): sub-agents, status and token usage"
            onClick={() => act.view("inspect tree", () => AR.inspect(sid))}
          >
            Inspect tree
          </MenuItem>
          <MenuItem
            title="also show low-level system events (mode changes, effects, barriers…) inline in the timeline"
            onClick={toggleSys}
          >
            {showSys ? "✓ " : ""}Show system events
          </MenuItem>
          {!isSub && (
            <>
              <MenuLabel>Advanced</MenuLabel>
              <MenuItem
                title="branch a new independent session into its own git worktree from a checkpoint; this session is untouched"
                onClick={() => openModal({ kind: "fork", sid })}
              >
                Fork into new worktree…
              </MenuItem>
              <MenuItem
                title="swap this session's agent spec — context carries over; takes effect on your next message (spec_changed)"
                onClick={() => openModal({ kind: "agent", sid })}
              >
                Switch agent…
              </MenuItem>
              <MenuItem
                title="recover a crashed or interrupted session (ar resume) so it can keep going"
                onClick={act.resume}
              >
                Resume (recover after crash/interrupt)
              </MenuItem>
            </>
          )}
          <MenuLabel>Organize</MenuLabel>
          <MenuItem
            title="keep this task in a Pinned section at the top of the sidebar"
            onClick={() => {
              togglePin(sid);
              toast(pinned.includes(sid) ? "unpinned" : "pinned", "info");
            }}
          >
            {pinned.includes(sid) ? "Unpin task" : "Pin task"}
          </MenuItem>
          <MenuItem
            title="give this task a custom name in the sidebar (stored in your browser)"
            onClick={() => openModal({ kind: "rename", sid })}
          >
            Rename task…
          </MenuItem>
          <MenuItem
            title="hide this task from the sidebar list (it stays on disk; toggle 'Show archived' to see it again)"
            onClick={() => {
              toggleArchive(sid);
              toast(archived.includes(sid) ? "unarchived" : "archived", "info");
            }}
          >
            {archived.includes(sid) ? "Unarchive task" : "Archive task"}
          </MenuItem>
        </Menu>
      </div>

      {view === "chat" && findOpen && (
        <FindBar scope={() => document.querySelector<HTMLElement>(".timeline")} onClose={() => setFindOpen(false)} />
      )}

      {view === "diff" ? (
        <DiffView sid={sid} />
      ) : (
        <TimelineView items={folded.items} pending={pending} typing={typing} showSys={showSys} />
      )}

      {view === "chat" && openApprovals.length > 0 && (
        <div className="approvals">
          <div className="approvals-title">
            <span>The agent needs your approval</span>
            {!isSub && (
              <span className="ap-bulk">
                {openApprovals.length > 1 && (
                  <>
                    <button className="primary sm" onClick={() => decideAll("approve")}>
                      Approve all {openApprovals.length}
                    </button>
                    <button className="danger sm" onClick={() => decideAll("deny")}>
                      Deny all
                    </button>
                  </>
                )}
                <span className="ap-hint mono">⌘↵ approve · ⌘⌫ deny</span>
              </span>
            )}
          </div>
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

      {view === "chat" && children.length > 0 && (
        <div className="workpanel subagents-panel">
          <Subagents nodes={children} onOpen={(s) => select(s)} />
        </div>
      )}

      {view === "chat" && tasks.length > 0 && (
        <div className="workpanel">
          <h4>In-flight background work · {tasks.length}</h4>
          {tasks.map((t) => (
            <div className="task-row" key={t.handle}>
              <span className="grow">
                {t.tool} · {t.detail || t.handle}
              </span>
              {!isSub && (
                <button className="sm danger" onClick={() => act.kill(t.handle)} title="cancel this background handle (ar kill) — the session itself keeps running">
                  kill
                </button>
              )}
            </div>
          ))}
        </div>
      )}

      {view === "chat" && isDriver && (
        <div className="composer">
          <div className="composer-inner dim" style={{ fontSize: 12, padding: "4px 2px" }}>
            This is an iteration driver (drive) — it runs its own loop and does not accept messages.
          </div>
        </div>
      )}

      {view === "chat" && !isSub && goal && (
        <div className={"goal-banner" + (goal.paused ? " paused" : "")}>
          <span className="gb-ico">🎯</span>
          <span className="gb-text">
            <b>Goal</b> · {goal.goal}
          </span>
          <span className="gb-meta">
            {goal.checks}
            {goal.max_checks ? `/${goal.max_checks}` : ""} checks{goal.paused ? " · paused" : ""}
          </span>
          <span className="spacer" />
          {goal.paused ? (
            <button className="sm" onClick={() => AR.goal(sid, { action: "resume" }).then(() => pollTasks()).catch((e) => toast(e.message))}>
              resume
            </button>
          ) : (
            <button className="sm" onClick={() => AR.goal(sid, { action: "pause" }).then(() => pollTasks()).catch((e) => toast(e.message))}>
              pause
            </button>
          )}
          <button className="sm danger" onClick={() => AR.goal(sid, { action: "cancel" }).then(() => pollTasks()).catch((e) => toast(e.message))}>
            cancel
          </button>
        </div>
      )}

      {view === "chat" && !isSub && !isDriver && (
        <Composer
          variant="session"
          sid={sid}
          workspace={sessions.find((s) => s.id === sid)?.workspace}
          running={running}
          onSend={doSend}
          onError={(m) => toast(m)}
          actions={{
            interrupt: act.interrupt,
            showDiff: () => setView("diff"),
            fork: () => openModal({ kind: "fork", sid }),
            switchAgentAdvanced: () => openModal({ kind: "agent", sid }),
            resume: act.resume,
          }}
        />
      )}
    </>
  );
}
