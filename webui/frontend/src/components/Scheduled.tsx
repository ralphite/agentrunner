import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, CaretDown, Crosshair, ArrowsClockwise, Stack, Play, X } from "@phosphor-icons/react";
import { useStore } from "../store";
import { ApiError } from "../api";
import { useAppServices } from "../app/appServices";
import { friendlyStatus } from "./pill";
import { projectLabel, scheduleLabel } from "../viewModels";
import { scheduledTitle } from "../scheduledTitle";
import { relTimeAgo, sessionDate } from "../time";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import type { Cadence, ScheduleDetail } from "../types";
import { scheduleFieldError } from "../scheduleValidate";
import { Modal } from "./Modals";
import { Button } from "../ui/Button";
import { Input, Select, Textarea } from "../ui/Field";
import { Spinner } from "../ui/Spinner";
import { StatusIndicator, type StatusIndicatorTone } from "../ui/StatusIndicator";
import { IconButton } from "../ui/IconButton";
import {
  ScheduledEmptyState,
  ScheduledRunActions,
  ScheduledRunItem,
  ScheduledSuggestions,
  ScheduledToolbar,
  type ScheduledFilter,
  type ScheduledRunItemModel,
} from "./ScheduledParts";

export { SUGGESTIONS } from "./ScheduledParts";

type Filter = ScheduledFilter;
const INITIAL_VISIBLE_ROWS = 5;
const ROWS_PER_PAGE = 10;

function statusTone(cls: string): StatusIndicatorTone {
  if (cls === "run") return "success";
  if (cls === "idle") return "info";
  if (cls === "appr" || cls === "stranded") return "warning";
  if (cls === "crash") return "danger";
  return "neutral";
}

// Static template suggestions (Codex parity). Clicking one opens the existing
// create-run modal prefilled for repeating work, with the description as the
// initial prompt. Colours are fixed to match Codex's accent glyphs.
//
// SC-18 — the card's rhythm is a SPEC, not a caption. Each suggestion used to
// carry its cadence as a hand-typed sentence while the click that follows opened
// the launcher on the Repeating preset's default `interval: 5m`: you clicked
// "Weekdays at 8:00 AM" and got a run that fires every five minutes. Two
// sources of truth, and the one on screen was the decorative one. Now a
// suggestion owns a real CadenceSpec — the same {schedule, cron, interval, n}
// fields the driver spec is built from and the server reads back
// (webui/schedule.go) — the card's words are RENDERED from it via cadenceText,
// and the click hands the very same spec to the modal. Change the cron here and
// the card, the form and the created schedule all move together; they cannot
// disagree, because there is nothing left to disagree with.

// SC-1 — what belongs on this page. A scheduled thing has a RHYTHM: left alone,
// it fires again. That is the whole reason the screen exists, and it is exactly
// what the schedule kind tells us (webui/schedule.go):
//
//   interval / cron   → a rhythm ("Every 30m", "Saturdays at 4:00 AM")   ✅
//   self_paced        → a driver that re-arms its own next iteration      ✅
//   immediate         → a one-shot run / a goal that runs until verified    ❌
//   parallel          → Best of N: attempts side by side, not a rhythm    ❌
//   (absent)          → a plain `submit` run: one-shot by construction    ❌
//
// Before this rule the page collected EVERY run and every driver session — 28
// rows, 26 of them "Runs once" / "Best of 3" — which buried the single genuinely
// scheduled run and pushed Suggestions off the first screen. The excluded work
// is not lost: one-shot runs stay reachable from ⌘K and their session lands in
// the sidebar like any other session.
const RHYTHMIC = new Set(["interval", "cron", "self_paced"]);

export function hasRhythm(c: Cadence): boolean {
  // A computed future tick is proof of a rhythm on its own; the server only
  // emits nextRunAt for a live interval/cron series.
  if (c.nextRunAt) return true;
  return RHYTHMIC.has((c.schedule || "").toLowerCase());
}

// Settled/terminal rows carry no useful colour on their leading dot — it reads
// as gray noise on every completed row (review sw-d-11). Drop the dot for these
// (a blank keeps the columns aligned); attention/running/unread still badge.
// Same semantics as the sidebar's W10 rule.
const SETTLED_STATUS = new Set(["done", "closed", "stopped", "cancelled"]);

