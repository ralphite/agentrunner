import { useMemo, useState } from "react";
import type { Icon } from "@phosphor-icons/react";
import { CalendarDots, Plus, MagnifyingGlass, Check, CaretDown, Crosshair, ArrowsClockwise, Stack, Play, Bell, Notebook, FileMagnifyingGlass, Circle, PlayCircle } from "@phosphor-icons/react";
import "../styles.scheduled.css";
import { useStore } from "../store";
import { friendlyStatus } from "./pill";
import { projectLabel, scheduleLabel, scheduledUnread } from "../viewModels";
import { relTime, sessionDate } from "../time";
import { Menu, MenuItem, MenuLabel } from "./Menu";

type Filter = "all" | "active" | "paused";

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

// Settled/terminal rows carry no useful colour on their leading dot — it reads
// as gray noise on every completed row (review sw-d-11). Drop the dot for these
// (a blank keeps the columns aligned); attention/running/unread still badge.
// Same semantics as the sidebar's W10 rule.
const SETTLED_STATUS = new Set(["done", "closed", "stopped"]);

interface SchedRow {
  key: string;
  title: string;
  cadence: string; // the rhythm: "Every 30m" / "Saturdays at 4:00 AM" / "Runs once"
  when: string; // "Next run in 12m" when known, else the honest "Ran 1d ago"
  isNext: boolean; // when names a FUTURE tick (styled as the live fact it is)
  project: string;
  meta: string; // the sub-line flattened, for search
  status: { text: string; cls: string };
  active: boolean; // live (running / waiting on you) vs finished
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

// Scheduled is Codex's Scheduled tasks hub: goals and repeating work that keep
// running on their own. The two facts that justify a scheduled thing lead every
// row — its CADENCE and its NEXT RUN (CX-3), both derived server-side from the
// driver spec (schedule/interval/cron/n) and served on /api/runs and
// /api/sessions. When there is no future tick to name (a one-shot goal, a
// finished series, a spec we could not read) the row falls back to the honest
// "Ran 1d ago" — never a fabricated time. Project and status stay, demoted.
// Search + All / Active / Paused filters map to our live-vs-finished states
// (INC-41 W7).
export function Scheduled() {
  const { runs, sessions, select, selectRun, openModal, unread, markRead } = useStore();
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");

  const unreadIds = useMemo(() => scheduledUnread(sessions, unread), [sessions, unread]);

  const rows = useMemo<SchedRow[]>(() => {
    const isActive = (cls: string) => cls === "run" || cls === "appr";
    const flagged = new Set(unread);
    const out: SchedRow[] = [];
    // row assembles the sub-line: cadence first, then the next run (or, absent
    // one, when it last ran), then the project.
    const row = (
      base: Omit<SchedRow, "when" | "isNext" | "meta">,
      nextRunAt: string | undefined,
      lastRan: Date | null,
    ): SchedRow => {
      const next = nextRunPhrase(nextRunAt);
      const ago = whenAgo(lastRan);
      const when = next || (ago ? `Ran ${ago}` : "");
      return {
        ...base,
        when,
        isNext: !!next,
        meta: [base.cadence, when, base.project].filter(Boolean).join(" · "),
      };
    };
    for (const run of runs) {
      const status = friendlyStatus(run.status);
      const ts = Date.parse(run.startedAt);
      const started = isNaN(ts) ? null : new Date(ts);
      out.push(
        row(
          {
            key: "run:" + run.id,
            title: run.label || run.id,
            // A submit run is a one-shot by construction; a drive run's cadence
            // comes from its spec (absent only if the spec was unreadable).
            cadence: run.cadence || (run.kind === "submit" ? "Runs once" : scheduleLabel(run.schedule)),
            project: projectLabel(run.workspace),
            status,
            active: isActive(status.cls),
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
      if (s.kind !== "driver") continue;
      const status = friendlyStatus(s.status);
      const d = sessionDate(s.id);
      out.push(
        row(
          {
            key: s.id,
            title: s.title || s.id,
            // cadence is the spec's real rhythm; scheduleLabel is the coarse
            // kind we fall back to when the journal could not be read.
            cadence: s.cadence || scheduleLabel(s.schedule),
            project: projectLabel(s.workspace),
            status,
            active: isActive(status.cls),
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
  }, [runs, sessions, select, selectRun, unread]);

  // We have no real paused flag, so "Paused" == the non-active (finished) rows,
  // relabelled to match Codex's All / Active / Paused pills. Codex shows no
  // numeric counts on the pills (N-parity), so we only compute the filter.
  const ql = query.trim().toLowerCase();
  const filtered = rows.filter((r) => {
    if (filter === "active" && !r.active) return false;
    if (filter === "paused" && r.active) return false;
    if (ql && !(r.title.toLowerCase().includes(ql) || r.meta.toLowerCase().includes(ql))) return false;
    return true;
  });
  const totalEmpty = rows.length === 0;

  return (
    <div className="scheduled-page">
      <div className="page-heading">
        <div>
          <h2>Scheduled tasks</h2>
          <p>Ask AgentRunner to schedule tasks, set goals, or monitor for updates.</p>
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
              placeholder="Search scheduled work…"
              aria-label="Search scheduled work"
            />
          </div>
          <div className="sched-filters">
            <div className="sched-tabs" role="tablist" aria-label="Filter scheduled work">
              {(["all", "active", "paused"] as Filter[]).map((f) => (
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
            <span>Start a goal or repeating task when work should continue on its own.</span>
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
          filtered.map((r) => (
            <button className={"scheduled-row" + (r.unread ? " is-unread" : "")} key={r.key} onClick={r.onClick}>
              {SETTLED_STATUS.has(r.status.cls) ? (
                <span className="sched-blank" aria-hidden="true" />
              ) : (
                <span className="sched-glyph" title={r.status.text}>
                  {r.status.cls === "run" ? (
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
                  {r.when && (
                    <>
                      {" · "}
                      <span className={r.isNext ? "sched-next" : undefined}>{r.when}</span>
                    </>
                  )}
                  {r.project && <>{" · "}{r.project}</>}
                </span>
              </span>
              {/* RS-3: the unread dot lives at the row's far right — the left column
                  is the status column, the right end is "there is something new".
                  Always rendered (empty when read) so the copy column keeps one
                  width and the titles never shift. */}
              <span className="sched-trail" aria-hidden="true">
                {r.unread && <span className="sched-unread" title="New activity" />}
              </span>
            </button>
          ))
        )}
      </div>

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
