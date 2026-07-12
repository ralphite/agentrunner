import { useMemo, useState } from "react";
import type { Icon } from "@phosphor-icons/react";
import { CalendarDots, Plus, MagnifyingGlass, Check, CaretDown, Crosshair, ArrowsClockwise, Stack, Play, Bell, Notebook, FileMagnifyingGlass, Circle, PlayCircle, WarningCircle, DotsThree, PushPin } from "@phosphor-icons/react";
import "../styles.scheduled.css";
import { useStore } from "../store";
import { AR } from "../api";
import { friendlyStatus } from "./pill";
import { projectLabel, scheduleLabel, scheduledUnread } from "../viewModels";
import { scheduledTitle } from "../scheduledTitle";
import { relTime, sessionDate } from "../time";
import { copyText } from "../clipboard";
import { ContextMenu } from "./ContextMenu";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import type { Cadence } from "../types";

// We have no real paused flag (nothing suspends a driver), so the third tab is
// the honest "Finished" — the rows that have stopped ticking — not Codex's
// "Paused" word borrowed for a different fact. The word stays; what changed
// (SC-11) is the test behind it, which is now about the SERIES rather than
// about whatever happens to be executing this second. See seriesActive.
type Filter = "all" | "active" | "finished";

// Static template suggestions (Codex parity). Clicking one opens the existing
// create-task modal prefilled for a repeating task, with the description as the
// initial task text. Colours are fixed to match Codex's accent glyphs.
interface Suggestion {
  icon: Icon;
  color: string;
  title: string;
  cadence: string;
  desc: string;
}
const SUGGESTIONS: Suggestion[] = [
  {
    icon: Bell,
    color: "#3b82f6",
    title: "Daily brief",
    cadence: "Weekdays at 8:00 AM",
    desc: "Start each weekday with a summary of your priorities",
  },
  {
    icon: Notebook,
    color: "#8b5cf6",
    title: "Weekly review",
    cadence: "Fridays at 4:00 PM",
    desc: "Summarize the week's changes and open work",
  },
  {
    icon: FileMagnifyingGlass,
    color: "#22c55e",
    title: "Follow-up monitor",
    cadence: "Every 6 hours",
    desc: "Watch for failures and follow up",
  },
];

// SC-1 — what belongs on this page. A scheduled thing has a RHYTHM: left alone,
// it fires again. That is the whole reason the screen exists, and it is exactly
// what the schedule kind tells us (webui/schedule.go):
//
//   interval / cron   → a rhythm ("Every 30m", "Saturdays at 4:00 AM")   ✅
//   self_paced        → a driver that re-arms its own next iteration      ✅
//   immediate         → a one-shot task / a goal that runs until verified ❌
//   parallel          → Best of N: attempts side by side, not a rhythm    ❌
//   (absent)          → a plain `submit` run: one-shot by construction    ❌
//
// Before this rule the page collected EVERY run and every driver session — 28
// rows, 26 of them "Runs once" / "Best of 3" — which buried the single genuinely
// scheduled task and pushed Suggestions off the first screen. The excluded work
// is not lost: one-shot runs stay reachable from ⌘K and their session lands in
// the sidebar like any other task.
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
const SETTLED_STATUS = new Set(["done", "closed", "stopped"]);

// SC-10 — a BROKEN series must not look like a healthy one. `crash` ("Failed")
// and `stranded` ("Needs recovery") used to render as the same gray Circle as an
// idle-between-ticks row, with the status text hidden in a `title=` tooltip: a
// driver that advertised "Every 30m" but had been dead for four hours was
// pixel-identical to one about to fire. This hub exists to answer "are my
// background tasks still alive?", so a dead one has to say so on screen.
const ALERT_STATUS = new Set(["crash", "stranded"]);

// SC-11 — "Active" is a fact about the SERIES, not about this instant. Judging
// it by "an iteration is executing right now" (cls run/appr) made the tab
// structurally empty: a healthy `Every 30m` task is idle between ticks by
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
  title: string; // SC-13: the derived NAME (short, scannable); never the whole prompt
  full: string; // the raw label/prompt — the tooltip, and what search reads
  cadence: string; // the rhythm: "Every 30m" / "Saturdays at 4:00 AM" / "Self-paced"
  when: string; // "Next run in 12m" when known, else the honest "Ran 1d ago"
  isNext: boolean; // when names a FUTURE tick (styled as the live fact it is)
  alert: string; // SC-10: "Failed" / "Needs recovery" — shown, not tooltipped
  project: string;
  workspace: string;
  meta: string; // the row's facts flattened (project included), for search
  status: { text: string; cls: string };
  active: boolean; // the series still has ticks coming / needs you (SC-11)
  running: boolean; // an iteration is executing right now — the only stoppable state
  unread: boolean; // driver row with new activity you haven't opened (F2)
  sortTs: number;
  onClick: () => void;
}

