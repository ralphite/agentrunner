import { ArrowLeft, CaretDown, Crosshair, ArrowsClockwise, Stack, Play, X } from "@phosphor-icons/react";
import { friendlyStatus } from "./pill";
import { projectLabel, scheduleLabel } from "../viewModels";
import { Menu, MenuItem, MenuLabel } from "./Menu";
import type { ScheduleDetail } from "../types";
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
} from "./ScheduledParts";
import {
  useScheduleEditController,
  useScheduledController,
  type ScheduledController,
} from "../features/scheduled/useScheduledController";

export { SUGGESTIONS } from "./ScheduledParts";
export {
  hasRhythm,
  isLimitStatus,
} from "../features/scheduled/useScheduledController";

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
  const controller = useScheduleEditController(detail, onSaved);

  return (
    <Modal
      title="Edit schedule"
      onClose={onClose}
      footer={
        <>
          <Button
            variant="outline"
            disabled={controller.busy}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            variant="solid"
            loading={controller.busy}
            disabled={controller.blocked}
            onClick={controller.save}
          >
            Save
          </Button>
        </>
      }
    >
      <label className="field" htmlFor="schedule-edit-prompt">Prompt</label>
      <Textarea
        id="schedule-edit-prompt"
        rows={4}
        value={controller.prompt}
        onChange={(event) => controller.setPrompt(event.target.value)}
      />
      <label className="field" htmlFor="schedule-edit-repeat">Repeat</label>
      <div className="row-flex">
        <Select
          id="schedule-edit-repeat"
          value={controller.schedule}
          onChange={(event) =>
            controller.setSchedule(
              event.target.value as "interval" | "cron",
            )}
        >
          <option value="interval">Every interval</option>
          <option value="cron">Cron schedule</option>
        </Select>
        <Input
          aria-label={
            controller.schedule === "interval"
              ? "Interval"
              : "Cron expression"
          }
          value={controller.cadenceValue}
          onChange={(event) =>
            controller.setCadenceValue(event.target.value)}
          placeholder={
            controller.schedule === "interval"
              ? "30m · 1h"
              : "0 8 * * 1-5"
          }
        />
      </div>
      {controller.cadenceError && (
        <div className="text-[12px] text-red" role="alert">
          {controller.cadenceError}
        </div>
      )}
      <label className="field" htmlFor="schedule-edit-overlap">If a run is still active</label>
      <Select
        id="schedule-edit-overlap"
        value={controller.overlap}
        onChange={(event) =>
          controller.setOverlap(
            event.target.value as "skip" | "coalesce",
          )}
      >
        <option value="skip">Skip missed runs</option>
        <option value="coalesce">Run once when available</option>
      </Select>
      {controller.error && (
        <div
          className="rounded-lg border border-line bg-bg p-3 text-[12px] text-red"
          role="alert"
        >
          {controller.error}
        </div>
      )}
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
  const controller = useScheduledController();
  return <ScheduledView controller={controller} />;
}