// SC-10 — a BROKEN series must not look like a healthy one. `crash` ("Failed")
// and `stranded` ("Needs recovery") used to render as the same gray Circle as an
// idle-between-ticks row, with the status text hidden in a `title=` tooltip: a
// driver that advertised "Every 30m" but had been dead for four hours was
// pixel-identical to one about to fire. This hub exists to answer "are my
// background work still alive?", so a dead one has to say so on screen.
const ALERT_STATUS = new Set(["crash", "stranded"]);

// SC-16 — a CONFIGURED LIMIT is not a malfunction. `friendlyStatus` files
// max_iterations / max_generation_steps / budget under cls "stranded" (pill.ts),
// which is right for the session header's terminal banner ("Iteration limit reached
// — review the run before extending it") but catastrophic here: it painted a
// driver that ran exactly the N iterations you asked for in the same amber, with
// the same WarningCircle, as one whose host died mid-flight — and then filed it
// under Active, a series that will never fire again. Codex's list has zero
// alert colours; amber is a scarce resource and three rows shouting is nobody
// shouting.
//
// So this page judges the RAW status word itself, before friendlyStatus collapses
// it (the cls mapping stays untouched — SessionView depends on it). A limit row
// is settled: no alert colour and the neutral last-run sub-line.
const LIMIT_RE = /max_iterations|max_generation_steps|max_tokens|limit_exceeded|budget|step limit|token limit/i;

export function isLimitStatus(raw: string): boolean {
  return LIMIT_RE.test(raw || "");
}

// SC-11 — "Active" is a fact about the SERIES, not about this instant. Judging
// it by "an iteration is executing right now" (cls run/appr) made the tab
// structurally empty: a healthy `Every 30m` run is idle between ticks by
// construction. A series is active while it still has a future tick to fire,
// or while it is running / waiting on you / broken. Paused is a separate
// durable lifecycle, never inferred from a missing tick.
const LIVE_STATUS = new Set(["run", "appr", "stranded", "crash"]);

function seriesActive(cls: string, hasNextTick: boolean): boolean {
  return hasNextTick || LIVE_STATUS.has(cls);
}

type SchedRow = ScheduledRunItemModel;

// nextRunPhrase renders the backend's nextRunAt (RFC3339) as Codex's
// "Next run in 12m". A tick already due (an iteration is running, or the driver
// is catching up) says so instead of counting backwards.
function nextRunPhrase(iso: string | undefined, now: number): string {
  if (!iso) return "";
  const t = Date.parse(iso);
  if (isNaN(t)) return "";
  const sec = (t - now) / 1000;
  if (sec <= 30) return "Next run due now";
  const min = sec / 60;
  if (min < 60) return `Next run in ${Math.max(1, Math.round(min))}m`;
  const hr = min / 60;
  if (hr < 24) return `Next run in ${Math.floor(hr)}h`;
  const day = hr / 24;
  if (day < 7) return `Next run in ${Math.floor(day)}d`;
  const wk = day / 7;
  if (wk < 5) return `Next run in ${Math.floor(wk)}w`;
  return `Next run in ${Math.floor(day / 30)}mo`;
}

// SCH-ICON — the leading glyph, one per row, derived from the row's authoritative
// lifecycle facts.
//
//   broken      WarningCircle  amber/red — the ONE loud mark (SC-10), unchanged
//   running     PlayCircle     an iteration is executing this second
//   settled     CheckCircle    terminal: closed, or a limit you configured (SC-16)
//   active      Circle         a healthy series, idle between ticks (SC-11)
//   paused      PauseCircle    durable SeriesPaused lifecycle
function detailTime(iso?: string): string {
  if (!iso) return "Not scheduled";
  const d = new Date(iso);
  return isNaN(d.getTime())
    ? "Not available"
    : d.toLocaleString([], { dateStyle: "medium", timeStyle: "short" });
}

function reasoningText(detail: ScheduleDetail): string {
  if (!detail.thinkingEnabled) return "Off";
  if (detail.thinkingBudgetTokens) {
    return `${detail.thinkingBudgetTokens.toLocaleString()} token budget`;
  }
  return "Enabled";
}

