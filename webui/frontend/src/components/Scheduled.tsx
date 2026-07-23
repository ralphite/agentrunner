import { Fragment, useMemo, useState } from "react";
import type { Icon } from "@phosphor-icons/react";
import { CalendarDots, MagnifyingGlass, Check, CaretDown, Crosshair, ArrowsClockwise, Stack, Play, Bell, Notebook, FileMagnifyingGlass, Circle, PlayCircle, PauseCircle, CheckCircle, WarningCircle, DotsThree, PushPin } from "@phosphor-icons/react";
import { useStore } from "../store";
import { AR } from "../api";
import { friendlyStatus } from "./pill";
import { projectLabel, scheduleLabel } from "../viewModels";
import { scheduledTitle } from "../scheduledTitle";
import { relTimeAgo, sessionDate } from "../time";
import { ContextMenu } from "./ContextMenu";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import { cadenceText, type CadenceSpec } from "../runPreset";
import type { Cadence } from "../types";

// We have no real paused flag (nothing suspends a driver), so the third tab is
// the honest "Finished" — the rows that have stopped ticking — not Codex's
// "Paused" word borrowed for a different fact. The word stays; what changed
// (SC-11) is the test behind it, which is now about the SERIES rather than
// about whatever happens to be executing this second. See seriesActive.
type Filter = "all" | "active" | "finished";

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
interface Suggestion {
  icon: Icon;
  color: string;
  title: string;
  cadence: CadenceSpec;
  desc: string;
}
export const SUGGESTIONS: Suggestion[] = [
  {
    icon: Bell,
    color: "#3b82f6",
    title: "Daily brief",
    cadence: { schedule: "cron", cron: "0 8 * * 1-5" }, // Weekdays at 8:00 AM
    desc: "Start each weekday with a summary of your priorities",
  },
  {
    icon: Notebook,
    color: "#8b5cf6",
    title: "Weekly review",
    cadence: { schedule: "cron", cron: "0 16 * * 5" }, // Fridays at 4:00 PM
    desc: "Summarize the week's changes and open work",
  },
  {
    icon: FileMagnifyingGlass,
    color: "#22c55e",
    title: "Follow-up monitor",
    cadence: { schedule: "cron", cron: "0 */6 * * *" }, // Every 6 hours
    desc: "Watch for failures and follow up",
  },
];

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
// under Active, a series that will never fire again. Live: 3/3 rows amber,
// All=3 / Active=3 / Finished=0. Codex's list has zero alert colours; amber is a
// scarce resource and three rows shouting is nobody shouting.
//
// So this page judges the RAW status word itself, before friendlyStatus collapses
// it (the cls mapping stays untouched — SessionView depends on it). A limit row
// is settled: no alert colour, no WarningCircle, an empty glyph slot and the
// neutral "Ran 2d ago" sub-line every other finished row wears.
const LIMIT_RE = /max_iterations|max_generation_steps|max_tokens|limit_exceeded|budget|step limit|token limit/i;

export function isLimitStatus(raw: string): boolean {
  return LIMIT_RE.test(raw || "");
}

// SC-11 — "Active" is a fact about the SERIES, not about this instant. Judging
// it by "an iteration is executing right now" (cls run/appr) made the tab
// structurally empty: a healthy `Every 30m` run is idle between ticks by
// construction, so every well-behaved series was filed under Finished — which
// then advertised its cadence and its next run, a flat lie. A series is active
// while it still has a future tick to fire, or while it is running / waiting on
// you / broken (a crashed or stranded series is emphatically not finished — it
// is waiting for YOU). Finished means terminal: nothing more will ever happen.
const LIVE_STATUS = new Set(["run", "appr", "stranded", "crash"]);

function seriesActive(cls: string, hasNextTick: boolean): boolean {
  return hasNextTick || LIVE_STATUS.has(cls);
}

interface SchedRow {
  key: string;
  id: string; // the store id this row is about (session id / run id)
  kind: "session" | "run"; // which of the two things it is — decides its actions (SC-12)
  title: string; // SC-13: the derived NAME (short, scannable); never the whole prompt — this is what the row SHOWS
  full: string; // the raw label/prompt — the tooltip, and what search reads
  cadence: string; // the rhythm: "Every 30m" / "Saturdays at 4:00 AM" / "Self-paced"
  when: string; // "Next run in 12m" when known, else the honest "Ran 1d ago"
  isNext: boolean; // when names a FUTURE tick (styled as the live fact it is)
  alert: string; // SC-10: "Failed" / "Needs recovery" — shown, not tooltipped
  project: string;
  workspace: string;
  raw: string; // the daemon's own status word, before friendlyStatus collapses it
  meta: string; // the row's facts flattened (project included), for search
  status: { text: string; cls: string };
  active: boolean; // the series still has ticks coming / needs you (SC-11)
  running: boolean; // an iteration is executing right now — the only stoppable state
  settled: boolean; // nothing more will happen: closed/done/stopped, or a limit (SC-16)
  recover: boolean; // genuinely broken (crash/stranded) — Resume is the fix (SC-17)
  unread: boolean; // driver row with new activity you haven't opened (F2)
  sortTs: number;
  onClick: () => void;
}

