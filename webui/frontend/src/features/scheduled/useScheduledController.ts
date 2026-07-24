import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { ApiError } from "../../api";
import { useAppServices } from "../../app/appServices";
import { scheduleFieldError } from "../../scheduleValidate";
import { scheduledTitle } from "../../scheduledTitle";
import { useStore } from "../../store";
import { relTimeAgo, sessionDate } from "../../time";
import type { Cadence, ScheduleDetail } from "../../types";
import { projectLabel, scheduleLabel } from "../../viewModels";
import { friendlyStatus } from "../../components/pill";
import type {
  ScheduledFilter,
  ScheduledRunItemModel,
  ScheduledSuggestion,
} from "../../components/ScheduledParts";

const INITIAL_VISIBLE_ROWS = 5;
const ROWS_PER_PAGE = 10;
const RHYTHMIC = new Set(["interval", "cron", "self_paced"]);
const SETTLED_STATUS = new Set(["done", "closed", "stopped", "cancelled"]);
const ALERT_STATUS = new Set(["crash", "stranded"]);
const LIVE_STATUS = new Set(["run", "appr", "stranded", "crash"]);
const LIMIT_RE =
  /max_iterations|max_generation_steps|max_tokens|limit_exceeded|budget|step limit|token limit/i;

export function hasRhythm(cadence: Cadence): boolean {
  if (cadence.nextRunAt) return true;
  return RHYTHMIC.has((cadence.schedule || "").toLowerCase());
}

export function isLimitStatus(raw: string): boolean {
  return LIMIT_RE.test(raw || "");
}

function seriesActive(statusClass: string, hasNextTick: boolean): boolean {
  return hasNextTick || LIVE_STATUS.has(statusClass);
}

function nextRunPhrase(iso: string | undefined, now: number): string {
  if (!iso) return "";
  const timestamp = Date.parse(iso);
  if (isNaN(timestamp)) return "";
  const seconds = (timestamp - now) / 1000;
  if (seconds <= 30) return "Next run due now";
  const minutes = seconds / 60;
  if (minutes < 60) return `Next run in ${Math.max(1, Math.round(minutes))}m`;
  const hours = minutes / 60;
  if (hours < 24) return `Next run in ${Math.floor(hours)}h`;
  const days = hours / 24;
  if (days < 7) return `Next run in ${Math.floor(days)}d`;
  const weeks = days / 7;
  if (weeks < 5) return `Next run in ${Math.floor(weeks)}w`;
  return `Next run in ${Math.floor(days / 30)}mo`;
}

export interface ScheduledContextMenu {
  x: number;
  y: number;
  key: string;
}

export interface ScheduledDetailController {
  sid: string | null;
  value: ScheduleDetail | null;
  loading: boolean;
  error: string;
  acting: boolean;
  editing: boolean;
  title: string;
  close: () => void;
  retry: () => void;
  openHistory: () => void;
  cadence: (action: "pause" | "resume") => void;
  beginEdit: () => void;
  closeEdit: () => void;
  saved: () => Promise<void>;
}

export interface ScheduledController {
  filter: ScheduledFilter;
  query: string;
  visibleRows: ScheduledRunItemModel[];
  filteredCount: number;
  totalEmpty: boolean;
  searching: boolean;
  visibleCount: number;
  unreadVisibleCount: number;
  contextMenu: ScheduledContextMenu | null;
  menuRow: ScheduledRunItemModel | undefined;
  detail: ScheduledDetailController;
  setFilter: (filter: ScheduledFilter) => void;
  setQuery: (query: string) => void;
  markAllVisibleRead: () => void;
  showMore: () => void;
  showFewer: () => void;
  projectMatched: (row: ScheduledRunItemModel) => boolean;
  isPinned: (id: string) => boolean;
  isArchived: (id: string) => boolean;
  isUnread: (id: string) => boolean;
  openContextMenu: (contextMenu: ScheduledContextMenu) => void;
  closeContextMenu: () => void;
  create: (
    preset: "one-time" | "goal" | "repeating" | "best-of-n",
  ) => void;
  selectSuggestion: (
    suggestion: ScheduledSuggestion,
    opener: HTMLElement,
  ) => void;
  resumeMenuRow: () => void;
  retryMenuRow: () => void;
  pauseMenuRow: () => void;
  cancelMenuRow: () => void;
  toggleMenuRowPin: () => void;
  renameMenuRow: () => void;
  toggleMenuRowRead: () => void;
  toggleMenuRowArchive: () => void;
  stopMenuRow: () => void;
}