function ScheduledView({ controller }: { controller: ScheduledController }) {
  return (
    <div
      className={
        "scheduled-shell" + (controller.detail.sid ? " has-detail" : "")
      }
    >
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
            <MenuItem onClick={() => controller.create("one-time")}>
              <Play size={15} /><span className="scheduled-create-option flex min-w-0 flex-1 flex-col gap-0.5"><b>One-time run</b><small>Run once in the background</small></span>
            </MenuItem>
            <MenuItem onClick={() => controller.create("goal")}>
              <Crosshair size={15} /><span className="scheduled-create-option flex min-w-0 flex-1 flex-col gap-0.5"><b>Goal</b><small>Keep working until verified</small></span>
            </MenuItem>
            <MenuItem onClick={() => controller.create("repeating")}>
              <ArrowsClockwise size={15} /><span className="scheduled-create-option flex min-w-0 flex-1 flex-col gap-0.5"><b>Repeating</b><small>Run on an interval or cron schedule</small></span>
            </MenuItem>
            <MenuItem onClick={() => controller.create("best-of-n")}>
              <Stack size={15} /><span className="scheduled-create-option flex min-w-0 flex-1 flex-col gap-0.5"><b>Best of N</b><small>Run isolated attempts and select the best</small></span>
            </MenuItem>
          </Menu>
        </div>
      </div>

      {/* RS-3: two rows, as Codex has them — the search field owns a full row, and
          the filters sit on their own line below it (tabs left, Mark all as read
          right). The right-aligned button can appear and disappear with the unread
          set without ever nudging the tabs. */}
      {!controller.totalEmpty && (
        <ScheduledToolbar
          query={controller.query}
          filter={controller.filter}
          unreadCount={controller.unreadVisibleCount}
          onQueryChange={controller.setQuery}
          onFilterChange={controller.setFilter}
          onMarkAllRead={controller.markAllVisibleRead}
        />
      )}

      <div className="scheduled-list">
        {controller.totalEmpty ? (
          <ScheduledEmptyState kind="empty" />
        ) : controller.filteredCount === 0 ? (
          <ScheduledEmptyState
            kind={controller.searching ? "search" : "filter"}
            query={controller.query}
            filter={controller.filter}
          />
        ) : (
          controller.visibleRows.map((row) => {
            return (
              <ScheduledRunItem
                key={row.key}
                row={row}
                pinned={controller.isPinned(row.id)}
                archived={controller.isArchived(row.id)}
                menuOpen={controller.contextMenu?.key === row.key}
                showProjectHit={controller.projectMatched(row)}
                onOpenMenu={(x, y) =>
                  controller.openContextMenu({ x, y, key: row.key })}
              />
            );
          })
        )}
        {!controller.searching &&
          controller.filteredCount > INITIAL_VISIBLE_ROWS && (
          <div className="sched-disclosure">
            {controller.visibleRows.length < controller.filteredCount && (
              <button
                className="show-more"
                onClick={controller.showMore}
              >
                Show{" "}
                {Math.min(
                  ROWS_PER_PAGE,
                  controller.filteredCount - controller.visibleRows.length,
                )}{" "}
                more ·{" "}
                {controller.filteredCount - controller.visibleRows.length}{" "}
                remaining
              </button>
            )}
            {controller.visibleCount > INITIAL_VISIBLE_ROWS && (
              <button
                className="show-more"
                onClick={controller.showFewer}
              >
                Show fewer · newest {INITIAL_VISIBLE_ROWS}
              </button>
            )}
          </div>
        )}
        <ScheduledSuggestions onSelect={controller.selectSuggestion} />
      </div>

      {controller.menuRow && controller.contextMenu && (
        <ScheduledRunActions
          row={controller.menuRow}
          x={controller.contextMenu.x}
          y={controller.contextMenu.y}
          pinned={controller.isPinned(controller.menuRow.id)}
          archived={controller.isArchived(controller.menuRow.id)}
          unread={controller.isUnread(controller.menuRow.id)}
          onClose={controller.closeContextMenu}
          onResume={controller.resumeMenuRow}
          onRetry={controller.retryMenuRow}
          onPause={controller.pauseMenuRow}
          onCancel={controller.cancelMenuRow}
          onTogglePin={controller.toggleMenuRowPin}
          onRename={controller.renameMenuRow}
          onToggleRead={controller.toggleMenuRowRead}
          onToggleArchive={controller.toggleMenuRowArchive}
          onStop={controller.stopMenuRow}
        />
      )}
      </main>
      {controller.detail.sid && (
        <ScheduleDetailPanel
          title={controller.detail.title}
          detail={controller.detail.value}
          loading={controller.detail.loading}
          error={controller.detail.error}
          acting={controller.detail.acting}
          onClose={controller.detail.close}
          onRetry={controller.detail.retry}
          onHistory={controller.detail.openHistory}
          onCadence={controller.detail.cadence}
          onEdit={controller.detail.beginEdit}
        />
      )}
      {controller.detail.sid &&
        controller.detail.value &&
        controller.detail.editing && (
        <ScheduleEditDialog
          detail={controller.detail.value}
          onClose={controller.detail.closeEdit}
          onSaved={controller.detail.saved}
        />
      )}
    </div>
  );
}