export interface ScheduleDetailPanelProps {
  title: string;
  detail: ScheduleDetail | null;
  loading: boolean;
  error: string;
  acting: boolean;
  onClose: () => void;
  onRetry: () => void;
  onHistory: () => void;
  onCadence: (action: "pause" | "resume") => void;
  onEdit: () => void;
}

export function ScheduleDetailPanel({
  title,
  detail,
  loading,
  error,
  acting,
  onClose,
  onRetry,
  onHistory,
  onCadence,
  onEdit,
}: ScheduleDetailPanelProps) {
  const status = (detail?.status || "").toLowerCase() === "active"
    ? { text: "Active", cls: "run" }
    : friendlyStatus(detail?.status || "");
  const paused = (detail?.status || "").toLowerCase() === "paused";
  const project = detail?.workspace ? projectLabel(detail.workspace) : "No project";
  const model = detail?.model
    ? [detail.provider, detail.model].filter(Boolean).join(" · ")
    : "Not recorded";
  const overlap = detail?.overlap ? detail.overlap[0].toUpperCase() + detail.overlap.slice(1) : "Skip";
  const progress = detail?.maxIterations
    ? `${detail.iterations} of ${detail.maxIterations}`
    : `${detail?.iterations || 0}`;

  return (
    <aside className="schedule-detail" aria-label={`Schedule details for ${title}`}>
      <header className="schedule-detail-head">
        <IconButton
          className="schedule-detail-back-icon"
          variant="ghost"
          size="md"
          onClick={onClose}
          aria-label="Back to scheduled runs"
        >
          <ArrowLeft size={17} />
        </IconButton>
        <Button
          className="schedule-detail-back-label"
          variant="ghost"
          size="md"
          onClick={onClose}
          aria-label="Back to scheduled runs"
        >
          <ArrowLeft size={17} />
          <span>Scheduled</span>
        </Button>
        <IconButton
          className="schedule-detail-close"
          variant="ghost"
          size="md"
          onClick={onClose}
          aria-label="Close schedule details"
        >
          <X size={17} />
        </IconButton>
      </header>
      {loading ? (
        <Spinner className="schedule-detail-loading" display="standalone" label="Loading schedule details…" />
      ) : error ? (
        <div className="schedule-detail-error" role="alert">
          <b>Schedule details unavailable</b>
          <span>{error}</span>
          <Button variant="outline" onClick={onRetry}>Try again</Button>
        </div>
      ) : detail ? (
        <>
          <div className="schedule-detail-scroll">
            <div className="schedule-detail-title">
              <StatusIndicator
                className={`status ${status.cls}`}
                display="pill"
                label={status.text}
                tone={statusTone(status.cls)}
              />
              <h2>{title}</h2>
            </div>

            <div className="schedule-detail-prompt">{detail.prompt || "No standing prompt recorded."}</div>

            <section className="schedule-detail-section" aria-labelledby="schedule-detail-general">
              <h3 id="schedule-detail-general">Details</h3>
              <dl>
                <div><dt>Project</dt><dd title={detail.workspace}>{project}</dd></div>
                <div><dt>Agent</dt><dd>{detail.agent || "Default agent"}</dd></div>
                <div><dt>Model</dt><dd>{model}</dd></div>
                <div><dt>Reasoning</dt><dd>{reasoningText(detail)}</dd></div>
              </dl>
            </section>

            <section className="schedule-detail-section" aria-labelledby="schedule-detail-frequency">
              <div className="schedule-detail-section-head">
                <h3 id="schedule-detail-frequency">Frequency</h3>
                {detail.scheduleEdit && <Button size="sm" variant="ghost" onClick={onEdit}>Edit</Button>}
              </div>
              <dl>
                <div><dt>Cadence</dt><dd>{detail.cadence || scheduleLabel(detail.schedule)}</dd></div>
                <div><dt>Next run</dt><dd>{paused ? "Paused" : detailTime(detail.nextRunAt)}</dd></div>
                <div><dt>Overlap</dt><dd>{overlap}</dd></div>
                <div><dt>Iterations</dt><dd>{progress}</dd></div>
              </dl>
            </section>
          </div>
          <div className="schedule-detail-actions">
            {detail.scheduleControl && (
              <Button
                variant="solid"
                loading={acting}
                onClick={() => onCadence(paused ? "resume" : "pause")}
              >
                {paused ? "Resume" : "Pause"}
              </Button>
            )}
            <Button variant="outline" onClick={onHistory}>Open history</Button>
          </div>
        </>
      ) : null}
    </aside>
  );
}

