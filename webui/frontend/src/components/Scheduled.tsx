import { useMemo, useState } from "react";
import { CalendarDots, Plus, ArrowUpRight, MagnifyingGlass, Check } from "@phosphor-icons/react";
import { useStore } from "../store";
import { friendlyStatus } from "./pill";
import { projectLabel, scheduleLabel, scheduledUnread } from "../viewModels";
import { relTime, sessionDate } from "../time";

type Filter = "all" | "active" | "completed";

// Settled/terminal rows carry no useful colour on their leading dot — it reads
// as gray noise on every completed row (review sw-d-11). Drop the dot for these
// (a blank keeps the columns aligned); attention/running/unread still badge.
// Same semantics as the sidebar's W10 rule.
const SETTLED_STATUS = new Set(["done", "closed", "stopped"]);

interface SchedRow {
  key: string;
  title: string;
  meta: string; // sub-line: type · project · when
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

// Scheduled is Codex's Scheduled tasks hub: goals and repeating work that keep
// running on their own. We have no cron/next-run contract, so each row shows
// the honest facts we do hold — the schedule type, the project, and when it
// last started — plus a search box and All / Active / Completed filters mapped
// to our real live-vs-finished states (INC-41 W7).
export function Scheduled() {
  const { runs, sessions, select, selectRun, openModal, unread, markRead } = useStore();
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");

  const unreadIds = useMemo(() => scheduledUnread(sessions, unread), [sessions, unread]);

  const rows = useMemo<SchedRow[]>(() => {
    const isActive = (cls: string) => cls === "run" || cls === "appr";
    const flagged = new Set(unread);
    const out: SchedRow[] = [];
    for (const run of runs) {
      const status = friendlyStatus(run.status);
      const ts = Date.parse(run.startedAt);
      const started = isNaN(ts) ? null : new Date(ts);
      out.push({
        key: "run:" + run.id,
        title: run.label || run.id,
        meta: [run.kind === "submit" ? "One-time" : "Drive", projectLabel(run.workspace), whenAgo(started)]
          .filter(Boolean)
          .join(" · "),
        status,
        active: isActive(status.cls),
        unread: false,
        sortTs: isNaN(ts) ? 0 : ts,
        onClick: () => selectRun(run.id),
      });
    }
    for (const s of sessions) {
      if (s.kind !== "driver") continue;
      const status = friendlyStatus(s.status);
      const d = sessionDate(s.id);
      out.push({
        key: s.id,
        title: s.title || s.id,
        meta: [scheduleLabel(s.schedule), projectLabel(s.workspace), whenAgo(d)].filter(Boolean).join(" · "),
        status,
        active: isActive(status.cls),
        unread: flagged.has(s.id),
        sortTs: d ? d.getTime() : 0,
        onClick: () => select(s.id),
      });
    }
    // Newest-first; the coloured status dot and label carry the state.
    out.sort((a, b) => b.sortTs - a.sortTs);
    return out;
  }, [runs, sessions, select, selectRun, unread]);

  const counts = {
    all: rows.length,
    active: rows.filter((r) => r.active).length,
    completed: rows.filter((r) => !r.active).length,
  };
  const ql = query.trim().toLowerCase();
  const filtered = rows.filter((r) => {
    if (filter === "active" && !r.active) return false;
    if (filter === "completed" && r.active) return false;
    if (ql && !(r.title.toLowerCase().includes(ql) || r.meta.toLowerCase().includes(ql))) return false;
    return true;
  });
  const totalEmpty = rows.length === 0;

  return (
    <div className="scheduled-page">
      <div className="page-heading">
        <div>
          <span className="page-eyebrow"><CalendarDots size={16} /> Scheduled</span>
          <h2>Scheduled work</h2>
          <p>Goals and repeating tasks stay visible here, including after a restart.</p>
        </div>
        <button className="primary page-action" onClick={() => openModal({ kind: "run" })}>
          <Plus size={15} /> New schedule
        </button>
      </div>

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
          <div className="sched-tabs" role="tablist" aria-label="Filter scheduled work">
            {(["all", "active", "completed"] as Filter[]).map((f) => (
              <button
                key={f}
                role="tab"
                aria-selected={filter === f}
                className={"sched-tab" + (filter === f ? " on" : "")}
                onClick={() => setFilter(f)}
              >
                {f[0].toUpperCase() + f.slice(1)}
                <span className="sched-tab-count">{counts[f]}</span>
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
              <span className="sched-lead" aria-hidden="true">
                {r.unread && <span className="sched-unread" title="New activity" />}
              </span>
              {SETTLED_STATUS.has(r.status.cls) ? (
                <span className="sched-blank" aria-hidden="true" />
              ) : (
                <span className={"status-dot " + r.status.cls} title={r.status.text} />
              )}
              <span className="scheduled-copy">
                <b>{r.title}</b>
                <span>{r.meta}</span>
              </span>
              <span className={"scheduled-status " + r.status.cls}>{r.status.text}</span>
              <ArrowUpRight size={15} />
            </button>
          ))
        )}
      </div>
    </div>
  );
}