// whenAgo turns a relative stamp into a sub-line phrase without the awkward
// "just now ago".
function whenAgo(when: Date | null): string {
  const rel = relTime(when);
  if (!rel) return "";
  return rel === "just now" ? "just now" : `${rel} ago`;
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

// Scheduled is Codex's Scheduled tasks hub: repeating work that keeps running on
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
    toast,
  } = useStore();
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  // SC-12 — the cursor-anchored row menu (same component the sidebar rows use).
  const [ctx, setCtx] = useState<{ x: number; y: number; key: string } | null>(null);

  const unreadIds = useMemo(() => scheduledUnread(sessions, unread), [sessions, unread]);

  const rows = useMemo<SchedRow[]>(() => {
    const flagged = new Set(unread);
    const out: SchedRow[] = [];
    // row assembles the sub-line: cadence first, then the next run (or, absent
    // one, when it last ran), then the project. It also decides the two facts
    // the row is judged by: whether the SERIES is still live (SC-11) and whether
    // it is broken and needs saying so (SC-10).
    const row = (
      base: Omit<SchedRow, "when" | "isNext" | "meta" | "active" | "alert" | "title" | "running">,
      nextRunAt: string | undefined,
      lastRan: Date | null,
    ): SchedRow => {
      const next = nextRunPhrase(nextRunAt);
      const ago = whenAgo(lastRan);
      const when = next || (ago ? `Ran ${ago}` : "");
      const alert = ALERT_STATUS.has(base.status.cls) ? base.status.text : "";
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
        active: seriesActive(base.status.cls, !!next),
        running: base.status.cls === "run",
        // The alert phrase is searchable too — "needs recovery" should find the
        // rows that do. So is the full prompt (`full`), matched at the call site.
        meta: [base.cadence, alert, when, base.project].filter(Boolean).join(" · "),
      };
    };
    for (const run of runs) {
      if (!hasRhythm(run)) continue; // SC-1: one-shot / best-of-N is not scheduled work
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

  return (
    <div className="scheduled-page">
      <div className="page-heading">
        <div>
          <h2>Scheduled tasks</h2>
          <p>Ask AgentRunner to schedule tasks, set goals, or monitor for updates</p>
        </div>
        <div className="scheduled-create">
          <Menu
            ariaLabel="Create scheduled work"
            triggerClassName="page-action"
            label={<><Plus size={15} /> Create <CaretDown size={13} /></>}
          >
            <MenuLabel>Create</MenuLabel>
            <MenuItem onClick={() => openModal({ kind: "run", preset: "one-time" })}>
              <Play size={15} /><span><b>One-time task</b><small>Run once in the background</small></span>
            </MenuItem>
            <MenuItem onClick={() => openModal({ kind: "run", preset: "goal" })}>
              <Crosshair size={15} /><span><b>Goal</b><small>Keep working until verified</small></span>
            </MenuItem>
            <MenuItem onClick={() => openModal({ kind: "run", preset: "repeating" })}>
              <ArrowsClockwise size={15} /><span><b>Repeating</b><small>Run on an interval or cron schedule</small></span>
            </MenuItem>
            <MenuItem onClick={() => openModal({ kind: "run", preset: "best-of-n" })}>
              <Stack size={15} /><span><b>Best of N</b><small>Run isolated attempts and select the best</small></span>
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
              placeholder="Search scheduled tasks"
              aria-label="Search scheduled tasks"
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
            <span>Start a repeating task when work should keep running on its own.</span>
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
            const openMenu = (x: number, y: number) => setCtx({ x, y, key: r.key });
            return (
            <div
              className={
                "scheduled-row-wrap" +
                (r.unread ? " is-unread" : "") +
                (isArchived ? " is-archived" : "") +
                (ctx?.key === r.key ? " menu-open" : "")
              }
              key={r.key}
              onContextMenu={(e) => {
                e.preventDefault();
                openMenu(e.clientX, e.clientY);
              }}
            >
            <button
              className={"scheduled-row" + (r.unread ? " is-unread" : "")}
              onClick={r.onClick}
              onKeyDown={(e) => {
                // Same keyboard affordance the sidebar rows carry: the menu is
                // reachable without a right mouse button.
                if (!((e.shiftKey && e.key === "F10") || e.key === "ContextMenu")) return;
                e.preventDefault();
                const rect = e.currentTarget.getBoundingClientRect();
                openMenu(rect.left + 20, rect.top + rect.height);
              }}
              // SC-13: the derived name is what the row SHOWS; the raw prompt is
              // one hover away, so nothing is hidden — only unshouted.
              title={[r.full, `${r.cadence}${r.when ? ` · ${r.when}` : ""}`, r.project].filter(Boolean).join("\n")}
            >
              {SETTLED_STATUS.has(r.status.cls) ? (
                <span className="sched-blank" aria-hidden="true" />
              ) : (
                <span
                  className={"sched-glyph" + (r.alert ? ` sched-warn is-${r.status.cls}` : "")}
                  title={r.status.text}
                >
                  {/* SC-10: a broken series gets its own glyph. Failed / needs
                      recovery must not share the healthy row's gray ring. */}
                  {r.alert ? (
                    <WarningCircle size={20} weight="regular" />
                  ) : r.status.cls === "run" ? (
                    <PlayCircle size={20} weight="regular" />
                  ) : (
                    <Circle size={20} weight="regular" />
                  )}
                </span>
              )}
              <span className="scheduled-copy">
                <b>{r.title}</b>
                <span className="sched-sub">
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
            <button
              className="sched-more"
              aria-label={`Actions for ${r.title}`}
              aria-haspopup="menu"
              title="Task actions"
              onClick={(e) => {
                e.stopPropagation();
                const rect = e.currentTarget.getBoundingClientRect();
                openMenu(rect.right - 8, rect.bottom + 4);
              }}
            >
              <DotsThree size={18} weight="bold" />
            </button>
            </div>
            );
          })
        )}
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
              <MenuItem onClick={() => togglePin(menuRow.id)}>{pinned.includes(menuRow.id) ? "Unpin" : "Pin"}</MenuItem>
              <MenuItem onClick={() => openModal({ kind: "rename", sid: menuRow.id })}>Rename…</MenuItem>
              <MenuItem onClick={() => (unread.includes(menuRow.id) ? markRead(menuRow.id) : markUnread(menuRow.id))}>
                {unread.includes(menuRow.id) ? "Mark as read" : "Mark as unread"}
              </MenuItem>
              <MenuItem onClick={() => toggleArchive(menuRow.id)}>
                {archived.includes(menuRow.id) ? "Unarchive" : "Archive"}
              </MenuItem>
              <MenuLabel>Copy</MenuLabel>
              <MenuItem onClick={() => { copyText(menuRow.id); toast("copied session id", "info"); }}>Session ID</MenuItem>
              <MenuItem onClick={() => { copyText(`${location.origin}/#${menuRow.id}`); toast("copied link", "info"); }}>Task link</MenuItem>
            </>
          ) : (
            <>
              {menuRow.running && <MenuItem onClick={() => void stopRun(menuRow.id)}>Stop</MenuItem>}
              <MenuLabel>Copy</MenuLabel>
              <MenuItem onClick={() => { copyText(menuRow.id); toast("copied run id", "info"); }}>Run ID</MenuItem>
              <MenuItem onClick={() => { copyText(`${location.origin}/#run:${menuRow.id}`); toast("copied link", "info"); }}>Run link</MenuItem>
            </>
          )}
        </ContextMenu>
      )}

      <div className="sched-suggestions">
        <div className="sched-suggestions-title">Suggestions</div>
        {SUGGESTIONS.map((s) => {
          const Ic = s.icon;
          return (
            <button
              key={s.title}
              className="sched-suggest"
              onClick={() => openModal({ kind: "run", preset: "repeating", task: s.desc })}
            >
              <span className="sched-suggest-icon">
                <Ic size={22} color={s.color} />
              </span>
              <span className="sched-suggest-body">
                <span className="sched-suggest-head">
                  <b className="sched-suggest-title">{s.title}</b>
                  <span className="sched-suggest-cadence">{s.cadence}</span>
                </span>
                <span className="sched-suggest-desc">{s.desc}</span>
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