export function ScheduleEditDialog({
  detail,
  onClose,
  onSaved,
}: {
  detail: ScheduleDetail;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
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
        ...(schedule === "interval" ? { interval: interval.trim() } : { cron: cron.trim() }),
        overlap,
      });
      await onSaved();
    } catch (e: any) {
      if (e instanceof ApiError && e.code === "schedule_conflict") {
        try {
          const latest = await api.scheduleDetail(detail.sessionId);
          setRevision(latest.revision);
        } catch {
          // Keep the draft and the old revision; a later Save will surface the
          // conflict again instead of replacing user input with guessed state.
        }
        setError("This schedule changed elsewhere. Your draft is preserved; review it, then save again.");
      } else {
        setError(e?.message || "The schedule could not be updated.");
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title="Edit schedule"
      onClose={onClose}
      footer={
        <>
          <Button variant="outline" disabled={busy} onClick={onClose}>Cancel</Button>
          <Button variant="solid" loading={busy} disabled={blocked} onClick={() => void save()}>
            Save
          </Button>
        </>
      }
    >
      <label className="field" htmlFor="schedule-edit-prompt">Prompt</label>
      <Textarea
        id="schedule-edit-prompt"
        rows={4}
        value={prompt}
        onChange={(event) => setPrompt(event.target.value)}
      />
      <label className="field" htmlFor="schedule-edit-repeat">Repeat</label>
      <div className="row-flex">
        <Select
          id="schedule-edit-repeat"
          value={schedule}
          onChange={(event) => setSchedule(event.target.value as "interval" | "cron")}
        >
          <option value="interval">Every interval</option>
          <option value="cron">Cron schedule</option>
        </Select>
        <Input
          aria-label={schedule === "interval" ? "Interval" : "Cron expression"}
          value={cadenceValue}
          onChange={(event) => schedule === "interval" ? setInterval(event.target.value) : setCron(event.target.value)}
          placeholder={schedule === "interval" ? "30m · 1h" : "0 8 * * 1-5"}
        />
      </div>
      {cadenceError && <div className="text-[12px] text-red" role="alert">{cadenceError}</div>}
      <label className="field" htmlFor="schedule-edit-overlap">If a run is still active</label>
      <Select
        id="schedule-edit-overlap"
        value={overlap}
        onChange={(event) => setOverlap(event.target.value as "skip" | "coalesce")}
      >
        <option value="skip">Skip missed runs</option>
        <option value="coalesce">Run once when available</option>
      </Select>
      {error && <div className="rounded-lg border border-line bg-bg p-3 text-[12px] text-red" role="alert">{error}</div>}
    </Modal>
  );
}

