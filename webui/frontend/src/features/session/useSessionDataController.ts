import { useCallback, useEffect, useRef, useState } from "react";
import { useAppServices, type AppEventStream } from "../../app/appServices";
import { useAppStoreApi, useStore } from "../../store";
import type { BackgroundWork, Envelope } from "../../types";
import type { BubbleItem } from "../../timeline";
import type { AskQuestion } from "../../components/AskForm";
import type { ProgressItem } from "../../components/SupervisionPanel";
import type { InspectDelegation, InspectNode } from "../../components/Subagents";
import { isSessionNotFound, isValidSessionId } from "./sessionIdentity";

export interface SSEApproval {
  id: string;
  tool: string;
  args: any;
  agent?: string;
  session?: string;
}

export interface PendingMessage {
  id: number;
  text: string;
  imgs: string[];
  files: number;
  delivery?: "steer" | "queue";
}

function continuationRequestID(
  storage: Storage,
  createID: () => string,
  sid: string,
  itemID: string,
): string {
  const key = `arwebui.continue.${sid}.${itemID}`;
  try {
    const prior = storage.getItem(key);
    if (prior) return prior;
    const id = createID();
    storage.setItem(key, id);
    return id;
  } catch {
    return createID();
  }
}

/**
 * Owns the session's remote state lifecycle. The view consumes its projection
 * and commands without knowing how journal polling, inspect polling, queue
 * reconciliation, SSE, idempotency, or store refreshes are wired.
 */