// nextRunPhrase renders the backend's nextRunAt (RFC3339) as Codex's
// "Next run in 12m". A tick already due (an iteration is running, or the driver
// is catching up) says so instead of counting backwards.
function nextRunPhrase(iso?: string): string {
  if (!iso) return "";
  const t = Date.parse(iso);
  if (isNaN(t)) return "";
  const sec = (t - Date.now()) / 1000;
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

// SCH-ICON — the leading glyph, one per row, derived from facts the row already
// holds. No new state, no new backend field: just a mark for each of the five
// answers this page exists to give at a glance.
//
//   broken      WarningCircle  amber/red — the ONE loud mark (SC-10), unchanged
//   running     PlayCircle     an iteration is executing this second
//   settled     CheckCircle    terminal: closed, or a limit you configured (SC-16)
//   active      Circle         a healthy series, idle between ticks (SC-11)
//   dormant     PauseCircle    no future tick, but not terminal either
//
// The last one is the honest local reading of Codex's paused ⊘: nothing suspends
// a driver here (see the Filter comment), so "paused" is not a flag we can read —
// but a series with no next tick that has not finished has, in fact, stopped
// ticking, and saying so is the whole point of the column.
export function glyphFor(r: Pick<SchedRow, "alert" | "running" | "settled" | "active">) {
  const size = 16;
  if (r.alert) return <WarningCircle size={size} weight="regular" />;
  if (r.running) return <PlayCircle size={size} weight="regular" />;
  if (r.settled) return <CheckCircle size={size} weight="regular" />;
  if (r.active) return <Circle size={size} weight="regular" />;
  return <PauseCircle size={size} weight="regular" />;
}

// SCH-ICON — a row that is not going to fire again should not shout as loudly as
// one that is. Codex greys the whole paused row, title included (`cloc` in the
// reference crop); ours used to paint a finished series' title in the same full
// ink as a live one, so the list read as one undifferentiated column of black.
// A broken row is never quiet — it is the one row that must keep its emphasis.
function isQuiet(r: SchedRow): boolean {
  return !r.recover && !r.active;
}

// Scheduled is Codex's Scheduled runs hub: repeating work that keeps running on
// its own (SC-1 — nothing one-shot lives here; see hasRhythm above). The two
// facts that justify a scheduled thing are the whole row — its CADENCE and its
// NEXT RUN (CX-3), both derived server-side from the driver spec
// (schedule/interval/cron/n) and served on /api/runs and /api/sessions. When
// there is no future tick to name (a finished series, a self-paced driver) the
// row falls back to the honest "Ran 1d ago" — never a fabricated time. Search +
// All / Active / Finished filters map to our live-vs-finished states (INC-41
// W7, SC-7).
export function Scheduled() {
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
  } = useStore();
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  // SC-12 — the cursor-anchored row menu (same component the sidebar rows use).
  const [ctx, setCtx] = useState<{ x: number; y: number; key: string } | null>(null);

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
        "when" | "isNext" | "meta" | "active" | "alert" | "title" | "running" | "settled" | "recover"
      >,
      nextRunAt: string | undefined,
      lastRan: Date | null,
    ): SchedRow => {
      const next = nextRunPhrase(nextRunAt);
      const ago = relTimeAgo(lastRan);
      const when = next || (ago ? `Ran ${ago}` : "");
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
        active: settled ? !!next : seriesActive(base.status.cls, !!next),
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
            unread: flagged.has(s.id),
            sortTs: d ? d.getTime() : 0,
            onClick: () => select(s.id),
          },
          s.nextRunAt,
          d,
        ),
      );
    }
    // Newest-first; the coloured status dot and label carry the state.
    out.sort((a, b) => b.sortTs - a.sortTs);
    return out;
  }, [runs, sessions, select, selectRun, unread, renames]);

  // Nothing suspends a driver, so there is no paused set to show: the third tab
  // names what we actually have — the rows that have stopped ticking for good
  // (SC-11: a healthy series idling between ticks is still Active). Codex shows
  // no numeric counts on the pills (N-parity), so we only compute the filter.
  const ql = query.trim().toLowerCase();
  const filtered = rows.filter((r) => {
    if (filter === "active" && !r.active) return false;
    if (filter === "finished" && r.active) return false;
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
  const unreadIds = filtered.filter((r) => r.unread).map((r) => r.id);

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
  // (AR.stopRun + a refresh); the hub for long-running work was the one screen
  // that could not reach it.
  const stopRun = async (rid: string) => {
    try {
      await AR.stopRun(rid);
      toast("stop requested", "info");
      setTimeout(refreshRuns, 800);
    } catch (e: any) {
      toast(e.message);
    }
  };

  // SC-17 — the hub for long-running background work could DIAGNOSE a broken
  // schedule ("Needs recovery", in amber, since SC-10) and then do nothing about
  // it: every item in the row menu was housekeeping (pin / rename / archive /
  // copy). The daemon calls that fix a series already exist and SessionView
  // already makes them (AR.resume / retry / stopSession); they
  // were simply unreachable from the one screen that names the problem. Same call
  // shapes as SessionView.tsx's `act`, plus a list refresh so the row's state
  // catches up with what you just did to it.
  //
  // Deliberately NOT here: Codex's pause / run-now / delete-schedule. There is no
  // daemon suspend/trigger/delete endpoint behind them, and a menu item that
  // cannot do what it says is worse than one that is missing.
  const act = {
    resume: async (sid: string) => {
      try {
        await AR.resume(sid);
        toast("resume sent", "info");
        setTimeout(refreshSessions, 800);
      } catch (e: any) {
        toast(e.message);
      }
    },
    retry: async (sid: string) => {
      try {
        await AR.retry(sid);
		toast("starting a new scheduled series", "info");
        setTimeout(refreshSessions, 800);
      } catch (e: any) {
        toast(e.message);
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
          await AR.stopSession(sid);
          toast("cancelling the series", "info");
          setTimeout(refreshSessions, 800);
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
    <div className="sched-suggestions" data-testid="scheduled-suggestions">
      <div className="sched-suggestions-title">Suggestions</div>
      {SUGGESTIONS.map((s) => {
        const Ic = s.icon;
        return (
          <button
            key={s.title}
            className="sched-suggest"
            // SC-18: the rhythm rides along with the prompt, so the modal
            // opens on the cadence this card just promised.
            onClick={(event) => openModal({
              kind: "run",
              preset: "repeating",
              prompt: s.desc,
              cadence: s.cadence,
              // Pointer click on macOS may leave BODY active. Pass the durable
              // card explicitly so dismiss can still restore the real opener.
              returnFocus: event.currentTarget,
            })}
          >
            <span className="sched-suggest-icon">
              <Ic size={22} color={s.color} />
            </span>
            <span
              className="sched-suggest-body flex min-w-0 flex-1 flex-col gap-1"
              style={{ display: "flex", flexDirection: "column", gap: 4 }}
            >
              <span
                className="sched-suggest-head flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5"
                style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", columnGap: 8, rowGap: 2 }}
              >
                <b className="sched-suggest-title font-semibold">{s.title}</b>
                {/* SC-18: rendered from the spec above — never a second,
                    hand-written copy of it. */}
                <span className="sched-suggest-cadence">{cadenceText(s.cadence)}</span>
              </span>
              <span className="sched-suggest-desc block" style={{ display: "block" }}>{s.desc}</span>
            </span>
          </button>
        );
      })}
    </div>
  );

  return (
    <div className="scheduled-page">
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
        <div className="sched-toolbar">
          <div className="sched-search">
            <MagnifyingGlass size={15} />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search scheduled runs"
              aria-label="Search scheduled runs"
            />
          </div>
          <div className="sched-filters">
            <div className="sched-tabs" role="tablist" aria-label="Filter scheduled work">
              {(["all", "active", "finished"] as Filter[]).map((f) => (
                <button
                  key={f}
                  role="tab"
                  aria-selected={filter === f}
                  className={"sched-tab" + (filter === f ? " on" : "")}
                  onClick={() => setFilter(f)}
                >
                  {f[0].toUpperCase() + f.slice(1)}
                </button>
              ))}
            </div>
            {unreadIds.length > 0 && (
              <button
                className="sched-markread"
                onClick={() => unreadIds.forEach(markRead)}
                title="Mark all scheduled activity as read"
              >
                <Check size={14} /> Mark all as read
              </button>
            )}
          </div>
        </div>
      )}

      <div className="scheduled-list">
        {totalEmpty ? (
          <div className="empty-state">
            <CalendarDots size={28} />
            <b>No scheduled work</b>
            <span>Start a repeating run when work should keep running on its own.</span>
          </div>
        ) : filtered.length === 0 ? (
          <div className="empty-state">
            <CalendarDots size={28} />
            <b>Nothing here</b>
            <span>
              {ql
                ? `No results for "${query.trim()}".`
                : filter === "all"
                  ? "No work matches this view."
                  : `No ${filter} work matches this view.`}
            </span>
          </div>
        ) : (
          filtered.map((r) => {
            const isPinned = pinned.includes(r.id);
            const isArchived = archived.includes(r.id);
            const hasActions = r.kind === "session" || r.running;
            const openMenu = (x: number, y: number) => setCtx({ x, y, key: r.key });
            return (
            <Fragment key={r.key}>
            <div
              className={
                "scheduled-row-wrap relative" +
                (r.unread ? " is-unread" : "") +
                (isArchived ? " is-archived" : "") +
                (ctx?.key === r.key ? " menu-open" : "")
              }
              onContextMenu={(e) => {
                if (!hasActions) return;
                e.preventDefault();
                openMenu(e.clientX, e.clientY);
              }}
            >
            <button
              className={
                // SCH-ICON-TOP: the leading glyph anchors the row's left column;
                // it must ride the TITLE'S FIRST LINE (Codex gold), not float to
                // the vertical middle of a two-line title. items-start tops the
                // glyph slot with the title; the glyph's own -mt below optically
                // centres its 28px ring on the 20px first line.
                "scheduled-row w-full items-start pr-14" +
                (r.unread ? " is-unread" : "") +
                // SCH-ICON: settled / dormant rows step back a shade — title
                // included — so the rows that are still ticking are the ones the
                // eye lands on first.
                (isQuiet(r) ? " is-quiet" : "")
              }
              onClick={r.onClick}
              onKeyDown={(e) => {
                // Same keyboard affordance the sidebar rows carry: the menu is
                // reachable without a right mouse button.
                if (!((e.shiftKey && e.key === "F10") || e.key === "ContextMenu")) return;
                if (!hasActions) return;
                e.preventDefault();
                const rect = e.currentTarget.getBoundingClientRect();
                openMenu(rect.left + 20, rect.top + rect.height);
              }}
              // SC-13: the derived name is what the row SHOWS; the raw prompt is
              // one hover away, so nothing is hidden — only unshouted.
              title={[r.full, `${r.cadence}${r.when ? ` · ${r.when}` : ""}`, r.project].filter(Boolean).join("\n")}
            >
              {/* SCH-ICON: EVERY row carries a leading glyph. The settled rows
                  used to render an empty slot (SC-16, which was right to strip
                  their amber and wrong to leave nothing behind): the column kept
                  the icon's width, so a finished row read as one whose icon had
                  failed to load, and the page could not answer its own question —
                  which of these are still ticking? — without reading every
                  sub-line. The glyph is now the row's anchor, chosen from the
                  state the row ALREADY computes, so it cannot disagree with the
                  words next to it. Neutral gray throughout; the alert colour
                  stays scarce and keeps meaning exactly what SC-10 made it mean. */}
              <span
                className={"sched-glyph -mt-1" + (r.alert ? ` sched-warn is-${r.status.cls}` : "")}
                title={r.status.text}
              >
                {/* SC-19: 16px — the gold standard's ring is 13.5px of ink. */}
                {glyphFor(r)}
              </span>
              <span className="scheduled-copy flex min-w-0 flex-col gap-0.5">
                {/* SC-13 — the row shows the derived NAME (short, scannable), one
                    line, exactly as its own title= comment and scheduledTitle.ts
                    promise. Codex names rows in 2–4 words on a single line; the
                    raw prompt stays one hover away (title=) and fully searchable.
                    A brief detour rendered the un-truncated prompt across two
                    clamped lines ("use available mobile width"), which reproduced
                    the very paragraph-wall SC-13 was built to prevent — near-
                    identical rows could not be told apart at a glance. */}
                <b className="min-w-0 truncate leading-5 font-semibold">{r.title}</b>
                <span
                  className="sched-sub block min-w-0 truncate leading-4"
                  title={[r.cadence, r.alert || r.when].filter(Boolean).join(" · ")}
                >
                  <span className="sched-cadence">{r.cadence}</span>
                  {/* SC-4: two facts, as Codex has them — the rhythm and the
                      next tick. The project used to ride along as a third
                      segment, which made every sub-line a run-on and gave the
                      one live fact nothing to stand out from. It stays
                      searchable (r.meta), just not shouted.

                      SC-10: when the series is BROKEN, its state is the second
                      fact — it takes the next-run slot, because there is no next
                      run and saying nothing is what let a four-hours-dead
                      "Every 30m" driver pass for healthy. The last-ran stamp
                      keeps the emphasis off the tooltip. */}
                  {r.alert ? (
                    <>
                      {" · "}
                      <span className={`sched-warn is-${r.status.cls}`} title={r.when || undefined}>
                        {r.alert}
                      </span>
                    </>
                  ) : (
                    r.when && (
                      <>
                        {" · "}
                        <span className={r.isNext ? "sched-next" : undefined}>{r.when}</span>
                      </>
                    )
                  )}
                </span>
              </span>
              {/* SC-14: the project, named only when the query is what put this row
                  on screen. A quiet chip, not a third sub-line fact. */}
              {projectHit(r) && <span className="sched-project-chip">{r.project}</span>}
              {/* RS-3: the unread dot lives at the row's far right — the left column
                  is the status column, the right end is "there is something new".
                  Always rendered (empty when read) so the copy column keeps one
                  width and the titles never shift. A pinned row says so here too,
                  so Pin from the menu has a visible effect on this screen. */}
              <span className="sched-trail" aria-hidden="true">
                {isPinned && <PushPin className="sched-pinned" size={12} weight="fill" />}
                {r.unread && <span className="sched-unread" title="New activity" />}
              </span>
            </button>
            {/* SC-12 — the row's actions. The button is invisible at rest and
                appears on hover/focus, in a lane the row always reserves, so it
                can never nudge the title or the trail. */}
            {hasActions && <button
              className="sched-more absolute right-1 top-1/2 z-10 grid h-11 w-11 -translate-y-1/2 place-items-center rounded-lg border-0 bg-transparent hover:bg-panel-2"
              aria-label={`Actions for ${r.title}`}
              aria-haspopup="menu"
              title="Run actions"
              onClick={(e) => {
                e.stopPropagation();
                const rect = e.currentTarget.getBoundingClientRect();
                openMenu(rect.right - 8, rect.bottom + 4);
              }}
            >
              <DotsThree size={18} weight="bold" />
            </button>}
            </div>
            </Fragment>
            );
          })
        )}
        {suggestions}
      </div>

      {menuRow && ctx && (
        <ContextMenu x={ctx.x} y={ctx.y} onClose={() => setCtx(null)}>
          <MenuLabel>{menuRow.title}</MenuLabel>
          {/* A run row and a driver-session row are different objects: pin /
              rename / archive / unread are session-scoped store state, and a run
              has none of it. Offer each row only what actually acts on it —
              a menu of no-ops is worse than no menu. */}
          {menuRow.kind === "session" ? (
            <>
              {/* SC-17/INC-83 — the actions act on the SERIES itself, above the
                  housekeeping: Resume only for a genuinely broken series,
                  Retry while there is a live series to retry, and Cancel — the
                  series' own domain terminal, not a session lifecycle verb. */}
              {menuRow.recover && <MenuItem onClick={() => void act.resume(menuRow.id)}>Resume</MenuItem>}
              {!menuRow.settled && <MenuItem onClick={() => void act.retry(menuRow.id)}>Retry</MenuItem>}
              {!menuRow.settled && (
                <MenuItem
                  danger
                  title="no more iterations; the series records its cancelled terminal"
                  onClick={() => act.cancel(menuRow.id)}
                >
                  Cancel series…
                </MenuItem>
              )}
              <MenuLabel>Organize</MenuLabel>
              <MenuItem onClick={() => togglePin(menuRow.id)}>{pinned.includes(menuRow.id) ? "Unpin" : "Pin"}</MenuItem>
              <MenuItem onClick={() => openModal({ kind: "rename", sid: menuRow.id })}>Rename…</MenuItem>
              <MenuItem onClick={() => (unread.includes(menuRow.id) ? markRead(menuRow.id) : markUnread(menuRow.id))}>
                {unread.includes(menuRow.id) ? "Mark as read" : "Mark as unread"}
              </MenuItem>
              <MenuItem onClick={() => toggleArchive(menuRow.id)}>
                {archived.includes(menuRow.id) ? "Unarchive" : "Archive"}
              </MenuItem>
            </>
          ) : (
            <>
              {menuRow.running && <MenuItem onClick={() => void stopRun(menuRow.id)}>Stop</MenuItem>}
            </>
          )}
        </ContextMenu>
      )}
    </div>
  );
}