// Scheduled is Codex's Scheduled runs hub: repeating work that keeps running on
// its own (SC-1 — nothing one-shot lives here; see hasRhythm above). The two
// facts that justify a scheduled thing are the whole row — its CADENCE and its
// NEXT RUN (CX-3), both derived server-side from the driver spec
// (schedule/interval/cron/n) and served on /api/runs and /api/sessions. When
// there is no future tick to name the row falls back to the honest last-run
// time. Search + All / Active / Paused use the backend's durable lifecycle.
export function Scheduled() {
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
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE_ROWS);
  const [detail, setDetail] = useState<ScheduleDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [detailActing, setDetailActing] = useState(false);
  const [detailEditing, setDetailEditing] = useState(false);
  const detailRequest = useRef(0);
  const detailOpener = useRef<HTMLElement | null>(null);
  // SC-12 — the cursor-anchored row menu (same component the sidebar rows use).
  const [ctx, setCtx] = useState<{ x: number; y: number; key: string } | null>(null);

  const loadDetail = useCallback(async (sid: string) => {
    const request = ++detailRequest.current;
    setDetailLoading(true);
    setDetailError("");
    try {
      const next = await api.scheduleDetail(sid);
      if (request === detailRequest.current) setDetail(next);
    } catch (e: any) {
      if (request === detailRequest.current) {
        setDetail(null);
        setDetailError(e?.message || "The schedule could not be read.");
      }
    } finally {
      if (request === detailRequest.current) setDetailLoading(false);
    }
  }, []);

  const detailSession = sessions.find((session) => session.id === scheduledDetailSid);
  useEffect(() => {
    if (!scheduledDetailSid) {
      detailRequest.current++;
      setDetail(null);
      setDetailLoading(false);
      setDetailError("");
      return;
    }
    void loadDetail(scheduledDetailSid);
  }, [scheduledDetailSid, detailSession?.status, detailSession?.updatedAt, loadDetail]);

  const closeDetail = useCallback(() => {
    showScheduledDetail(null);
    setDetailEditing(false);
    const opener = detailOpener.current;
    requestAnimationFrame(() => {
      if (opener?.isConnected) opener.focus();
      else document.querySelector<HTMLElement>(".sched-search input, .scheduled-row")?.focus();
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

  const rows = useMemo<SchedRow[]>(() => {
    const flagged = new Set(unread);
    const out: SchedRow[] = [];
    // row assembles the sub-line: cadence first, then the next run (or, absent
    // one, when it last ran), then the project. It also decides the two facts
    // the row is judged by: whether the SERIES is still live (SC-11) and whether
    // it is broken and needs saying so (SC-10).
    const row = (
      base: Omit<
        SchedRow,
        "when" | "isNext" | "meta" | "active" | "paused" | "alert" | "title" | "running" | "settled" | "recover"
      >,
      nextRunAt: string | undefined,
      lastRan: Date | null,
    ): SchedRow => {
      const next = nextRunPhrase(nextRunAt, clock.now());
      const ago = relTimeAgo(lastRan);
      const paused = base.raw.toLowerCase() === "paused";
      const when = paused ? "Paused" : next || (ago ? `Ran ${ago}` : "");
      // SC-16 — a series that stopped at a limit you configured is FINISHED, not
      // broken: it wears no alert, it is not live, and it settles like any other
      // completed row. Only a crash or a lost host earns the amber.
      const limit = isLimitStatus(base.raw);
      const recover = !limit && ALERT_STATUS.has(base.status.cls);
      const settled = limit || SETTLED_STATUS.has(base.status.cls);
      const alert = recover ? base.status.text : "";
      // SC-13 — a user rename is the row's name, full stop; otherwise the name is
      // derived from the prompt (first clause, no tooling tail, ≤48 chars). The
      // raw text survives in `full`: it is the tooltip, and it is what search
      // reads, so shortening the label makes nothing unfindable.
      const custom = (renames[base.id] || "").trim();
      return {
        ...base,
        title: custom || scheduledTitle(base.full, base.id),
        when,
        isNext: !!next,
        alert,
        // A settled row is only "active" if the server still dates a future tick
        // for it (it does not, for a terminal series) — never because its cls
        // happens to be spelled "stranded".
        active: paused ? false : settled ? !!next : seriesActive(base.status.cls, !!next),
        paused,
        running: base.status.cls === "run",
        settled,
        recover,
        // The alert phrase is searchable too — "needs recovery" should find the
        // rows that do. So is the full prompt (`full`), matched at the call site.
        meta: [base.cadence, alert, when, base.project].filter(Boolean).join(" · "),
      };
    };
    for (const run of runs) {
      if (!hasRhythm(run)) continue; // SC-1: one-shot / best-of-N is not scheduled work
      // The series SESSION is the canonical row (INC-80.3): once the run's
      // session landed in the sessions list, the transient run row bows out
      // — one piece of scheduled work, one row.
      if (run.sessionId && sessions.some((s) => s.id === run.sessionId)) continue;
      const status = friendlyStatus(run.status);
      const ts = Date.parse(run.startedAt);
      const started = isNaN(ts) ? null : new Date(ts);
      out.push(
        row(
          {
            key: "run:" + run.id,
            id: run.id,
            kind: "run",
            full: run.label || run.id,
            // The rhythm comes from the run's spec; scheduleLabel is the coarse
            // kind we fall back to when the spec could not be read.
            cadence: run.cadence || scheduleLabel(run.schedule),
            project: projectLabel(run.workspace),
            workspace: run.workspace || "",
            raw: run.status || "",
            status,
            scheduleControl: false,
            scheduleDetail: false,
            unread: false,
            sortTs: isNaN(ts) ? 0 : ts,
            onClick: () => selectRun(run.id),
          },
          run.nextRunAt,
          started,
        ),
      );
    }
    for (const s of sessions) {
      if (s.kind !== "driver" || !hasRhythm(s)) continue; // SC-1: same rhythm bar
      const status = friendlyStatus(s.status);
      const d = sessionDate(s.id);
      out.push(
        row(
          {
            key: s.id,
            id: s.id,
            kind: "session",
            full: s.title || s.id,
            // cadence is the spec's real rhythm; scheduleLabel is the coarse
            // kind we fall back to when the journal could not be read.
            cadence: s.cadence || scheduleLabel(s.schedule),
            project: projectLabel(s.workspace),
            workspace: s.workspace || "",
            raw: s.status || "",
            status,
            scheduleControl: !!s.scheduleControl,
            scheduleDetail: !!s.scheduleDetail,
            unread: flagged.has(s.id),
            sortTs: d ? d.getTime() : 0,
            onClick: (opener) => {
              if (!s.scheduleDetail) {
                select(s.id);
                return;
              }
              detailOpener.current = opener || null;
              markRead(s.id);
              showScheduledDetail(s.id);
            },
          },
          s.nextRunAt,
          d,
        ),
      );
    }
    // Newest-first; the coloured status dot and label carry the state.
    out.sort((a, b) => b.sortTs - a.sortTs);
    return out;
  }, [runs, sessions, select, selectRun, showScheduledDetail, markRead, unread, renames]);

  const ql = query.trim().toLowerCase();
  const filtered = rows.filter((r) => {
    if (filter === "active" && !r.active) return false;
    if (filter === "paused" && !r.paused) return false;
    if (
      ql &&
      !(
        r.title.toLowerCase().includes(ql) ||
        r.full.toLowerCase().includes(ql) ||
        r.meta.toLowerCase().includes(ql)
      )
    )
      return false;
    return true;
  });
  useEffect(() => setVisibleCount(INITIAL_VISIBLE_ROWS), [filter, ql]);
  // Search is an exact retrieval surface: never hide matching results behind a
  // second interaction. Browsing the unqueried history uses progressive
  // disclosure so Suggestions remain discoverable in a compact viewport.
  const visibleRows = ql ? filtered : filtered.slice(0, visibleCount);
  const totalEmpty = rows.length === 0;

  // SC-21 — "Mark all as read" clears exactly the rows you can SEE. It used to
  // read the store-wide scheduledUnread() set, which also holds driver sessions
  // this page deliberately does not list (one-shot / goal drivers — SC-1) and the
  // rows the current tab or the search box has filtered away: one click could
  // silently clear unread state for work that was nowhere on screen. Scoping it to
  // `filtered` also makes the button's own presence honest — it appears only while
  // the view in front of you actually has something unread in it, and never as a
  // dead grey control. The sidebar's Scheduled dot keeps its own store-wide count
  // (viewModels.scheduledUnread, E3): that badge is about the whole hub.
  const unreadIds = visibleRows.filter((r) => r.unread).map((r) => r.id);

  // SC-14 — a search hit has to be VISIBLE. `meta` matches the project, but SC-4
  // took the project off the sub-line, so searching "scratch" returned a row on
  // which the word "scratch" appeared nowhere: the result read as a bug. Rather
  // than make the project unsearchable (it is the one fact people group work by),
  // the row grows a quiet chip naming the project — but only for the query that
  // matched it. No query, or a query that matched something already on screen,
  // and the sub-line stays the two facts Codex shows.
  const projectHit = (r: SchedRow) => !!ql && !!r.project && r.project.toLowerCase().includes(ql);

  const menuRow = ctx ? rows.find((r) => r.key === ctx.key) : undefined;

  // SC-12 — Stop is the same call RunView.tsx already makes for the same run
  // (api.stopRun + a refresh); the hub for long-running work was the one screen
  // that could not reach it.
  const stopRun = async (rid: string) => {
    try {
      await api.stopRun(rid);
      toast("stop requested", "info");
      clock.setTimeout(refreshRuns, 800);
    } catch (e: any) {
      toast(e.message);
    }
  };

  // SC-17 — the hub for long-running background work could DIAGNOSE a broken
  // schedule ("Needs recovery", in amber, since SC-10) and then do nothing about
  // it: every item in the row menu was housekeeping (pin / rename / archive /
  // copy). The daemon calls that fix a series already exist and SessionView
  // already makes them (api.resume / retry / stopSession); they
  // were simply unreachable from the one screen that names the problem. Same call
  // shapes as SessionView.tsx's `act`, plus a list refresh so the row's state
  // catches up with what you just did to it.
  //
  const act = {
    resume: async (sid: string) => {
      try {
        await api.resume(sid);
        toast("resume sent", "info");
        clock.setTimeout(refreshSessions, 800);
      } catch (e: any) {
        toast(e.message);
      }
    },
    retry: async (sid: string) => {
      try {
        await api.retry(sid);
        toast("starting a new scheduled series", "info");
        clock.setTimeout(refreshSessions, 800);
      } catch (e: any) {
        toast(e.message);
      }
    },
    cadence: async (sid: string, action: "pause" | "resume") => {
      if (scheduledDetailSid === sid) setDetailActing(true);
      try {
        await api.schedule(sid, action);
        toast(action === "pause" ? "pause recorded" : "resuming schedule", "info");
        await refreshSessions();
        if (scheduledDetailSid === sid) await loadDetail(sid);
      } catch (e: any) {
        toast(e.message);
      } finally {
        if (scheduledDetailSid === sid) setDetailActing(false);
      }
    },
    cancel: (sid: string) => {
      openModal({
        kind: "confirm",
        title: "Cancel this series?",
        body: "No more iterations will run. The series records its own cancelled terminal; the work already done stays on disk.",
        confirmLabel: "Cancel series",
        danger: true,
        onConfirm: async () => {
          await api.stopSession(sid);
          toast("cancelling the series", "info");
          clock.setTimeout(refreshSessions, 800);
        },
      });
    },
  };

  // Suggestions are a terminal block: every real scheduled run renders first,
  // and the canned templates always close the list at the very bottom, with
  // nothing after them (Codex parity — the gold reference only *looks* like a
  // mid-list split because it happens to carry two real runs; it never carves
  // the list apart). This holds for any number of real rows.
  const suggestions = (
    <ScheduledSuggestions
      onSelect={(suggestion, opener) =>
        openModal({
          kind: "run",
          preset: "repeating",
          prompt: suggestion.desc,
          cadence: suggestion.cadence,
          returnFocus: opener,
        })}
    />
  );
  const selectedDetailRow = scheduledDetailSid
    ? rows.find((row) => row.id === scheduledDetailSid)
    : undefined;
  const detailTitle =
    selectedDetailRow?.title ||
    detail?.name ||
    detailSession?.title ||
    "Scheduled run";

  return (
    <div className={"scheduled-shell" + (scheduledDetailSid ? " has-detail" : "")}>
      <main className="scheduled-page">
      <div className="page-heading">
        <div>
          <h2>Scheduled runs</h2>
          <p>Schedule repeating work, goals, or monitoring runs</p>
        </div>
        <div className="scheduled-create">
          <Menu
            ariaLabel="Create scheduled work"
            triggerClassName="page-action"
            label={<>Create <CaretDown size={13} /></>}
          >
            <MenuLabel>Create</MenuLabel>
            <MenuItem onClick={() => openModal({ kind: "run", preset: "one-time" })}>
              <Play size={15} /><span className="scheduled-create-option flex min-w-0 flex-1 flex-col gap-0.5"><b>One-time run</b><small>Run once in the background</small></span>
            </MenuItem>
            <MenuItem onClick={() => openModal({ kind: "run", preset: "goal" })}>
              <Crosshair size={15} /><span className="scheduled-create-option flex min-w-0 flex-1 flex-col gap-0.5"><b>Goal</b><small>Keep working until verified</small></span>
            </MenuItem>
            <MenuItem onClick={() => openModal({ kind: "run", preset: "repeating" })}>
              <ArrowsClockwise size={15} /><span className="scheduled-create-option flex min-w-0 flex-1 flex-col gap-0.5"><b>Repeating</b><small>Run on an interval or cron schedule</small></span>
            </MenuItem>
            <MenuItem onClick={() => openModal({ kind: "run", preset: "best-of-n" })}>
              <Stack size={15} /><span className="scheduled-create-option flex min-w-0 flex-1 flex-col gap-0.5"><b>Best of N</b><small>Run isolated attempts and select the best</small></span>
            </MenuItem>
          </Menu>
        </div>
      </div>

      {/* RS-3: two rows, as Codex has them — the search field owns a full row, and
          the filters sit on their own line below it (tabs left, Mark all as read
          right). The right-aligned button can appear and disappear with the unread
          set without ever nudging the tabs. */}
      {!totalEmpty && (
        <ScheduledToolbar
          query={query}
          filter={filter}
          unreadCount={unreadIds.length}
          onQueryChange={setQuery}
          onFilterChange={setFilter}
          onMarkAllRead={() => unreadIds.forEach(markRead)}
        />
      )}

      <div className="scheduled-list">
        {totalEmpty ? (
          <ScheduledEmptyState kind="empty" />
        ) : filtered.length === 0 ? (
          <ScheduledEmptyState
            kind={ql ? "search" : "filter"}
            query={query}
            filter={filter}
          />
        ) : (
          visibleRows.map((r) => {
            return (
              <ScheduledRunItem
                key={r.key}
                row={r}
                pinned={pinned.includes(r.id)}
                archived={archived.includes(r.id)}
                menuOpen={ctx?.key === r.key}
                showProjectHit={projectHit(r)}
                onOpenMenu={(x, y) => setCtx({ x, y, key: r.key })}
              />
            );
          })
        )}
        {!ql && filtered.length > INITIAL_VISIBLE_ROWS && (
          <div className="sched-disclosure">
            {visibleRows.length < filtered.length && (
              <button
                className="show-more"
                onClick={() => setVisibleCount((count) => count + ROWS_PER_PAGE)}
              >
                Show {Math.min(ROWS_PER_PAGE, filtered.length - visibleRows.length)} more · {filtered.length - visibleRows.length} remaining
              </button>
            )}
            {visibleCount > INITIAL_VISIBLE_ROWS && (
              <button
                className="show-more"
                onClick={() => setVisibleCount(INITIAL_VISIBLE_ROWS)}
              >
                Show fewer · newest {INITIAL_VISIBLE_ROWS}
              </button>
            )}
          </div>
        )}
        {suggestions}
      </div>

      {menuRow && ctx && (
        <ScheduledRunActions
          row={menuRow}
          x={ctx.x}
          y={ctx.y}
          pinned={pinned.includes(menuRow.id)}
          archived={archived.includes(menuRow.id)}
          unread={unread.includes(menuRow.id)}
          onClose={() => setCtx(null)}
          onResume={() =>
            void (menuRow.recover
              ? act.resume(menuRow.id)
              : act.cadence(menuRow.id, "resume"))}
          onRetry={() => void act.retry(menuRow.id)}
          onPause={() => void act.cadence(menuRow.id, "pause")}
          onCancel={() => act.cancel(menuRow.id)}
          onTogglePin={() => togglePin(menuRow.id)}
          onRename={() => openModal({ kind: "rename", sid: menuRow.id })}
          onToggleRead={() =>
            unread.includes(menuRow.id)
              ? markRead(menuRow.id)
              : markUnread(menuRow.id)}
          onToggleArchive={() => toggleArchive(menuRow.id)}
          onStop={() => void stopRun(menuRow.id)}
        />
      )}
      </main>
      {scheduledDetailSid && (
        <ScheduleDetailPanel
          title={detailTitle}
          detail={detail}
          loading={detailLoading}
          error={detailError}
          acting={detailActing}
          onClose={closeDetail}
          onRetry={() => void loadDetail(scheduledDetailSid)}
          onHistory={() => select(scheduledDetailSid)}
          onCadence={(action) => void act.cadence(scheduledDetailSid, action)}
          onEdit={() => setDetailEditing(true)}
        />
      )}
      {scheduledDetailSid && detail && detailEditing && (
        <ScheduleEditDialog
          detail={detail}
          onClose={() => setDetailEditing(false)}
          onSaved={async () => {
            setDetailEditing(false);
            toast("schedule updated", "info");
            await refreshSessions();
            await loadDetail(scheduledDetailSid);
          }}
        />
      )}
    </div>
  );
}