export function useScheduledController(): ScheduledController {
  const { api, clock } = useAppServices();
  const {
    runs,
    sessions,
    select,
    selectRun,
    openModal,
    unread,
    markRead,
    markUnread,
    renames,
    pinned,
    togglePin,
    archived,
    toggleArchive,
    refreshRuns,
    refreshSessions,
    toast,
    scheduledDetailSid,
    showScheduledDetail,
  } = useStore();
  const [filter, setFilter] = useState<ScheduledFilter>("all");
  const [query, setQuery] = useState("");
  const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE_ROWS);
  const [detail, setDetail] = useState<ScheduleDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [detailActing, setDetailActing] = useState(false);
  const [detailEditing, setDetailEditing] = useState(false);
  const [contextMenu, setContextMenu] =
    useState<ScheduledContextMenu | null>(null);
  const detailRequest = useRef(0);
  const detailOpener = useRef<HTMLElement | null>(null);

  const loadDetail = useCallback(
    async (sid: string) => {
      const request = ++detailRequest.current;
      setDetailLoading(true);
      setDetailError("");
      try {
        const next = await api.scheduleDetail(sid);
        if (request === detailRequest.current) setDetail(next);
      } catch (error: unknown) {
        if (request === detailRequest.current) {
          setDetail(null);
          setDetailError(
            error instanceof Error
              ? error.message
              : "The schedule could not be read.",
          );
        }
      } finally {
        if (request === detailRequest.current) setDetailLoading(false);
      }
    },
    [api],
  );

  const detailSession = sessions.find(
    (session) => session.id === scheduledDetailSid,
  );
  useEffect(() => {
    if (!scheduledDetailSid) {
      detailRequest.current++;
      setDetail(null);
      setDetailLoading(false);
      setDetailError("");
      return;
    }
    void loadDetail(scheduledDetailSid);
  }, [
    scheduledDetailSid,
    detailSession?.status,
    detailSession?.updatedAt,
    loadDetail,
  ]);

  const closeDetail = useCallback(() => {
    showScheduledDetail(null);
    setDetailEditing(false);
    const opener = detailOpener.current;
    requestAnimationFrame(() => {
      if (opener?.isConnected) opener.focus();
      else
        document
          .querySelector<HTMLElement>(".sched-search input, .scheduled-row")
          ?.focus();
    });
  }, [showScheduledDetail]);

  useEffect(() => {
    if (!scheduledDetailSid) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return;
      event.preventDefault();
      closeDetail();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [scheduledDetailSid, closeDetail]);

  const rows = useMemo<ScheduledRunItemModel[]>(() => {
    const flagged = new Set(unread);
    const output: ScheduledRunItemModel[] = [];
    const row = (
      base: Omit<
        ScheduledRunItemModel,
        | "when"
        | "isNext"
        | "meta"
        | "active"
        | "paused"
        | "alert"
        | "title"
        | "running"
        | "settled"
        | "recover"
      >,
      nextRunAt: string | undefined,
      lastRan: Date | null,
    ): ScheduledRunItemModel => {
      const next = nextRunPhrase(nextRunAt, clock.now());
      const ago = relTimeAgo(lastRan);
      const paused = base.raw.toLowerCase() === "paused";
      const when = paused ? "Paused" : next || (ago ? `Ran ${ago}` : "");
      const limit = isLimitStatus(base.raw);
      const recover = !limit && ALERT_STATUS.has(base.status.cls);
      const settled = limit || SETTLED_STATUS.has(base.status.cls);
      const alert = recover ? base.status.text : "";
      const custom = (renames[base.id] || "").trim();
      return {
        ...base,
        title: custom || scheduledTitle(base.full, base.id),
        when,
        isNext: !!next,
        alert,
        active: paused
          ? false
          : settled
            ? !!next
            : seriesActive(base.status.cls, !!next),
        paused,
        running: base.status.cls === "run",
        settled,
        recover,
        meta: [base.cadence, alert, when, base.project]
          .filter(Boolean)
          .join(" · "),
      };
    };

    for (const run of runs) {
      if (!hasRhythm(run)) continue;
      if (
        run.sessionId &&
        sessions.some((session) => session.id === run.sessionId)
      )
        continue;
      const timestamp = Date.parse(run.startedAt);
      output.push(
        row(
          {
            key: `run:${run.id}`,
            id: run.id,
            kind: "run",
            full: run.label || run.id,
            cadence: run.cadence || scheduleLabel(run.schedule),
            project: projectLabel(run.workspace),
            workspace: run.workspace || "",
            raw: run.status || "",
            status: friendlyStatus(run.status),
            scheduleControl: false,
            scheduleDetail: false,
            unread: false,
            sortTs: isNaN(timestamp) ? 0 : timestamp,
            onClick: () => selectRun(run.id),
          },
          run.nextRunAt,
          isNaN(timestamp) ? null : new Date(timestamp),
        ),
      );
    }

    for (const session of sessions) {
      if (session.kind !== "driver" || !hasRhythm(session)) continue;
      const date = sessionDate(session.id);
      output.push(
        row(
          {
            key: session.id,
            id: session.id,
            kind: "session",
            full: session.title || session.id,
            cadence: session.cadence || scheduleLabel(session.schedule),
            project: projectLabel(session.workspace),
            workspace: session.workspace || "",
            raw: session.status || "",
            status: friendlyStatus(session.status),
            scheduleControl: !!session.scheduleControl,
            scheduleDetail: !!session.scheduleDetail,
            unread: flagged.has(session.id),
            sortTs: date ? date.getTime() : 0,
            onClick: (opener) => {
              if (!session.scheduleDetail) {
                select(session.id);
                return;
              }
              detailOpener.current = opener || null;
              markRead(session.id);
              showScheduledDetail(session.id);
            },
          },
          session.nextRunAt,
          date,
        ),
      );
    }

    output.sort((left, right) => right.sortTs - left.sortTs);
    return output;
  }, [
    clock,
    markRead,
    renames,
    runs,
    select,
    selectRun,
    sessions,
    showScheduledDetail,
    unread,
  ]);

  const normalizedQuery = query.trim().toLowerCase();
  const filteredRows = rows.filter((row) => {
    if (filter === "active" && !row.active) return false;
    if (filter === "paused" && !row.paused) return false;
    if (
      normalizedQuery &&
      !row.title.toLowerCase().includes(normalizedQuery) &&
      !row.full.toLowerCase().includes(normalizedQuery) &&
      !row.meta.toLowerCase().includes(normalizedQuery)
    )
      return false;
    return true;
  });
  useEffect(
    () => setVisibleCount(INITIAL_VISIBLE_ROWS),
    [filter, normalizedQuery],
  );
  const visibleRows = normalizedQuery
    ? filteredRows
    : filteredRows.slice(0, visibleCount);
  const unreadVisibleIds = visibleRows
    .filter((row) => row.unread)
    .map((row) => row.id);
  const menuRow = contextMenu
    ? rows.find((row) => row.key === contextMenu.key)
    : undefined;

  const stopRun = async (runId: string) => {
    try {
      await api.stopRun(runId);
      toast("stop requested", "info");
      clock.setTimeout(refreshRuns, 800);
    } catch (error: unknown) {
      toast(error instanceof Error ? error.message : "The run could not stop.");
    }
  };

  const resume = async (sessionId: string) => {
    try {
      await api.resume(sessionId);
      toast("resume sent", "info");
      clock.setTimeout(refreshSessions, 800);
    } catch (error: unknown) {
      toast(
        error instanceof Error ? error.message : "The series could not resume.",
      );
    }
  };

  const retry = async (sessionId: string) => {
    try {
      await api.retry(sessionId);
      toast("starting a new scheduled series", "info");
      clock.setTimeout(refreshSessions, 800);
    } catch (error: unknown) {
      toast(
        error instanceof Error ? error.message : "The series could not retry.",
      );
    }
  };

  const cadence = async (
    sessionId: string,
    action: "pause" | "resume",
  ) => {
    if (scheduledDetailSid === sessionId) setDetailActing(true);
    try {
      await api.schedule(sessionId, action);
      toast(
        action === "pause" ? "pause recorded" : "resuming schedule",
        "info",
      );
      await refreshSessions();
      if (scheduledDetailSid === sessionId) await loadDetail(sessionId);
    } catch (error: unknown) {
      toast(
        error instanceof Error
          ? error.message
          : "The schedule could not be changed.",
      );
    } finally {
      if (scheduledDetailSid === sessionId) setDetailActing(false);
    }
  };

  const cancel = (sessionId: string) => {
    openModal({
      kind: "confirm",
      title: "Cancel this series?",
      body: "No more iterations will run. The series records its own cancelled terminal; the work already done stays on disk.",
      confirmLabel: "Cancel series",
      danger: true,
      onConfirm: async () => {
        await api.stopSession(sessionId);
        toast("cancelling the series", "info");
        clock.setTimeout(refreshSessions, 800);
      },
    });
  };

  const requireMenuRow = (
    callback: (row: ScheduledRunItemModel) => void,
  ) => {
    if (menuRow) callback(menuRow);
  };

  const selectedDetailRow = scheduledDetailSid
    ? rows.find((row) => row.id === scheduledDetailSid)
    : undefined;
  const detailTitle =
    selectedDetailRow?.title ||
    detail?.name ||
    detailSession?.title ||
    "Scheduled run";

  return {
    filter,
    query,
    visibleRows,
    filteredCount: filteredRows.length,
    totalEmpty: rows.length === 0,
    searching: !!normalizedQuery,
    visibleCount,
    unreadVisibleCount: unreadVisibleIds.length,
    contextMenu,
    menuRow,
    setFilter,
    setQuery,
    markAllVisibleRead: () => unreadVisibleIds.forEach(markRead),
    showMore: () => setVisibleCount((count) => count + ROWS_PER_PAGE),
    showFewer: () => setVisibleCount(INITIAL_VISIBLE_ROWS),
    projectMatched: (row) =>
      !!normalizedQuery &&
      !!row.project &&
      row.project.toLowerCase().includes(normalizedQuery),
    isPinned: (id) => pinned.includes(id),
    isArchived: (id) => archived.includes(id),
    isUnread: (id) => unread.includes(id),
    openContextMenu: setContextMenu,
    closeContextMenu: () => setContextMenu(null),
    create: (preset) => openModal({ kind: "run", preset }),
    selectSuggestion: (suggestion, opener) =>
      openModal({
        kind: "run",
        preset: "repeating",
        prompt: suggestion.desc,
        cadence: suggestion.cadence,
        returnFocus: opener,
      }),
    resumeMenuRow: () =>
      requireMenuRow((row) => {
        void (row.recover ? resume(row.id) : cadence(row.id, "resume"));
      }),
    retryMenuRow: () =>
      requireMenuRow((row) => {
        void retry(row.id);
      }),
    pauseMenuRow: () =>
      requireMenuRow((row) => {
        void cadence(row.id, "pause");
      }),
    cancelMenuRow: () => requireMenuRow((row) => cancel(row.id)),
    toggleMenuRowPin: () => requireMenuRow((row) => togglePin(row.id)),
    renameMenuRow: () =>
      requireMenuRow((row) =>
        openModal({ kind: "rename", sid: row.id }),
      ),
    toggleMenuRowRead: () =>
      requireMenuRow((row) =>
        unread.includes(row.id) ? markRead(row.id) : markUnread(row.id),
      ),
    toggleMenuRowArchive: () =>
      requireMenuRow((row) => toggleArchive(row.id)),
    stopMenuRow: () =>
      requireMenuRow((row) => {
        void stopRun(row.id);
      }),
    detail: {
      sid: scheduledDetailSid,
      value: detail,
      loading: detailLoading,
      error: detailError,
      acting: detailActing,
      editing: detailEditing,
      title: detailTitle,
      close: closeDetail,
      retry: () => {
        if (scheduledDetailSid) void loadDetail(scheduledDetailSid);
      },
      openHistory: () => {
        if (scheduledDetailSid) select(scheduledDetailSid);
      },
      cadence: (action) => {
        if (scheduledDetailSid) void cadence(scheduledDetailSid, action);
      },
      beginEdit: () => setDetailEditing(true),
      closeEdit: () => setDetailEditing(false),
      saved: async () => {
        if (!scheduledDetailSid) return;
        setDetailEditing(false);
        toast("schedule updated", "info");
        await refreshSessions();
        await loadDetail(scheduledDetailSid);
      },
    },
  };
}