export function useSessionDataController({
  sid,
  isSub,
}: {
  sid: string;
  isSub: boolean;
}) {
  const { api, clock, ids, storage, streams } = useAppServices();
  const store = useAppStoreApi();
  const { select, toast } = useStore();

  const [events, setEvents] = useState<Envelope[]>([]);
  const [pending, setPending] = useState<PendingMessage[]>([]);
  const [typing, setTyping] = useState("");
  const [sseApprovals, setSseApprovals] = useState<Map<string, SSEApproval>>(new Map());
  const [resolvedLocal, setResolvedLocal] = useState<Set<string>>(new Set());
  const [backgroundWork, setBackgroundWork] = useState<BackgroundWork[]>([]);
  const [usage, setUsage] = useState<{ billed: number; steps: number } | null>(null);
  const [children, setChildren] = useState<InspectNode[]>([]);
  const [delegations, setDelegations] = useState<InspectDelegation[]>([]);
  const [subAgentName, setSubAgentName] = useState<string | undefined>();
  const [inspectReady, setInspectReady] = useState(false);
  const [eventsReady, setEventsReady] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [liveMode, setLiveMode] = useState<string | undefined>();
  const [goal, setGoal] = useState<{
    goal: string;
    checks: number;
    max_checks?: number;
    paused?: boolean;
    verifiers?: number;
    claimed?: boolean;
  } | null>(null);
  const [progress, setProgress] = useState<ProgressItem[]>([]);
  const [artifacts, setArtifacts] = useState<{ stream: string; version: number }[]>([]);
  const [askQuestions, setAskQuestions] = useState<AskQuestion[]>([]);
  const [queued, setQueued] = useState<{ command_id: string; text: string; revoked: boolean }[]>([]);

  const cursor = useRef(0);
  const pollBusy = useRef(false);
  const pendSeq = useRef(0);
  const sentImages = useRef(new Map<number, string[]>());
  const continueRequests = useRef<Map<string, string>>(new Map());
  const gone = useRef(false);

  const poll = useCallback(async () => {
    if (pollBusy.current || gone.current) return;
    pollBusy.current = true;
    try {
      const evs = await api.events(sid, cursor.current);
      if (evs.length) {
        setPending((prev) => {
          let next = prev;
          for (const event of evs) {
            if (
              event.type === "input_received" ||
              (event.type === "ask_resolved" && event.payload?.resolution === "answered")
            ) {
              const text =
                event.type === "ask_resolved" ? event.payload?.answer : event.payload?.text;
              const index = next.findIndex((item) => item.text === text);
              if (index >= 0) {
                if (next[index].imgs.length && event.seq) {
                  sentImages.current.set(event.seq, next[index].imgs);
                }
                next = next.filter((_, itemIndex) => itemIndex !== index);
              }
            }
            if (event.type === "assistant_message") setTyping("");
          }
          return next;
        });
        setEvents((previous) => [...previous, ...evs]);
        cursor.current = evs.reduce(
          (maximum, event) => Math.max(maximum, event.seq || 0),
          cursor.current,
        );
      }
    } catch (error) {
      if (isSessionNotFound(error)) {
        gone.current = true;
        setNotFound(true);
      }
    } finally {
      pollBusy.current = false;
      setEventsReady(true);
    }
  }, [api, sid]);

  const pollInspect = useCallback(async () => {
    if (gone.current) return;
    try {
      setBackgroundWork(await api.ps(sid));
    } catch {
      // Best-effort projection.
    }
    try {
      const inspect = await api.inspect(sid);
      const inspectUsage = inspect?.usage;
      if (inspectUsage) {
        setUsage({
          billed:
            inspectUsage.billed ??
            (inspectUsage.input_tokens || 0) + (inspectUsage.output_tokens || 0),
          steps: inspect.gen_steps || 0,
        });
      }
      setChildren(Array.isArray(inspect?.children) ? inspect.children : []);
      setDelegations(Array.isArray(inspect?.delegations) ? inspect.delegations : []);
      if (isSub && typeof inspect?.spec === "string" && inspect.spec.trim()) {
        setSubAgentName(inspect.spec.trim());
      }
      setGoal(inspect?.goal || null);
      setProgress(Array.isArray(inspect?.progress) ? inspect.progress : []);
      if (typeof inspect?.mode === "string" && inspect.mode) setLiveMode(inspect.mode);

      const latestArtifacts = new Map<string, number>();
      for (const artifact of Array.isArray(inspect?.artifacts) ? inspect.artifacts : []) {
        if (
          artifact?.stream &&
          (latestArtifacts.get(artifact.stream) || 0) < (artifact.version || 0)
        ) {
          latestArtifacts.set(artifact.stream, artifact.version);
        }
      }
      setArtifacts(
        [...latestArtifacts.entries()]
          .map(([stream, version]) => ({ stream, version }))
          .sort((left, right) => left.stream.localeCompare(right.stream)),
      );

      const waitingQuestions =
        inspect?.waiting?.kind === "input" ? inspect?.waiting?.ask_questions : undefined;
      setAskQuestions(Array.isArray(waitingQuestions) ? waitingQuestions : []);
    } catch (error) {
      if (isSessionNotFound(error)) {
        gone.current = true;
        setNotFound(true);
      }
    } finally {
      setInspectReady(true);
    }

    if (gone.current) return;
    try {
      const queue = await api.queue(sid);
      setQueued(Array.isArray(queue) ? queue : []);
    } catch {
      setQueued([]);
    }
  }, [api, isSub, sid]);

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
    setAskQuestions([]);
    setQueued([]);
    setInspectReady(false);
    setEventsReady(false);
    setNotFound(false);
    setLiveMode(undefined);
    gone.current = false;

    if (!isValidSessionId(sid)) {
      setNotFound(true);
      gone.current = true;
      setEventsReady(true);
      setInspectReady(true);
      return;
    }

    void poll();
    const eventTimer = clock.setInterval(poll, 1000);
    const inspectTimer = clock.setInterval(pollInspect, 2500);
    void pollInspect();

    const stream: AppEventStream = streams.open(`/api/sessions/${sid}/stream`);
    stream.onmessage = (message) => {
      let event: any;
      try {
        event = JSON.parse(message.data);
      } catch {
        return;
      }
      const foreign = event.session && event.session !== sid;
      if (!foreign && event.kind === "text_delta" && event.text) {
        setTyping((previous) => previous + event.text);
      }
      if (!foreign && event.kind === "discard") setTyping("");
      if (event.kind === "approval_request" && event.approval_id) {
        setSseApprovals((previous) => {
          const next = new Map(previous);
          next.set(event.approval_id, {
            id: event.approval_id,
            tool: event.tool,
            args: event.args,
            agent: event.text || (foreign ? event.session : ""),
            session: event.session || sid,
          });
          return next;
        });
      }
    };
    stream.addEventListener("end", () => stream.close());
    stream.onerror = () => {
      if (gone.current) stream.close();
    };

    return () => {
      clock.clearInterval(eventTimer);
      clock.clearInterval(inspectTimer);
      stream.close();
    };
  }, [clock, isSub, poll, pollInspect, sid, streams]);

  const answerAsk = useCallback(
    async (specs: string[]) => {
      try {
        await api.answer(sid, specs);
        setAskQuestions([]);
        void poll();
      } catch (error: any) {
        toast(error.message);
      }
    },
    [api, poll, sid, toast],
  );

  const skipAsk = useCallback(async () => {
    try {
      await api.skipAnswer(sid);
      setAskQuestions([]);
      void poll();
    } catch (error: any) {
      toast(error.message);
    }
  }, [api, poll, sid, toast]);

  const withdrawQueued = useCallback(
    async (commandId: string) => {
      try {
        await api.unqueue(sid, commandId);
        setQueued((previous) =>
          previous.map((message) =>
            message.command_id === commandId ? { ...message, revoked: true } : message,
          ),
        );
      } catch (error: any) {
        toast(error.message);
      }
    },
    [api, sid, toast],
  );

  const send = useCallback(
    async (
      text: string,
      images: string[],
      files: string[] = [],
      delivery?: "steer" | "queue",
      draft?: {
        draftId: string;
        sendRequestId: string;
        parts: Array<{
          kind: "image" | "file";
          ref?: string;
          path?: string;
          ordinal?: number;
        }>;
        replayOriginal: boolean;
      },
    ) => {
      const id = ++pendSeq.current;
      setPending((previous) => [
        ...previous,
        { id, text, imgs: images, files: files.length, delivery },
      ]);
      try {
        const result = await api.send(sid, text, images, files, delivery, draft);
        if (result?.status === "answered" || askQuestions.length > 0) {
          setPending((previous) => previous.filter((item) => item.id !== id));
        }
        if (delivery === "queue") {
          setPending((previous) => previous.filter((item) => item.id !== id));
          try {
            const queue = await api.queue(sid);
            setQueued(Array.isArray(queue) ? queue : []);
          } catch {
            // The inspect poll recovers the durable queue projection.
          }
        }
      } catch (error: any) {
        toast(error.message);
        setPending((previous) => previous.filter((item) => item.id !== id));
        throw error;
      }
    },
    [api, askQuestions.length, sid, toast],
  );

  const continueFromMessage = useCallback(
    async (item: BubbleItem) => {
      if (!item.itemId) return;
      let requestID = continueRequests.current.get(item.itemId);
      if (!requestID) {
        requestID = continuationRequestID(
          storage.session,
          () => ids.uuid("continue"),
          sid,
          item.itemId,
        );
        continueRequests.current.set(item.itemId, requestID);
      }
      const result = await api.continueFromMessage(sid, item.itemId, requestID);
      await store.getState().refreshSessions();
      select(result.session_id);
    },
    [api, ids, select, sid, storage.session, store],
  );

  const decideApproval = useCallback(
    async (
      id: string,
      decision: "approve" | "deny",
      reason: string,
      target = sid,
      always = false,
    ) => {
      await api.approve(target, id, decision, reason, always);
      setResolvedLocal((previous) => new Set(previous).add(id));
      if (always) {
        toast(
          "approved (always) — this session stops asking for this exact operation",
          "info",
        );
      }
    },
    [api, sid, toast],
  );

  return {
    events,
    pending,
    typing,
    sseApprovals,
    resolvedLocal,
    backgroundWork,
    usage,
    children,
    delegations,
    subAgentName,
    inspectReady,
    eventsReady,
    notFound,
    liveMode,
    goal,
    progress,
    artifacts,
    askQuestions,
    queued,
    sentImages,
    poll,
    pollInspect,
    answerAsk,
    skipAsk,
    withdrawQueued,
    send,
    continueFromMessage,
    decideApproval,
  };
}
