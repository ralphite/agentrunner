import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Archive, ArrowClockwise, ArrowLeft, ChatCircle, CheckCircle, ClockCountdown, Code, Crosshair, DotsThree, Files, Flag, GitFork, Pause, PencilSimple, Play, Prohibit, PushPin, Robot, SidebarSimple, SlidersHorizontal, Trash, WarningCircle, X, XCircle } from "@phosphor-icons/react";
import { AR, type ForkDraft } from "../api";
import { useStore } from "../store";
import type { BackgroundWork, Envelope } from "../types";
import { deriveGoalState, foldEvents, formatElapsed, isGoalTerminal, suppressEchoedChips, type ApprovalRef, type BubbleItem, type GoalDerived } from "../timeline";
import { TimelineView } from "./Timeline";
import { ApprovalCard } from "./ApprovalCard";
import { Composer } from "./Composer";
import { AskForm } from "./AskForm";
import { DiffView } from "./DiffView";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import type { InspectDelegation, InspectNode } from "./Subagents";
import { SupervisionPanel } from "./SupervisionPanel";
import { FindBar } from "./FindBar";
import { friendlyStatus, terminalNoticeFor } from "./pill";
import { displayTitle } from "../title";
import { dedupeInspectNodes } from "../viewModels";
import { ChangesOutcome } from "./ChangesOutcome";
import { DaemonAlert } from "./DaemonAlert";
import { SessionNotFound } from "./NotFound";
import { useBreakpoint } from "../hooks/useBreakpoint";

interface SSEApproval {
  id: string;
  tool: string;
  args: any;
  agent?: string;
  session?: string;
}