export interface ScheduleEditController {
  prompt: string;
  schedule: "interval" | "cron";
  cadenceValue: string;
  overlap: "skip" | "coalesce";
  cadenceError: string;
  busy: boolean;
  error: string;
  blocked: boolean;
  setPrompt: (value: string) => void;
  setSchedule: (value: "interval" | "cron") => void;
  setCadenceValue: (value: string) => void;
  setOverlap: (value: "skip" | "coalesce") => void;
  save: () => void;
}

export function useScheduleEditController(
  detail: ScheduleDetail,
  onSaved: () => Promise<void>,
): ScheduleEditController {
  const { api } = useAppServices();
  const [prompt, setPrompt] = useState(detail.prompt || "");
  const [schedule, setSchedule] = useState<"interval" | "cron">(
    detail.schedule === "cron" ? "cron" : "interval",
  );
  const [interval, setInterval] = useState(detail.interval || "30m");
  const [cron, setCron] = useState(detail.cron || "0 8 * * 1-5");
  const [overlap, setOverlap] = useState<"skip" | "coalesce">(
    detail.overlap === "coalesce" ? "coalesce" : "skip",
  );
  const [revision, setRevision] = useState(detail.revision);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const cadenceValue = schedule === "interval" ? interval : cron;
  const cadenceError = scheduleFieldError(schedule, cadenceValue);
  const blocked = !prompt.trim() || !cadenceValue.trim() || !!cadenceError;

  const save = async () => {
    if (blocked) return;
    setBusy(true);
    setError("");
    try {
      await api.scheduleUpdate(detail.sessionId, {
        expectedRevision: revision,
        prompt: prompt.trim(),
        schedule,
        ...(schedule === "interval"
          ? { interval: interval.trim() }
          : { cron: cron.trim() }),
        overlap,
      });
      await onSaved();
    } catch (caught: unknown) {
      if (caught instanceof ApiError && caught.code === "schedule_conflict") {
        try {
          const latest = await api.scheduleDetail(detail.sessionId);
          setRevision(latest.revision);
        } catch {
          // Keep the user's draft. A later Save will surface the conflict again.
        }
        setError(
          "This schedule changed elsewhere. Your draft is preserved; review it, then save again.",
        );
      } else {
        setError(
          caught instanceof Error
            ? caught.message
            : "The schedule could not be updated.",
        );
      }
    } finally {
      setBusy(false);
    }
  };

  return {
    prompt,
    schedule,
    cadenceValue,
    overlap,
    cadenceError,
    busy,
    error,
    blocked,
    setPrompt,
    setSchedule,
    setCadenceValue: (value) =>
      schedule === "interval" ? setInterval(value) : setCron(value),
    setOverlap,
    save: () => {
      void save();
    },
  };
}
