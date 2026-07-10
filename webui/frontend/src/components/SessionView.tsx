import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowClockwise, ArrowLeft, Check, DotsThree, Files, Folder, Stop, UsersThree } from "@phosphor-icons/react";
import { AR } from "../api";
import { useStore } from "../store";
import type { Envelope, Task } from "../types";
import { foldEvents, type ApprovalRef } from "../timeline";
import { TimelineView } from "./Timeline";
import { ApprovalCard } from "./ApprovalCard";
import { Composer } from "./Composer";
import { DiffView } from "./DiffView";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import type { InspectNode } from "./Subagents";
import { SupervisionPanel } from "./SupervisionPanel";
import { FindBar } from "./FindBar";
import { friendlyStatus } from "./pill";
import { displayTitle } from "../title";
import { dedupeInspectNodes } from "../viewModels";
import { ChangesOutcome } from "./ChangesOutcome";

interface SSEApproval {
  id: string;
  tool: string;
  args: any;
  agent?: string;
  session?: string;
}

// 1403 → "1.4k", 20 → "20" — a compact token count for the header badge.
function fmtTokens(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return (n / 1000).toFixed(n < 10_000 ? 1 : 0) + "k";
  return (n / 1_000_000).toFixed(1) + "M";
}

export function SessionView({ sid }: { sid: string }) {
  const { select, openModal, toast, showSys, toggleSys, sessions, sessionsReady, archived, toggleArchive, pinned, togglePin, renames } =
    useStore();
  const isSub = sid.includes("-sub-");
  const sessionMeta = sessions.find((s) => s.id === sid);
  const title = sessionsReady ? displayTitle(renames, sid, sessionMeta?.title) : "Loading task…";

  const [events, setEvents] = useState<Envelope[]>([]);
  const [pending, setPending] = useState<{ id: number; text: string; imgs: string[]; files: number }[]>([]);
  const [typing, setTyping] = useState<string>("");
  const [sseApprovals, setSseApprovals] = useState<Map<string, SSEApproval>>(new Map());
  const [resolvedLocal, setResolvedLocal] = useState<Set<string>>(new Set());
  const [tasks, setTasks] = useState<Task[]>([]);
  const [usage, setUsage] = useState<{ billed: number; steps: number } | null>(null);
  const [children, setChildren] = useState<InspectNode[]>([]);
  const [inspectReady, setInspectReady] = useState(false);
  const [goal, setGoal] = useState<{ goal: string; checks: number; max_checks?: number; paused?: boolean; verifiers?: number; claimed?: boolean } | null>(null);
  // The model-maintained checklist from inspect's progress projection (INC-37).
  const [progress, setProgress] = useState<import("./SupervisionPanel").ProgressItem[]>([]);
  // Non-null while the banner's goal text is being edited (INC-10): the value
  // is the draft; save issues a goal update (text only — verifier/budget keep).
  const [goalEdit, setGoalEdit] = useState<string | null>(null);
  const [view, setView] = useState<"chat" | "diff">("chat");
  const [findOpen, setFindOpen] = useState(false);
  const [wideViewport, setWideViewport] = useState(() => window.innerWidth > 900);
  // Supervision starts CLOSED and remembers the user's choice (W5): an empty
  // panel taking a third of the screen on every session was the single most
  // asked-about annoyance. A pending approval force-opens it (see below).
  const [supervisionOpen, setSupervisionOpen] = useState(() => window.innerWidth > 900 && localStorage.getItem("arwebui.supervision") === "1");
  const setSupervision = (open: boolean) => {
    setSupervisionOpen(open);
    try {
      localStorage.setItem("arwebui.supervision", open ? "1" : "0");
    } catch {
      /* ignore quota */
    }
  };

  const cursor = useRef(0);
  const pollBusy = useRef(false);
  const pendSeq = useRef(0);
  // journal seq → local upload paths, so a confirmed user bubble keeps its
  // image thumbnails (the journal itself only records a CAS ref).
  const sentImages = useRef(new Map<number, string[]>());
  const approvalAutoOpenedSupervision = useRef(false);

  useEffect(() => {
    const syncViewport = () => setWideViewport(window.innerWidth > 900);
    window.addEventListener("resize", syncViewport);
    return () => window.removeEventListener("resize", syncViewport);
  }, []);

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
              if (i >= 0) {
                // Hand the pending bubble's thumbnails over to the journal
                // bubble that replaces it (idempotent under re-runs).
                if (next[i].imgs.length && e.seq) sentImages.current.set(e.seq, next[i].imgs);
                next = next.filter((_, j) => j !== i);
              }
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
      setProgress(Array.isArray(ins?.progress) ? ins.progress : []);
      setInspectReady(true);
    } catch {
      /* ignore — usage badge / subagents are best-effort */
    }
  }, [sid]);

  const saveGoalEdit = () => {
    const g = (goalEdit || "").trim();
    if (!g) return;
    AR.goal(sid, { action: "update", goal: g })
      .then(() => {
        setGoalEdit(null);
        return pollTasks();
      })
      .catch((e) => toast(e.message));
  };

  useEffect(() => {
    cursor.current = 0;
    sentImages.current = new Map();
    setEvents([]);
    setPending([]);
    setTyping("");
    setSseApprovals(new Map());
    setResolvedLocal(new Set());
    setUsage(null);
    setChildren([]);
    setGoal(null);
    setInspectReady(false);
    poll();
    const e = setInterval(poll, 1000);
    const t = setInterval(pollTasks, 2500);
    pollTasks();
    let es: EventSource | null = null;
    {
      // Child sessions stream too (INC-12.6): the daemon routes a -sub- id
      // through the tree root's hub filtered to the member.
      es = new EventSource(`/api/sessions/${sid}/stream`);
      es.onmessage = (m) => {
        let ev: any;
        try {
          ev = JSON.parse(m.data);
        } catch {
          return;
        }
        // Tree members tag their own events; keep THIS view's typing bubble
        // to its own stream (approvals below still bubble tree-wide).
        const foreign = ev.session && ev.session !== sid;
        if (!foreign && ev.kind === "text_delta" && ev.text) setTyping((prev) => prev + ev.text);
        if (!foreign && ev.kind === "discard") setTyping("");
        // Child asks exist ONLY on this stream (they never touch the parent
        // journal). e.text carries the requesting agent's name.
        if (ev.kind === "approval_request" && ev.approval_id) {
          setSseApprovals((prev) => {
            const next = new Map(prev);
            next.set(ev.approval_id, {
              id: ev.approval_id,
              tool: ev.tool,
              args: ev.args,
              agent: ev.text || (foreign ? ev.session : ""),
              session: ev.session || sid,
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
  const openApprovals: (ApprovalRef & { agent?: string; viaSSE?: boolean; session?: string })[] = [];
  for (const a of folded.approvals.values()) {
    if (!a.resolved && !resolvedLocal.has(a.id)) openApprovals.push(a);
  }
  for (const s of sseApprovals.values()) {
    if (folded.approvals.has(s.id) || resolvedLocal.has(s.id)) continue;
    openApprovals.push({ id: s.id, tool: s.tool, args: s.args, gates: [], agent: s.agent, viaSSE: true, session: s.session });
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
      ? { text: "Needs approval", cls: "appr" }
      : { text: "Working…", cls: "run" }
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
  const needsRecovery = !live && /strand|interrupt/i.test(listStatus || "");
  const running = status.cls === "run";
  const abnormalAgentCount = dedupeInspectNodes(children).filter((node) => {
    const childStatus = friendlyStatus(node.reason || node.report?.reason || node.report?.status || "");
    return childStatus.cls === "crash" || childStatus.cls === "stranded";
  }).length;
  const attentionCount = openApprovals.length + (needsRecovery ? 1 : 0) + abnormalAgentCount + (tasks.length > 0 && !running ? 1 : 0);

  const doSend = async (text: string, images: string[], files: string[] = []) => {
    const id = ++pendSeq.current;
    setPending((p) => [...p, { id, text, imgs: images, files: files.length }]);
    try {
      await AR.send(sid, text, images, files);
    } catch (e: any) {
      toast(e.message);
      setPending((p) => p.filter((x) => x.id !== id));
    }
  };

  const decideApproval = async (id: string, decision: "approve" | "deny", reason: string, target = sid, always = false) => {
    await AR.approve(target, id, decision, reason, always);
    setResolvedLocal((s) => new Set(s).add(id));
    if (always) toast("approved — an exact allow rule was saved, this call won't ask again", "info");
  };

  // ⌘↵ approves the top pending request, ⌘⌫ denies it (Codex's Approve request).
  // A ref keeps the latest first-id / handler without rebinding each render.
  const apprKb = useRef<{ first: { id: string; session?: string } | null; decide: typeof decideApproval }>({
    first: null,
    decide: decideApproval,
  });
  apprKb.current = { first: !isSub && openApprovals[0] ? openApprovals[0] : null, decide: decideApproval };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      const { first, decide } = apprKb.current;
      if (!first) return;
      if (e.key === "Enter") {
        e.preventDefault();
        decide(first.id, "approve", "", first.session || sid).catch((err) => toast(err.message));
      } else if (e.key === "Backspace" || e.key === "Delete") {
        e.preventDefault();
        decide(first.id, "deny", "", first.session || sid).catch((err) => toast(err.message));
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
    stop: async () => {
      try {
        await AR.stopSession(sid);
        toast("session stopped — send a message to revive it", "info");
      } catch (e: any) {
        toast(e.message);
      }
    },
    close: async () => {
      openModal({
        kind: "confirm",
        title: "Close task?",
        body: "This ends the current conversation and marks it closed. Sending a new message later will reopen it.",
        confirmLabel: "Close task",
        danger: true,
        onConfirm: async () => {
          await AR.closeSession(sid);
          toast("task closed", "info");
        },
      });
    },
    kill: async (handle: string) => {
      try {
        await AR.kill(sid, handle);
      } catch (e: any) {
        toast(e.message);
      }
    },
    barrier: async () => {
      try {
        const r = await AR.barrier(sid);
        toast(`checkpoint ${r.barrier || "created"} — fork from it anytime`, "info");
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

  // The inline approval card is the primary action. On roomy desktop layouts
  // Supervision may reinforce it, but narrow screens must not be covered by an
  // auto-open overlay. If we opened the panel, close it when attention clears.
  const hasApprovals = openApprovals.length > 0;
  useEffect(() => {
    if (hasApprovals && view === "chat" && wideViewport) {
      if (!supervisionOpen) {
        approvalAutoOpenedSupervision.current = true;
        setSupervisionOpen(true);
      }
      return;
    }
    if (approvalAutoOpenedSupervision.current) {
      approvalAutoOpenedSupervision.current = false;
      setSupervisionOpen(false);
    }
  }, [hasApprovals, view, wideViewport]);

  const showSupervision = supervisionOpen && view === "chat";

  return (
    <div className="session-view">
      <header className="task-topbar">
        {isSub && (
          <button className="topbar-icon" onClick={() => select(sid.slice(0, sid.lastIndexOf("-sub-")))} title="Back to parent task">
            <ArrowLeft size={16} />
          </button>
        )}
        <div className="tt-left">
          <Folder size={17} />
          <div className="tt-title" title={`${sessions.find((s) => s.id === sid)?.title || title}\n${sid}`}>{title}</div>
          {isSub && <span className="readonly-tag">Read-only subtask</span>}
        </div>
        <span className="spacer" />
        {!isSub && running && (
          <button className="topbar-tool stop" onClick={act.interrupt} title="Stop the active turn">
            <Stop size={14} weight="fill" /> Stop
          </button>
        )}
        {!isSub && needsRecovery && (
          <button className="topbar-tool recovery" onClick={act.resume} title="Resume this task from its last durable checkpoint">
            <ArrowClockwise size={15} /> Resume
          </button>
        )}
        <button className={`topbar-tool${view === "diff" ? " active" : ""}`} onClick={() => setView(view === "diff" ? "chat" : "diff")} title="Review workspace changes">
          <Files size={16} /> Changes
        </button>
        <button className={`topbar-tool${showSupervision ? " active" : ""}`} onClick={() => {
          if (view === "diff") setView("chat");
          setSupervision(!showSupervision);
        }} title="Show supervision">
          <UsersThree size={16} /> Supervision
          {attentionCount > 0 && <span className="topbar-attention">{attentionCount}</span>}
        </button>
        <Menu label={<DotsThree size={18} weight="bold" />} ariaLabel="More task actions">
          <MenuLabel>View</MenuLabel>
          <MenuItem onClick={() => setView("chat")}>Conversation</MenuItem>
          <MenuItem onClick={() => setView("diff")}>Changes</MenuItem>
          <MenuItem onClick={() => setSupervision(!supervisionOpen)}>{supervisionOpen ? "Hide" : "Show"} supervision</MenuItem>
          <MenuItem
            title="also show low-level system events (mode changes, effects, barriers…) inline in the timeline"
            onClick={toggleSys}
          >
            {showSys && <Check size={14} />}Show system events
          </MenuItem>
          {!isSub && (
            <>
              <MenuLabel>Advanced</MenuLabel>
              <MenuItem
                title="checkpoint the session right now (ar barrier) so you can fork from this exact point later"
                onClick={act.barrier}
              >
                Create checkpoint
              </MenuItem>
              <MenuItem
                title="continue from a checkpoint in a new task and worktree; this task is untouched"
                onClick={() => openModal({ kind: "fork", sid })}
              >
                Continue in new task…
              </MenuItem>
              <MenuItem
                title="swap this session's agent spec — context carries over; takes effect on your next message (spec_changed)"
                onClick={() => openModal({ kind: "agent", sid })}
              >
                Switch agent…
              </MenuItem>
              <MenuLabel>Lifecycle</MenuLabel>
              {needsRecovery && <MenuItem onClick={act.resume}>Resume task</MenuItem>}
              {running && <MenuItem onClick={act.stop}>Stop active run</MenuItem>}
              <MenuItem
                danger
                title="gracefully end the conversation and mark it closed (ar close); a later send reopens it"
                onClick={act.close}
              >
                Close session…
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
      </header>

      {view === "chat" && findOpen && (
        <FindBar scope={() => document.querySelector<HTMLElement>(".timeline")} onClose={() => setFindOpen(false)} />
      )}
      <div className={`session-layout${showSupervision ? "" : " single"}`}>
        <main className="session-primary">
          {view === "diff" ? (
            <DiffView sid={sid} />
          ) : (
            <>
              {showSys && (
                <div className="system-events-notice">
                  System events are visible
                  <button onClick={toggleSys}>Hide</button>
                </div>
              )}
              <TimelineView
                items={folded.items}
                pending={pending}
                typing={typing}
                showSys={showSys}
                sentImages={sentImages.current}
                statusLine={running || hasApprovals || needsRecovery ? (
                  <div className={`run-status-line ${status.cls}`}>
                    {running && <span className="spin" />}
                    <span>{running ? "Working" : status.text}</span>
                    {usage && usage.billed > 0 && (
                      <span title="billed tokens · model generation steps this session">
                        {fmtTokens(usage.billed)} tokens{usage.steps ? ` · ${usage.steps} steps` : ""}
                      </span>
                    )}
                  </div>
                ) : undefined}
                approvalSlot={openApprovals.length > 0 ? (
                  <div className="approval-stack">
                    {openApprovals.map((approval) => (
                      <ApprovalCard
                        key={approval.id}
                        approval={approval}
                        readonly={isSub}
                        workspace={sessions.find((s) => s.id === sid)?.workspace}
                        onDecide={(id, decision, reason, always) => decideApproval(id, decision, reason, approval.session || sid, always)}
                        onError={(message) => toast(message)}
                      />
                    ))}
                  </div>
                ) : undefined}
                active={running}
                onContinue={() => openModal({ kind: "fork", sid })}
                outcomeSlot={folded.items.some((item) => item.kind === "assistant") ? (
                  <ChangesOutcome sid={sid} refreshKey={events.length} onReview={() => setView("diff")} />
                ) : undefined}
              />
              {isDriver && <div className="driver-note">This scheduled run manages its own iterations and does not accept follow-up messages.</div>}
              {!isSub && !isDriver && (
                <Composer
                  variant="session"
                  sid={sid}
                  workspace={sessions.find((session) => session.id === sid)?.workspace}
                  running={running}
                  onSend={doSend}
                  onError={(message) => toast(message)}
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
          )}
        </main>
        {showSupervision && (
          <SupervisionPanel
            loading={!inspectReady}
            goal={goal}
            goalEdit={goalEdit}
            progress={progress}
            children={children}
            tasks={tasks}
            approvals={openApprovals.length}
            sessionIdle={!running}
            recovery={needsRecovery}
            onGoalEdit={setGoalEdit}
            onGoalSave={saveGoalEdit}
            onGoalDiscard={() => setGoalEdit(null)}
            onGoalAction={(action) => AR.goal(sid, { action }).then(() => pollTasks()).catch((error) => toast(error.message))}
            onOpenChild={(childSid) => select(childSid)}
            onKillTask={act.kill}
            onInspect={() => AR.inspect(sid).then((data) => openModal({
              kind: "inspect",
              data,
              status: sessions.find((session) => session.id === sid)?.status,
            })).catch((error) => toast(error.message))}
            onClose={() => setSupervision(false)}
          />
        )}
      </div>
    </div>
  );
}