// INC-41 RT-7 · The server's own id grammar (webui/ar.go `idPattern` +
// `validID`): everything it splices into an `ar` argv position must match, and
// anything that doesn't is answered with a flat 400 "invalid session id" — the
// route never even reaches the daemon. Mirroring the grammar here lets a hand-
// mangled deep link (`#/s/hello world`, `#/s/bad!id`) short-circuit to the
// not-found state WITHOUT firing a single request: no 1s poll loop, no
// EventSource reconnect storm, no composer over a session that cannot exist.
const SESSION_ID_RE = /^[A-Za-z0-9._#-]+$/;
export function isValidSessionId(sid: string): boolean {
  return !!sid && sid.length <= 200 && SESSION_ID_RE.test(sid);
}

// Persist the idempotency key across a response-loss reload. Keeping the key
// after success is harmless and guarantees that a later retry of the same row
// resolves the already-published child instead of creating a sibling.
function continuationRequestID(sid: string, itemID: string): string {
  const key = `arwebui.continue.${sid}.${itemID}`;
  try {
    const prior = sessionStorage.getItem(key);
    if (prior) return prior;
    const id = `continue_${crypto.randomUUID().replaceAll("-", "_")}`;
    sessionStorage.setItem(key, id);
    return id;
  } catch {
    return `continue_${crypto.randomUUID().replaceAll("-", "_")}`;
  }
}

// INC-41 L2/L5/RT-7 · "this id doesn't exist" vs "the fetch happened to fail".
// The backend answers an unknown session id with a real 404 + code=
// session_not_found (arFail owns the single string match against the CLI's
// verdict), and a syntactically impossible one with 400 "invalid session id"
// (api.go `sid()`/`badRequest`) — both are permanent verdicts about the ID, so
// both end the poll. The 400 arm is deliberately narrow: it demands the
// server's exact "invalid session id" wording, so an ordinary transient 400
// from some other endpoint (bad body, bad scope) still counts as transient and
// keeps polling. Everything else (daemon restarting, network blip, timeout,
// 502) stays transient too: we never accuse a real session of not existing. The
// stderr match survives only as a fallback for a stale webui binary that still
// 502s. Duck-typed on purpose — an ApiError from a mocked/older api module
// still classifies.
export function isSessionNotFound(err: unknown): boolean {
  const e = err as { status?: unknown; code?: unknown; message?: unknown } | null | undefined;
  if (e && (e.status === 404 || e.code === "session_not_found")) return true;
  const msg = err instanceof Error ? err.message : typeof err === "string" ? err : "";
  if (e && e.status === 400 && /invalid session id/i.test(msg)) return true;
  return /no session matches/i.test(msg);
}

// 1403 → "1.4k", 20 → "20" — a compact token count for the header badge.
function fmtTokens(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return (n / 1000).toFixed(n < 10_000 ? 1 : 0) + "k";
  return (n / 1_000_000).toFixed(1) + "M";
}

export function SessionView({ sid, mobileNavigationOpen = false }: { sid: string; mobileNavigationOpen?: boolean }) {
  const { select, openModal, toast, showSys, toggleSys, sessions, archived, toggleArchive, pinned, togglePin, renames } =
    useStore();
  // A real sub-agent session id is `<parent>-sub-call_<callId>-<suffix>` — the
  // `-sub-call_` marker is what the daemon appends. Plain `-sub-` also matches
  // top-level sessions whose TITLE slug happens to contain "sub" (e.g.
  // "…-worker-sub-agent-4886"), which wrongly flagged them read-only, showed a
  // dead "Back to parent" link, and hid the composer (R4-1).
  const subMarker = "-sub-call_";
  const isSub = sid.includes(subMarker);
  const sessionMeta = sessions.find((s) => s.id === sid);
  // A deep link can hydrate its journal well before the paged session list.
  // Use the existing durable-id fallback immediately, then replace it with the
  // journal title as soon as that metadata page arrives.
  const title = displayTitle(renames, sid, sessionMeta?.title);

  const [events, setEvents] = useState<Envelope[]>([]);
  const continueRequests = useRef<Map<string, string>>(new Map());
  const [pending, setPending] = useState<{ id: number; text: string; imgs: string[]; files: number; delivery?: "steer" | "queue" }[]>([]);
  const [typing, setTyping] = useState<string>("");
  const [sseApprovals, setSseApprovals] = useState<Map<string, SSEApproval>>(new Map());
  const [resolvedLocal, setResolvedLocal] = useState<Set<string>>(new Set());
  const [backgroundWork, setBackgroundWork] = useState<BackgroundWork[]>([]);
  const [usage, setUsage] = useState<{ billed: number; steps: number } | null>(null);
  const [children, setChildren] = useState<InspectNode[]>([]);
  const [delegations, setDelegations] = useState<InspectDelegation[]>([]);
  // Child journals deliberately inherit the parent's durable title. Inspect
  // carries the actual agent spec, which is the label a person needs when
  // moving between three otherwise indistinguishable worker sessions.
  const [subAgentName, setSubAgentName] = useState<string | undefined>();
  const [inspectReady, setInspectReady] = useState(false);
  // The first events fetch for this sid hasn't returned yet (INC-41 L1) — the
  // timeline is UNKNOWN, not empty. Flips on the first settled poll (success or
  // failure), never back: a later poll failing must not re-skeleton a thread
  // that's already on screen.
  const [eventsReady, setEventsReady] = useState(false);
  // The daemon says this session id doesn't exist (INC-41 L2).
  const [notFound, setNotFound] = useState(false);
  // Mirrors `notFound` for the pollers: they are closures on intervals, and a
  // dead id must stop spawning `ar` subprocesses every second.
  const gone = useRef(false);
  // The session's LIVE permission mode from inspect's fold (INC-42, G29):
  // /mode switches it mid-session, so the composer pill must not freeze on
  // the launch-time value.
  const [liveMode, setLiveMode] = useState<string | undefined>(undefined);
  const [goal, setGoal] = useState<{ goal: string; checks: number; max_checks?: number; paused?: boolean; verifiers?: number; claimed?: boolean } | null>(null);
  // The model-maintained checklist from inspect's progress projection (INC-37).
  const [progress, setProgress] = useState<import("./SupervisionPanel").ProgressItem[]>([]);
  // Published artifacts from inspect (INC-40): stream/version rows.
  const [artifacts, setArtifacts] = useState<{ stream: string; version: number }[]>([]);
  // A structured ask_user park's questions (INC-47.2): non-empty while the
  // session waits on a questions[] ask, so a form card renders in place.
  const [askQuestions, setAskQuestions] = useState<import("./AskForm").AskQuestion[]>([]);
  // Queued (not-yet-consumed) messages (INC-47.2): each withdrawable.
  const [queued, setQueued] = useState<{ command_id: string; text: string; revoked: boolean }[]>([]);
  // Non-null while the banner's goal text is being edited (INC-10): the value
  // is the draft; save issues a goal update (text only — verifier/budget keep).
  const [goalEdit, setGoalEdit] = useState<string | null>(null);
  // Which control initiated the goal edit (FB-2): the banner and the
  // Supervision panel share goalEdit, so a single ✎ click used to render TWO
  // editors with focus landing on the one you didn't click. The editor now
  // renders only at its source; the other side stays read-only.
  const [goalEditSrc, setGoalEditSrc] = useState<"banner" | "panel">("banner");
  const [goalPendingUpdate, setGoalPendingUpdate] = useState<string | null>(null);
  const [view, setView] = useState<"chat" | "diff">("chat");
  // One-shot scope hint for the diff panel: set only by the changes card (whose
  // title names a scope), cleared by every entry that makes no scope claim, so
  // the panel always opens on the scope its entry point advertised.
  const [diffScopeHint, setDiffScopeHint] = useState<"working-tree" | "last-turn" | null>(null);
  // INC-98.4c · Changes is a temporary inspection surface. Keep the element
  // that opened it so its only close control can return keyboard users to
  // their exact place; a menuitem disappears with its popover, so that path
  // falls back to the stable topbar trigger.
  const diffOpenerRef = useRef<HTMLElement | null>(null);
  // QA-0719 · UI-side workspace mutations (Undo/commit/push) emit no session
  // event, so event-driven git surfaces went stale; the epoch rides along in
  // their refreshKey.
  const workspaceEpoch = useStore((s) => s.workspaceEpoch);
  const openDiff = (hint: "working-tree" | "last-turn" | null = null) => {
    const active = document.activeElement;
    diffOpenerRef.current = active instanceof HTMLElement && active !== document.body ? active : null;
    setDiffScopeHint(hint);
    setView("diff");
  };
  const closeDiff = () => {
    setView("chat");
    requestAnimationFrame(() => {
      const opener = diffOpenerRef.current;
      const fallback = document.querySelector<HTMLButtonElement>('button[aria-label="More session actions"]');
      (opener?.isConnected ? opener : fallback)?.focus();
      diffOpenerRef.current = null;
    });
  };
  // RT-5 · The failure banner's "technical details" fold is closed by default:
  // the raw provider string is available, never in your face.
  const [failureRawOpen, setFailureRawOpen] = useState(false);
  const [failureRetrying, setFailureRetrying] = useState(false);
  const [findOpen, setFindOpen] = useState(false);
  const bp = useBreakpoint();
  // Supervision starts CLOSED and remembers the user's choice (W5): an empty
  // panel taking a third of the screen on every session was the single most
  // asked-about annoyance. A pending approval force-opens it (see below).
  // Codex shows the right context panel by default on a wide screen (R1-3);
  // open it unless the user has explicitly closed it before ("0"). Narrow
  // screens stay collapsed so the conversation isn't squeezed.
  const [supervisionOpen, setSupervisionOpen] = useState(() => (bp.desktop || bp.wide) && localStorage.getItem("arwebui.supervision") !== "0");
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
  const approvalAdjustedSupervision = useRef<"opened" | "closed" | null>(null);

  // Mobile navigation and Environment are both full-height overlays. Opening
  // the navigation drawer must make it the sole active layer, otherwise its
  // scrim traps a second drawer (and a second close button) underneath it.
  useEffect(() => {
    if (mobileNavigationOpen && (bp.compact || bp.tablet)) setSupervisionOpen(false);
  }, [mobileNavigationOpen, bp.compact, bp.tablet]);

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
    if (pollBusy.current || gone.current) return;
    pollBusy.current = true;
    try {
      const evs = await AR.events(sid, cursor.current);
      if (evs.length) {
        setPending((prev) => {
          let next = prev;
          for (const e of evs) {
            if (e.type === "input_received" ||
                (e.type === "ask_resolved" && e.payload?.resolution === "answered")) {
              // A normal follow-up is journaled as InputReceived. While the
              // session is parked on ask_user, the compatibility composer is
              // instead consumed directly as AskResolved{answer}; no duplicate
              // user message is written. Reconcile both durable receipts or
              // the optimistic answer survives as a false `queued…` bubble
              // until reload even though the agent already continued.
              const t = e.type === "ask_resolved" ? e.payload?.answer : e.payload?.text;
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
    } catch (e) {
      /* daemon down / transient: health dot tells the story */
      if (isSessionNotFound(e)) {
        gone.current = true;
        setNotFound(true);
      }
    } finally {
      pollBusy.current = false;
      setEventsReady(true);
    }
  }, [sid]);

  const pollInspect = useCallback(async () => {
    if (gone.current) return;
    try {
      setBackgroundWork(await AR.ps(sid));
    } catch {
      /* ignore */
    }
    try {
      const ins = await AR.inspect(sid);
      const u = ins?.usage;
      if (u) setUsage({ billed: u.billed ?? (u.input_tokens || 0) + (u.output_tokens || 0), steps: ins.gen_steps || 0 });
      setChildren(Array.isArray(ins?.children) ? ins.children : []);
      setDelegations(Array.isArray(ins?.delegations) ? ins.delegations : []);
      if (isSub && typeof ins?.spec === "string" && ins.spec.trim()) setSubAgentName(ins.spec.trim());
      setGoal(ins?.goal || null);
      setProgress(Array.isArray(ins?.progress) ? ins.progress : []);
      if (typeof ins?.mode === "string" && ins.mode) setLiveMode(ins.mode);
      {
        // Latest version per stream — inspect lists every published version.
        const latest = new Map<string, number>();
        for (const a of Array.isArray(ins?.artifacts) ? ins.artifacts : []) {
          if (a?.stream && (latest.get(a.stream) || 0) < (a.version || 0)) latest.set(a.stream, a.version);
        }
        setArtifacts([...latest.entries()].map(([stream, version]) => ({ stream, version })).sort((x, y) => x.stream.localeCompare(y.stream)));
      }
      // A structured ask park surfaces its questions (INC-47.2); a plain
      // idle or single-question ask leaves the form empty (the composer
      // answers those).
      const wq = ins?.waiting?.kind === "input" ? ins?.waiting?.ask_questions : undefined;
      setAskQuestions(Array.isArray(wq) ? wq : []);
    } catch (e) {
      /* ignore — usage badge / subagents are best-effort */
      if (isSessionNotFound(e)) {
        gone.current = true;
        setNotFound(true);
      }
    } finally {
      // INC-41 L2 · inspect has ANSWERED (even with an error) — Supervision's
      // three "Checking…" spinners must settle. Leaving this inside the success
      // path made a failing inspect spin forever.
      setInspectReady(true);
    }
    if (gone.current) return;
    // Queued messages (INC-47.2): withdrawable until consumed. Best-effort.
    try {
      const q = await AR.queue(sid);
      setQueued(Array.isArray(q) ? q : []);
    } catch {
      setQueued([]);
    }
  }, [sid, isSub]);

  const answerAsk = async (specs: string[]) => {
    try {
      await AR.answer(sid, specs);
      setAskQuestions([]);
      poll();
    } catch (e: any) {
      toast(e.message);
    }
  };
  const skipAsk = async () => {
    try {
      await AR.skipAnswer(sid);
      setAskQuestions([]);
      poll();
    } catch (e: any) {
      toast(e.message);
    }
  };
  const withdrawQueued = async (commandId: string) => {
    try {
      await AR.unqueue(sid, commandId);
      setQueued((prev) => prev.map((m) => (m.command_id === commandId ? { ...m, revoked: true } : m)));
    } catch (e: any) {
      toast(e.message);
    }
  };

  const saveGoalEdit = () => {
    const g = (goalEdit || "").trim();
    if (!g) return;
    AR.goal(sid, { action: "update", goal: g })
      .then(() => {
        setGoalPendingUpdate(g);
        setGoalEdit(null);
        toast("goal update queued", "info");
        return pollInspect();
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
    setDelegations([]);
    setSubAgentName(undefined);
    setGoal(null);
    setGoalPendingUpdate(null);
    setGoalDismissedAt(null);
    setAskQuestions([]);
    setQueued([]);
    setInspectReady(false);
    setEventsReady(false);
    setFailureRawOpen(false);
    setNotFound(false);
    gone.current = false;
    setLiveMode(undefined);
    // RT-7 · A sid the server's grammar cannot accept is not a session that might
    // show up later — it is a broken link. Settle on Not found immediately and
    // start NOTHING: no poll interval, no inspect interval, no EventSource.
    // (A well-formed but unknown id still takes the network path; the daemon's
    // 404 lands in the catch arms below and stops the pollers there.)
    if (!isValidSessionId(sid)) {
      setNotFound(true);
      gone.current = true;
      setEventsReady(true);
      setInspectReady(true);
      return;
    }
    poll();
    const e = setInterval(poll, 1000);
    const t = setInterval(pollInspect, 2500);
    pollInspect();
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
      // A nonexistent id can't ever stream; EventSource would otherwise
      // reconnect forever, re-running `ar` on every attempt (INC-41 L2).
      es.onerror = () => {
        if (gone.current) es?.close();
      };
    }
    return () => {
      clearInterval(e);
      clearInterval(t);
      es?.close();
    };
  }, [sid, isSub, poll, pollInspect]);

  const folded = useMemo(() => foldEvents(events), [events]);

  // Goal banner (W6): derive the goal's lifecycle from the durable journal so a
  // settled goal keeps a terminal banner instead of vanishing (inspect drops
  // it once done). A live clock ticks the active elapsed; a terminal banner is
  // dismissable as pure view state (keyed by the settlement time, so a NEW goal
  // re-shows and a page refresh reproduces it — no persistence).
  const goalState = useMemo(() => deriveGoalState(events), [events]);
  const goalTerminal = goalState ? isGoalTerminal(goalState.phase) : false;
  // Fix 3 · a run that settled as "achieved" gets an inline verdict on the
  // final assistant answer's action row. Reuses the same elapsed source that
  // drives GoalBanner (formatElapsed over goalState.elapsedMs).
  const goalVerdict =
    !isSub && goalState && goalState.phase === "achieved" && goalState.elapsedMs !== undefined
      ? { elapsed: formatElapsed(goalState.elapsedMs) }
      : null;
  const [now, setNow] = useState(() => Date.now());
  const [goalDismissedAt, setGoalDismissedAt] = useState<number | null>(null);
  useEffect(() => {
    if (!goalPendingUpdate) return;
    if (goalState?.goal === goalPendingUpdate || goal?.goal === goalPendingUpdate || goalTerminal) {
      setGoalPendingUpdate(null);
    }
  }, [goal?.goal, goalPendingUpdate, goalState?.goal, goalTerminal]);
  useEffect(() => {
    if (!goalState || goalTerminal) return;
    setNow(Date.now());
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [goalState?.phase, goalState?.attachedAt, goalTerminal]);
  const goalAction = (action: "pause" | "resume" | "cancel") =>
    AR.goal(sid, { action }).then(() => pollInspect()).catch((e) => toast(e.message));

  // Open approvals = journal asks not yet resolved + SSE-only child asks.
  const openApprovals: (ApprovalRef & {
    agent?: string;
    viaSSE?: boolean;
    session?: string;
    workspace?: string;
    workspaceMode?: string;
  })[] = [];
  for (const a of folded.approvals.values()) {
    if (!a.resolved && !resolvedLocal.has(a.id)) openApprovals.push(a);
  }
  for (const s of sseApprovals.values()) {
    if (folded.approvals.has(s.id) || resolvedLocal.has(s.id)) continue;
    openApprovals.push({ id: s.id, tool: s.tool, args: s.args, gates: [], agent: s.agent, viaSSE: true, session: s.session });
  }
  // G39: a child parked on an approval lives only in its own sub-journal —
  // the SSE ask above is live-only and vanishes on refresh. The inspect tree
  // now carries each child's waiting:approval durably; promote those into the
  // approval stack (decide targets the child session — routing already works).
  {
    const walk = (nodes: InspectNode[], ownerDelegations: InspectDelegation[]) => {
      for (const node of dedupeInspectNodes(nodes)) {
        const w = node.report?.waiting;
        const delegation = ownerDelegations.find((d) => d.assigned_to === node.session);
        if (w?.kind === "approval" && w.approval_id &&
            !folded.approvals.has(w.approval_id) && !resolvedLocal.has(w.approval_id)) {
          const existingIndex = openApprovals.findIndex((a) => a.id === w.approval_id);
          const childContext = {
            agent: node.agent,
            session: node.session,
            workspace: delegation?.workspace?.path,
            workspaceMode: delegation?.workspace?.mode,
          };
          if (existingIndex >= 0) {
            openApprovals[existingIndex] = { ...openApprovals[existingIndex], ...childContext };
          } else {
            openApprovals.push({
              id: w.approval_id, tool: w.tool || "", args: w.args || "", gates: [],
              ...childContext,
            });
          }
        }
        if (node.report?.children) walk(node.report.children, node.report.delegations || []);
      }
    };
    walk(children, delegations);
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
  const isBestOfN = folded.seriesKind === "best_of_n" || folded.seriesKind === "parallel";
  // QA-0719 S7: a user-initiated interrupt is NOT a stranded session. The
  // thread already says "Stopped — you interrupted this turn" and the very
  // next send resumes it — painting it with the recovery banner ("The
  // previous host stopped…" + Resume) misreads a deliberate act as a crash.
  // Recovery is for stranded sessions; interrupted keeps Retry (canRetry
  // below still matches it) and the ordinary composer.
  const needsRecovery = !live && /strand/i.test(listStatus || "");
  // Retry (INC-44 §B) re-sends the last user message as a NEW turn — offered
  // wherever the last one plausibly went wrong: crashed/failed/interrupted/
  // stranded, but never mid-run or while a wait wants its answer.
  const canRetry = !live && /strand|interrupt|crash|fail/i.test(listStatus || "");
  const running = status.cls === "run";
  // An explicitly-closed session still accepts input (a send reopens it), but
  // the composer alone reads as "live" — surface the closed state so it isn't
  // mistaken for an active conversation (R3-5).
  const isClosed = !live && status.text.toLowerCase() === "closed";
  const abnormalAgentCount = dedupeInspectNodes(children).filter((node) => {
    const childStatus = friendlyStatus(node.reason || node.report?.reason || node.report?.status || "");
    return childStatus.cls === "crash" || childStatus.cls === "stranded";
  }).length;
  const attentionCount = openApprovals.length + (needsRecovery ? 1 : 0) + abnormalAgentCount + (backgroundWork.length > 0 && !running ? 1 : 0);

  const doSend = async (text: string, images: string[], files: string[] = [], delivery?: "steer" | "queue",
    draft?: { draftId: string; sendRequestId: string;
      parts: Array<{ kind: "image" | "file"; ref?: string; path?: string; ordinal?: number }>;
      replayOriginal: boolean }) => {
    const id = ++pendSeq.current;
    setPending((p) => [...p, { id, text, imgs: images, files: files.length, delivery }]);
    try {
      const result = await AR.send(sid, text, images, files, delivery, draft);
      if (result?.status === "answered" || askQuestions.length > 0) {
        // A successful send while the durable structured ask form is visible
        // is a compatibility answer, even though today's CLI receipt says the
        // generic `delivered`. Remove its optimistic bubble from that exact
        // UI context too: journal polling alone can race past the receipt in
        // the real browser and leave a false queued row until reload.
        setPending((p) => p.filter((x) => x.id !== id));
      }
      if (delivery === "queue") {
        // Queue has its own durable, withdrawable projection. Once send has
        // acknowledged it, keeping the optimistic timeline bubble would show
        // the same message twice; after Withdraw it would become a ghost
        // forever because a revoked command never emits input_received.
        setPending((p) => p.filter((x) => x.id !== id));
        try {
          const q = await AR.queue(sid);
          setQueued(Array.isArray(q) ? q : []);
        } catch {
          // The regular poll will recover the durable queue card. Do not put
          // back a projection that can no longer be withdrawn truthfully.
        }
      }
    } catch (e: any) {
      toast(e.message);
      setPending((p) => p.filter((x) => x.id !== id));
      throw e;
    }
  };

  const continueFromMessage = async (item: BubbleItem) => {
    if (!item.itemId) return;
    let requestID = continueRequests.current.get(item.itemId);
    if (!requestID) {
      requestID = continuationRequestID(sid, item.itemId);
      continueRequests.current.set(item.itemId, requestID);
    }
    const result = await AR.continueFromMessage(sid, item.itemId, requestID);
    await useStore.getState().refreshSessions();
    select(result.session_id);
  };

  const forkDraft = useMemo<ForkDraft | null>(() => {
    const genesis = events.find((e) => e.type === "forked_from" && e.payload?.draft)?.payload?.draft as ForkDraft | undefined;
    if (!genesis) return null;
    let parkSeq = 0;
    for (const e of events) if (e.type === "fork_awaiting_input") parkSeq = Math.max(parkSeq, e.seq || 0);
    if (!parkSeq) return null;
    const consumed = events.some((e) => e.type === "input_received" && (e.seq || 0) > parkSeq &&
      ["", "user", "cli", "unix-socket", "tty"].includes(e.payload?.source || ""));
    return consumed ? null : genesis;
  }, [events]);
  const forkParked = useMemo(() => {
    let parkSeq = 0;
    for (const e of events) if (e.type === "fork_awaiting_input") parkSeq = Math.max(parkSeq, e.seq || 0);
    if (!parkSeq) return false;
    return !events.some((e) => e.type === "input_received" && (e.seq || 0) > parkSeq &&
      ["", "user", "cli", "unix-socket", "tty"].includes(e.payload?.source || ""));
  }, [events]);
  const forkSeedReleasedAt = useMemo(() => {
	let parkSeq = 0;
	for (const e of events) if (e.type === "fork_awaiting_input") parkSeq = Math.max(parkSeq, e.seq || 0);
	let released = 0;
	for (const e of events) {
		if ((e.seq || 0) <= parkSeq) continue;
		if ((e.type === "command_handled" && e.payload?.result === "input_rejected") || e.type === "input_revoked") {
			released = Math.max(released, e.seq || 0);
		}
	}
	return released;
  }, [events]);

  const decideApproval = async (id: string, decision: "approve" | "deny", reason: string, target = sid, always = false) => {
    await AR.approve(target, id, decision, reason, always);
    setResolvedLocal((s) => new Set(s).add(id));
    // Honest wording (G35/INC-62): the loop is the authority on what was
    // remembered — it emits a "remembered:" message only when the rule
    // actually persisted. The toast claims only the always-allow intent.
    if (always) toast("approved (always) — this session stops asking for this exact operation", "info");
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
    retry: async () => {
      try {
        await AR.retry(sid);
        toast("retrying your last message as a new turn", "info");
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
    inspect: async () => {
      try {
        const data = await AR.inspect(sid);
        // Reuse the product Run details projection already used elsewhere.
        // The terminal banner previously dumped the entire inspect JSON as the
        // modal body, making a two-iteration schedule several screens of debug
        // text. Raw data remains available behind the existing disclosure.
        openModal({ kind: "inspect", data, status: listStatus || folded.status.text });
      } catch (e: any) {
        toast(e.message);
      }
    },
  };

  // RT-5 · A model call that failed and never recovered. Shown while the
  // session is idle: if it's live, the runtime's own retry is still in flight
  // and an alarm would be premature. Sub-sessions are read-only (no retry).
  const failure = !live && !isSub && !isDriver ? folded.failure : undefined;
  const runFailureRetry = () => {
    setFailureRetrying(true);
    AR.retry(sid)
      .then(() => {
        toast("retrying your last message as a new turn", "info");
        return poll();
      })
      .catch((e: any) => toast(e.message))
      .finally(() => setFailureRetrying(false));
  };

  // The folded failure card has the provider-specific explanation, details,
  // and retry action. A generic "Session failed" terminal card below it says
  // the same fact with less help, so it is only a fallback when no detailed
  // failure was recovered from the journal.
  const terminalNotice = live || failure ? null : terminalNoticeFor(listStatus || folded.status.text, isDriver);
  const runTerminalAction = () => {
    if (!terminalNotice) return;
    if (terminalNotice.action === "continue") {
      openModal({ kind: "fork", sid });
      return;
    }
    if (terminalNotice.action === "resume") {
      void act.resume();
      return;
    }
    void act.inspect();
  };

  // TH-12 · the same terminal fact was landing on screen 3–5 times: a goal
  // cancellation as an in-thread chip AND the goal banner AND Supervision's
  // goal group; a step-limit stop as a red chip AND the terminal alert. Codex
  // says it once. The chrome above the composer is the actionable copy (it
  // carries the elapsed/checks and the "Continue in new session" button), so when
  // it's on screen the thread's echo of it is dropped — and ONLY then, so a
  // session with no banner (sub-agent, dismissed banner, no goal) still tells
  // the whole story from the thread alone.
  //
  // TH-14 (round 33) · TH-12 deduped the *thread*, but the chrome itself was
  // still saying it twice: a `terminal-alert` ("Step limit reached… Continue in
  // new session") with a `gbar` ("Goal cancelled · 00:34 · ✕") stacked underneath
  // it — 93px of banner about ONE ending, pinned above the composer, squeezing
  // the reading column to 630px of a 900px window. Codex pins nothing: its
  // terminal fact is a grey line in the last message that scrolls away with the
  // thread. We keep one banner (the abnormal ending is actionable and must not
  // scroll off), and the goal's outcome rides inside it as a meta tail rather
  // than as a second bar. So: the goal has a banner of its OWN only when there
  // is no terminal alert to ride on.
  const goalLive = !isSub && !!goalState && (!goalTerminal || goalDismissedAt !== goalState.endedAt);
  const goalBannerShown = goalLive && !terminalNotice;
  const goalInAlert = goalLive && !!terminalNotice;
  // The goal's label + elapsed, folded into the terminal alert's meta segment.
  // The goal *text* stays on the tooltip (and in the thread + Environment rail):
  // a banner that ellipsizes a sentence it has no room for said nothing anyway.
  const goalAlertMeta = goalInAlert && goalState
    ? {
        label: GOAL_TERMINAL_META[goalState.phase]?.label || "Goal",
        elapsedMs: goalTerminal
          ? goalState.elapsedMs
          : goalState.attachedAt !== undefined
            ? now - goalState.attachedAt
            : undefined,
        goal: goalState.goal,
      }
    : null;
  const threadItems = useMemo(
    // `goalLive` (not `goalBannerShown`): whether the goal rides in its own bar
    // or inside the terminal alert, the chrome above the composer has said it —
    // so the thread must not echo it a third time.
    () => suppressEchoedChips(folded.items, { goalBanner: goalLive, terminalAlert: !!terminalNotice || !!failure }),
    [folded.items, goalLive, terminalNotice, failure],
  );

  // The inline approval card is the primary action. On roomy desktop layouts
  // Supervision may reinforce it, but narrow screens must not be covered by an
  // auto-open overlay. At 900–1400px the 340px floating card overlaps the
  // primary approval decision, so temporarily close it and restore it after
  // the decision. A >1400px viewport has room for both and may reinforce the
  // inline card by auto-opening Environment.
  const hasApprovals = openApprovals.length > 0;
  useEffect(() => {
    if (hasApprovals && view === "chat") {
      if (bp.desktop && supervisionOpen) {
        approvalAdjustedSupervision.current ||= "closed";
        setSupervisionOpen(false);
      } else if (bp.wide && !supervisionOpen) {
        approvalAdjustedSupervision.current ||= "opened";
        setSupervisionOpen(true);
      }
      return;
    }
    if (approvalAdjustedSupervision.current === "opened") {
      setSupervisionOpen(false);
    } else if (approvalAdjustedSupervision.current === "closed") {
      setSupervisionOpen(true);
    }
    approvalAdjustedSupervision.current = null;
  }, [hasApprovals, view, bp.desktop, bp.wide]);

  const showSupervision = supervisionOpen && view === "chat";
  const visibleTitle = isSub && subAgentName ? subAgentName : title;

  // INC-41 L2 · The daemon knows no such session: everything below (timeline,
  // composer, Supervision) would be a working-looking shell over nothing. Every
  // hook above has already run, so this early return is safe.
  if (notFound) {
    return (
      <div className="session-view">
        <DaemonAlert />
        <main className="session-primary">
          <div className="timeline">
            <div className="tl-inner">
              <SessionNotFound sid={sid} onBack={() => select(null)} />
            </div>
          </div>
        </main>
      </div>
    );
  }

  return (
    <div className="session-view">
      <DaemonAlert />
      <header className="session-topbar">
        {/* Mobile navigation is an overlay owned by App. Reserve its 36px slot
            so it cannot cover the beginning of the session title. */}
        {(bp.compact || bp.tablet) && <span className="session-topbar-nav-slot h-9 w-9 shrink-0" aria-hidden="true" />}
        {isSub && (
          <button className="topbar-icon" onClick={() => select(sid.slice(0, sid.lastIndexOf(subMarker)))} title="Back to parent session">
            <ArrowLeft size={16} />
          </button>
        )}
        <div className="tt-left">
          {/* N-parity: the session title is prose, no leading file icon (weight
              change is handled in tw.css). */}
          <div className="tt-title" title={`${visibleTitle}${visibleTitle !== title ? `\n${title}` : ""}\n${sid}`}>{visibleTitle}</div>
          {isSub && <span className="readonly-tag">Read-only sub-agent</span>}
        </div>
        <span className="spacer" />
        {!isSub && needsRecovery && (
          <button className="topbar-tool recovery" onClick={act.resume} title="Resume this session from its last durable checkpoint" aria-label="Resume session">
            <ArrowClockwise size={15} /> <span className="topbar-tool-label">Resume</span>
          </button>
        )}
        {!isSub && canRetry && ((bp.desktop || bp.wide) || !needsRecovery) && (
          <button className="topbar-tool" onClick={act.retry} title="Re-send your last message as a new turn; double-clicks are idempotent" aria-label="Retry session">
            <ArrowClockwise size={15} /> <span className="topbar-tool-label">Retry</span>
          </button>
        )}
        {/* INC-41 TH-15 · ONE rail, ONE name, ONE door. The topbar used to carry
            two tool pills — `Changes` and `Supervision` — for what is a single
            mental object: the pill said "Supervision", the panel it opened was
            titled "Environment", and that panel's FIRST row was itself called
            "Changes". Three names, two doors, one thing. Codex names the rail
            `Environment` everywhere and keeps `Changes` as a row *inside* it;
            its topbar carries neither pill. So the Changes pill is gone (the
            rail's Changes row is the primary door, and `···` → Changes is the
            keyboard-free fallback), and the surviving pill wears the rail's own
            name and icon — click it and the label you land on is the label you
            clicked. */}
        <button className={`topbar-tool${showSupervision ? " active" : ""}`} onClick={() => {
          if (view === "diff") setView("chat");
          setSupervision(!showSupervision);
        }} title={showSupervision ? "Hide the Environment rail" : "Show the Environment rail — workspace changes, worktree, git, goal"}
          aria-label="Environment">
          <SlidersHorizontal size={16} /> <span className="topbar-tool-label">Environment</span>
          {attentionCount > 0 && <span className="topbar-attention">{attentionCount}</span>}
        </button>
        <Menu label={<DotsThree size={18} weight="bold" />} ariaLabel="More session actions">
          {/* Session organization leads; view and advanced actions follow. */}
          <MenuItem
            title="keep this session in a Pinned section at the top of the sidebar"
            onClick={() => {
              togglePin(sid);
              toast(pinned.includes(sid) ? "unpinned" : "pinned", "info");
            }}
          >
            <PushPin size={16} weight={pinned.includes(sid) ? "fill" : "regular"} />{pinned.includes(sid) ? "Unpin session" : "Pin session"}
          </MenuItem>
          <MenuItem
            title="give this session a custom name in the sidebar (stored in your browser)"
            onClick={() => openModal({ kind: "rename", sid })}
          >
            <PencilSimple size={16} />Rename session…
          </MenuItem>
          <MenuItem
            title="hide this session from the sidebar list (it stays on disk; toggle 'Show archived' to see it again)"
            onClick={() => {
              toggleArchive(sid);
              toast(archived.includes(sid) ? "unarchived" : "archived", "info");
            }}
          >
            <Archive size={16} />{archived.includes(sid) ? "Unarchive session" : "Archive session"}
          </MenuItem>
          <MenuLabel>View</MenuLabel>
          {view === "diff" ? (
            <MenuItem onClick={() => setView("chat")}><ChatCircle size={16} />Conversation</MenuItem>
          ) : (
            <MenuItem onClick={() => openDiff()}><Files size={16} />Changes</MenuItem>
          )}
          <MenuItem onClick={() => setSupervision(!supervisionOpen)}><SidebarSimple size={16} />{supervisionOpen ? "Hide" : "Show"} Environment</MenuItem>
          <MenuItem
            title="also show low-level system events (mode changes, effects, barriers…) inline in the timeline"
            onClick={toggleSys}
          >
            <Code size={16} />{showSys ? "Hide" : "Show"} system events
          </MenuItem>
          {!isSub && (
            <>
              <MenuLabel>Advanced</MenuLabel>
              <MenuItem
                title="checkpoint the session right now (ar barrier) so you can fork from this exact point later"
                onClick={act.barrier}
              >
                <Flag size={16} />Create checkpoint
              </MenuItem>
              <MenuItem
                title="continue from a checkpoint in a new session and worktree; this session is untouched"
                onClick={() => openModal({ kind: "fork", sid })}
              >
                <GitFork size={16} />Continue in new session…
              </MenuItem>
              <MenuItem
                title="swap this session's agent spec — context carries over; takes effect on your next message (spec_changed)"
                onClick={() => openModal({ kind: "agent", sid })}
              >
                <Robot size={16} />Switch agent…
              </MenuItem>
              {((canRetry && (bp.compact || bp.tablet)) || needsRecovery) && (
                <>
                  <MenuLabel>Run</MenuLabel>
                  {canRetry && (bp.compact || bp.tablet) && <MenuItem onClick={act.retry}><ArrowClockwise size={16} />Retry last message</MenuItem>}
                  {needsRecovery && <MenuItem onClick={act.resume}><ArrowClockwise size={16} />Resume session</MenuItem>}
                </>
              )}
            </>
          )}
        </Menu>
      </header>

      {findOpen && (
        <FindBar scope={() => document.querySelector<HTMLElement>(".timeline")} onClose={() => setFindOpen(false)} />
      )}
      {/* INC-41 RD-B · the Environment rail no longer owns a layout column. It's a
          floating card now (tw.css), so the thread keeps the full width
          whether the rail is open or shut — opening it used to shove the column
          the user was mid-sentence in 144px to the left. Changes (`view==="diff"`)
          is untouched: a review surface genuinely needs half the window, and gets
          it via the `.changes` track. */}
      <div className={`session-layout${view === "diff" ? " changes" : " single"}${showSupervision ? " environment" : ""}`}>
        <main className="session-primary">
          {/* The conversation stays mounted even while Changes is open — Codex
              shows the diff as a right-side split, not a full takeover (R1-2). */}
          {showSys && (
                <div className="system-events-notice">
                  System events are visible
                  <button onClick={toggleSys}>Hide</button>
                </div>
              )}
              <TimelineView
                items={threadItems}
                pending={pending}
                typing={running ? (typing || "Thinking") : typing}
                showSys={showSys}
                loading={!eventsReady}
                sentImages={sentImages.current}
                onContinue={continueFromMessage}
                statusLine={hasApprovals ? (
                  <div className={`run-status-line ${status.cls}`}>
                    <span>{status.text}</span>
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
                        workspace={approval.workspace || sessions.find((s) => s.id === sid)?.workspace}
                        workspaceMode={approval.workspaceMode}
                        onDecide={(id, decision, reason, always) => decideApproval(id, decision, reason, approval.session || sid, always)}
                        onError={(message) => toast(message)}
                      />
                    ))}
                  </div>
                ) : undefined}
                active={running}
                goalVerdict={goalVerdict}
                outcomeSlot={folded.items.some((item) => item.kind === "assistant") ? (
                  <ChangesOutcome sid={sid} refreshKey={events.length + workspaceEpoch} onReview={(scope) => openDiff(scope === "workspace" ? "working-tree" : "last-turn")} />
                ) : undefined}
              />
              {failure && (
                <div className="turn-error" role="alert">
                  <span className="turn-error-ic">
                    <WarningCircle size={17} weight="fill" />
                  </span>
                  <div className="turn-error-body">
                    <b>{failure.title}</b>
                    {failure.hint && <span className="turn-error-hint">{failure.hint}</span>}
                    <button
                      type="button"
                      className="turn-error-toggle"
                      aria-expanded={failureRawOpen}
                      onClick={() => setFailureRawOpen((v) => !v)}
                    >
                      {failureRawOpen ? "Hide technical details" : "Technical details"}
                    </button>
                    {failureRawOpen && <pre className="turn-error-raw">{failure.raw}</pre>}
                  </div>
                  <button
                    type="button"
                    className="turn-error-action"
                    disabled={failureRetrying}
                    onClick={runFailureRetry}
                    title="Re-send your last message as a new turn; double-clicks are idempotent"
                  >
                    <ArrowClockwise size={14} /> {failureRetrying ? "Retrying…" : "Retry"}
                  </button>
                </div>
              )}
              {terminalNotice?.action === "resume" && (
                <div
                  className={`terminal-alert ${terminalNotice.tone} grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-3 sm:grid-cols-[auto_minmax(0,1fr)_auto]`}
                  role="alert"
                >
                  <span className="terminal-alert-ic">
                    <WarningCircle size={17} weight="fill" />
                  </span>
                  <div className="min-w-0">
                    <b className="block leading-5">{terminalNotice.title}</b>
                    <span className="mt-1 block text-[12px] leading-[1.5] text-dim">{terminalNotice.body}</span>
                    {goalAlertMeta && (
                      <span className="terminal-alert-meta mt-2 flex gap-2" title={goalAlertMeta.goal}>
                        <span className="tam-label">{goalAlertMeta.label}</span>
                        {goalAlertMeta.elapsedMs !== undefined && <span>{formatElapsed(goalAlertMeta.elapsedMs)}</span>}
                      </span>
                    )}
                  </div>
                  <button
                    type="button"
                    className="terminal-alert-action col-span-2 flex w-full items-center justify-center gap-2 sm:col-span-1 sm:col-start-3 sm:row-start-1 sm:self-center sm:w-auto"
                    onClick={runTerminalAction}
                  >
                    <ArrowClockwise size={14} />
                    {terminalNotice.actionLabel}
                  </button>
                </div>
              )}
              {terminalNotice && terminalNotice.action !== "resume" && (
                /* Same responsive grid as the resume variant above: icon + text
                   columns, the action full-width on its own row below sm. The
                   old flex row let the intrinsic-width button (+ meta tail)
                   crush the text column to 4px on a 390px phone — one word per
                   line, title bleeding under the meta (QA v2sim). */
                <div
                  className={`terminal-alert ${terminalNotice.tone} grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-3 sm:grid-cols-[auto_minmax(0,1fr)_auto]`}
                  role="alert"
                >
                  <span className="terminal-alert-ic">
                    {terminalNotice.tone === "danger" ? <XCircle size={17} weight="fill" /> : <WarningCircle size={17} weight="fill" />}
                  </span>
                  <div className="terminal-alert-text" title={`${terminalNotice.title} — ${terminalNotice.body}`}>
                    <b>{terminalNotice.title}</b>
                    <span>{terminalNotice.body}</span>
                    {/* TH-14: the goal's ending rides HERE — inside the card, as
                        a meta tail — instead of as a second pinned bar. */}
                    {goalAlertMeta && (
                      <span className="terminal-alert-meta mt-2 flex gap-2" title={goalAlertMeta.goal}>
                        <span className="tam-label">{goalAlertMeta.label}</span>
                        {goalAlertMeta.elapsedMs !== undefined && <span>{formatElapsed(goalAlertMeta.elapsedMs)}</span>}
                      </span>
                    )}
                  </div>
                  <button
                    type="button"
                    className="terminal-alert-action col-span-2 flex w-full items-center justify-center gap-2 sm:col-span-1 sm:col-start-3 sm:row-start-1 sm:self-center sm:w-auto"
                    onClick={runTerminalAction}
                  >
                    {terminalNotice.actionLabel}
                  </button>
                </div>
              )}
              {goalBannerShown && goalState && (
                <GoalBanner
                  state={goalPendingUpdate ? { ...goalState, goal: goalPendingUpdate } : goalState}
                  elapsedMs={goalTerminal ? goalState.elapsedMs : goalState.attachedAt !== undefined ? now - goalState.attachedAt : undefined}
                  editing={goalEditSrc === "banner" ? goalEdit : null}
                  updatePending={!!goalPendingUpdate}
                  onEditStart={() => { setGoalEditSrc("banner"); setGoalEdit(goalState.goal); }}
                  onEditChange={setGoalEdit}
                  onSave={saveGoalEdit}
                  onDiscard={() => setGoalEdit(null)}
                  onAction={goalAction}
                  onDismiss={() => setGoalDismissedAt(goalState.endedAt ?? -1)}
                />
              )}
              {isDriver && <div className="driver-note">This scheduled run manages its own iterations and does not accept follow-up messages.</div>}
              {!live && folded.bestIter ? (
                <div className="driver-note">
                  {isBestOfN ? "Best-of-N winner" : "Selected iteration"}: #{folded.bestIter}.{" "}
                  <button
                    className="ghost"
                    onClick={async () => {
                      try {
                        const r = await AR.promote(sid);
                        toast(r.status || "winner applied", "info");
                      } catch (e: any) {
                        toast(String(e?.message || e), "error");
                      }
                    }}
                    title={`Apply the ${isBestOfN ? "winning attempt's" : "selected iteration's"} changes onto the project workspace (clean-or-nothing, lands unstaged)`}
                  >
                    {isBestOfN ? "Apply winner" : "Apply selected iteration"}
                  </button>
                </div>
              ) : null}
              {!isSub && !isDriver && isClosed && (
                <div className="driver-note">This conversation is idle — send a message to continue it.</div>
              )}
              {!isSub && askQuestions.length > 0 && (
                <AskForm questions={askQuestions} onSubmit={answerAsk} onSkip={skipAsk} />
              )}
              {!isSub && queued.filter((m) => !m.revoked).length > 0 && (
                <div className="queued-list">
                  {queued.filter((m) => !m.revoked).map((m) => {
                    // A child→parent mail arrives framed as
                    // "[message from <agent> (<child session id>)] body…" — the
                    // sender matters to a human, the internal session id does
                    // not (QA-0719 review #9). Named senders get a "from X"
                    // kicker; the raw frame stays in the title tooltip.
                    const framed = /^\[message from ([^\s(]+)[^\]]*\]\s*/.exec(m.text);
                    const body = framed ? m.text.slice(framed[0].length) : m.text;
                    return (
                      <div className="queued-row" key={m.command_id}>
                        <ClockCountdown size={15} className="queued-ic" aria-hidden="true" />
                        <span className="queued-kicker">{framed ? `Queued · from ${framed[1]}` : "Queued"}</span>
                        <span className="queued-text" title={m.text}>{body}</span>
                        <button className="queued-drop" onClick={() => withdrawQueued(m.command_id)} title="Withdraw this queued message before it runs">
                          Withdraw
                        </button>
                      </div>
                    );
                  })}
                </div>
              )}
              {!isSub && !isDriver && (
                <Composer
                  variant="session"
                  sid={sid}
                  workspace={sessions.find((session) => session.id === sid)?.workspace}
                  mode={liveMode}
                  running={running}
                  seed={forkDraft}
				  seedReleasedAt={forkSeedReleasedAt}
                  focusOnMount={forkParked}
                  onSend={doSend}
                  onError={(message) => toast(message)}
                  actions={{
                    interrupt: act.interrupt,
                    showDiff: () => openDiff(),
                    fork: () => openModal({ kind: "fork", sid }),
                    switchAgentAdvanced: () => openModal({ kind: "agent", sid }),
                    resume: act.resume,
                  }}
                />
              )}
        </main>
        {view === "diff" ? (
          <aside className="changes-panel session-side">
            {/* INC-41 RV-1 · no `.changes-panel-head` any more: it repeated the
                topbar's `Changes` pill (which is itself the toggle) and cost the
                rail 48px above a toolbar that could already wrap to two rows.
                Codex opens straight onto the diff under one toolbar; the ✕ moved
                into it (DiffView's `onClose`). */}
            <DiffView sid={sid} onClose={closeDiff} initialScope={diffScopeHint} />
          </aside>
        ) : showSupervision ? (
          <SupervisionPanel
            loading={!inspectReady}
            // QA-0719 S5: on budget-exhausted/stopped goals inspect keeps
            // returning the goal object (unlike `satisfied`, which drops it),
            // so the rail kept LIVE controls — Edit/Pause/Cancel on a goal
            // that already ended, surviving reload. The journal's terminal
            // verdict outranks inspect's projection: force the settled path.
            goal={goalTerminal ? null : goal && goalPendingUpdate ? { ...goal, goal: goalPendingUpdate } : goal}
            goalEdit={goalEditSrc === "panel" ? goalEdit : null}
            progress={progress}
            artifacts={artifacts}
            children={children}
            backgroundWork={backgroundWork}
            approvals={openApprovals.length}
            sessionIdle={!running}
            recovery={needsRecovery}
            // TH-14 · the chrome above the composer already carries this goal's
            // outcome (banner or terminal-alert tail) — the rail must not spend
            // a three-line block saying "Cancelled · 00:34 · 0 checks" a second
            // time. It collapses to one line.
            goalEchoed={goalLive && goalTerminal}
            // INC-41 RD-A · same tick the Changes card above the composer runs on
            // (`refreshKey={events.length}`, :890). The rail's git rows read once
            // on mount and then went blind — it could sit next to a card saying
            // "Edited 12 files" while still showing a clean tree. Now the stream
            // drives both. workspaceEpoch adds the UI-side mutations the stream
            // can't see (Undo/commit/push — QA-0719).
            refreshKey={events.length + workspaceEpoch}
            // TH-15 · the rail's Changes row used to open the diff by synthesising
            // a click on the topbar's Changes pill. That pill is gone, so the row
            // drives the view directly — which is what it should always have done.
            onOpenChanges={() => openDiff()}
            onGoalEdit={(text) => { setGoalEditSrc("panel"); setGoalEdit(text); }}
            onGoalSave={saveGoalEdit}
            onGoalDiscard={() => setGoalEdit(null)}
            onOpenArtifact={(stream, version) =>
              AR.artifact(sid, stream, version)
                .then((text) => openModal({ kind: "viewer", title: `${stream} · v${version}`, body: text }))
                .catch((error) => toast(error.message))}
            onGoalAction={(action) => AR.goal(sid, { action }).then(() => pollInspect()).catch((error) => toast(error.message))}
            onOpenChild={(childSid) => select(childSid)}
            onInspect={() => AR.inspect(sid).then((data) => openModal({
              kind: "inspect",
              data,
              status: sessions.find((session) => session.id === sid)?.status,
            })).catch((error) => toast(error.message))}
            onClose={() => setSupervision(false)}
          />
        ) : null}
      </div>
    </div>
  );
}

// GoalBanner is the persistent goal strip above the composer (W6). While active
// it shows the goal, a live elapsed clock, and edit/pause/cancel actions; once
// the goal settles it becomes a terminal banner (complete / stopped / cancelled)
// with total elapsed and a single dismiss. Codex form: ◎ Goal · text · elapsed.
const GOAL_TERMINAL_META: Record<string, { cls: string; label: string; sub?: string }> = {
  achieved: { cls: "done", label: "Goal complete" },
  stopped: { cls: "stopped", label: "Goal stopped", sub: "check budget exhausted" },
  cancelled: { cls: "cancelled", label: "Goal cancelled" },
};

function GoalBanner({
  state,
  elapsedMs,
  editing,
  updatePending,
  onEditStart,
  onEditChange,
  onSave,
  onDiscard,
  onAction,
  onDismiss,
}: {
  state: GoalDerived;
  elapsedMs?: number;
  editing: string | null;
  updatePending: boolean;
  onEditStart: () => void;
  onEditChange: (value: string) => void;
  onSave: () => void;
  onDiscard: () => void;
  onAction: (action: "pause" | "resume" | "cancel") => void;
  onDismiss: () => void;
}) {
  const terminal = GOAL_TERMINAL_META[state.phase];
  const elapsed = elapsedMs !== undefined ? formatElapsed(elapsedMs) : undefined;

  if (terminal) {
    const checks = state.phase !== "cancelled" && state.checks > 0
      ? `${state.checks} check${state.checks === 1 ? "" : "s"}`
      : undefined;
    return (
      <div className={`gbar ${terminal.cls}`} role="status">
        <span className="gbar-ico">
          {state.phase === "achieved" ? <CheckCircle size={16} weight="fill" /> : state.phase === "stopped" ? <WarningCircle size={16} weight="fill" /> : <Prohibit size={16} />}
        </span>
        <span className="gbar-label">{terminal.label}</span>
        {terminal.sub && <span className="gbar-sub">· {terminal.sub}</span>}
        <span className="gbar-text" title={state.goal}>{state.goal}</span>
        <span className="gbar-meta">
          {checks && <span>{checks}</span>}
          {elapsed && <span>{elapsed}</span>}
        </span>
        <button className="gbar-btn" onClick={onDismiss} title="Dismiss" aria-label="Dismiss goal banner">
          <X size={15} />
        </button>
      </div>
    );
  }

  const paused = state.phase === "paused";
  return (
    <div className={`gbar${paused ? " paused" : ""}`} role="status">
      <span className="gbar-ico"><Crosshair size={16} /></span>
      <span className="gbar-label">Goal{paused ? " · paused" : ""}</span>
      {editing === null ? (
        <span className="gbar-text" title={state.goal}>{state.goal}</span>
      ) : (
        <input
          className="gbar-input"
          autoFocus
          value={editing}
          onChange={(e) => onEditChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") onSave();
            if (e.key === "Escape") onDiscard();
          }}
        />
      )}
      {editing === null && (() => {
        // G3 · show N/M checks (a verifier budget) or the count so far, next to
        // the live elapsed clock — Codex's goal-banner progress read-out.
        const showChecks = state.maxChecks !== undefined || state.checks > 0;
        if (!showChecks && !elapsed && !updatePending) return null;
        const checksLabel = state.maxChecks !== undefined
          ? `${state.checks}/${state.maxChecks} checks`
          : `${state.checks} check${state.checks === 1 ? "" : "s"}`;
        return (
          <span className="gbar-meta">
            {showChecks && <span className="gbar-checks">{checksLabel}</span>}
            {elapsed && <span>{elapsed}</span>}
            {updatePending && <span>Update queued</span>}
          </span>
        );
      })()}
      <span className="gbar-actions">
        {editing === null ? (
          <>
            <button className="gbar-btn" onClick={onEditStart} title={updatePending ? "Goal update queued" : "Edit goal"} aria-label="Edit goal" disabled={updatePending}><PencilSimple size={15} /></button>
            <button className="gbar-btn" onClick={() => onAction(paused ? "resume" : "pause")} title={paused ? "Resume goal" : "Pause goal"} aria-label={paused ? "Resume goal" : "Pause goal"}>
              {paused ? <Play size={15} weight="fill" /> : <Pause size={15} weight="fill" />}
            </button>
            <button className="gbar-btn danger" onClick={() => onAction("cancel")} title="Cancel goal" aria-label="Cancel goal"><Trash size={15} /></button>
          </>
        ) : (
          <>
            <button className="gbar-btn text" onClick={onSave}>Save</button>
            <button className="gbar-btn text" onClick={onDiscard}>Discard</button>
          </>
        )}
      </span>
    </div>
  );
}
