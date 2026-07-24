import type { Icon } from "@phosphor-icons/react";
import {
  Bell,
  CalendarDots,
  Check,
  CheckCircle,
  Circle,
  DotsThree,
  FileMagnifyingGlass,
  MagnifyingGlass,
  Notebook,
  PauseCircle,
  PlayCircle,
  PushPin,
  WarningCircle,
} from "@phosphor-icons/react";
import { cadenceText, type CadenceSpec } from "../runPreset";
import { ContextMenu } from "./ContextMenu";
import { MenuItem, MenuLabel } from "./Menu";
import { Button } from "../ui/Button";
import { IconButton } from "../ui/IconButton";

export type ScheduledFilter = "all" | "active" | "paused";

export interface ScheduledSuggestion {
  icon: Icon;
  color: string;
  title: string;
  cadence: CadenceSpec;
  desc: string;
}

export const SUGGESTIONS: ScheduledSuggestion[] = [
  {
    icon: Bell,
    color: "#3b82f6",
    title: "Daily brief",
    cadence: { schedule: "cron", cron: "0 8 * * 1-5" },
    desc: "Start each weekday with a summary of your priorities",
  },
  {
    icon: Notebook,
    color: "#8b5cf6",
    title: "Weekly review",
    cadence: { schedule: "cron", cron: "0 16 * * 5" },
    desc: "Summarize the week's changes and open work",
  },
  {
    icon: FileMagnifyingGlass,
    color: "#22c55e",
    title: "Follow-up monitor",
    cadence: { schedule: "cron", cron: "0 */6 * * *" },
    desc: "Watch for failures and follow up",
  },
];

export interface ScheduledRunItemModel {
  key: string;
  id: string;
  kind: "session" | "run";
  title: string;
  full: string;
  cadence: string;
  when: string;
  isNext: boolean;
  alert: string;
  project: string;
  workspace: string;
  raw: string;
  meta: string;
  status: { text: string; cls: string };
  active: boolean;
  paused: boolean;
  scheduleControl: boolean;
  scheduleDetail: boolean;
  running: boolean;
  settled: boolean;
  recover: boolean;
  unread: boolean;
  sortTs: number;
  onClick: (opener?: HTMLElement) => void;
}

export function scheduledRunGlyph(
  row: Pick<ScheduledRunItemModel, "alert" | "running" | "settled" | "active">,
) {
  const size = 16;
  if (row.alert) return <WarningCircle size={size} weight="regular" />;
  if (row.running) return <PlayCircle size={size} weight="regular" />;
  if (row.settled) return <CheckCircle size={size} weight="regular" />;
  if (row.active) return <Circle size={size} weight="regular" />;
  return <PauseCircle size={size} weight="regular" />;
}

export interface ScheduledRunItemProps {
  row: ScheduledRunItemModel;
  pinned?: boolean;
  archived?: boolean;
  menuOpen?: boolean;
  showProjectHit?: boolean;
  onOpenMenu: (x: number, y: number) => void;
}

export function ScheduledRunItem({
  row,
  pinned = false,
  archived = false,
  menuOpen = false,
  showProjectHit = false,
  onOpenMenu,
}: ScheduledRunItemProps) {
  const hasActions = row.kind === "session" || row.running;
  const quiet = !row.recover && !row.active;
  return (
    <div
      className={
        "scheduled-row-wrap relative" +
        (row.unread ? " is-unread" : "") +
        (archived ? " is-archived" : "") +
        (menuOpen ? " menu-open" : "")
      }
      onContextMenu={(event) => {
        if (!hasActions) return;
        event.preventDefault();
        onOpenMenu(event.clientX, event.clientY);
      }}
    >
      <button
        className={
          "scheduled-row w-full items-start pr-14" +
          (row.unread ? " is-unread" : "") +
          (quiet ? " is-quiet" : "")
        }
        onClick={(event) => row.onClick(event.currentTarget)}
        onKeyDown={(event) => {
          if (
            !((event.shiftKey && event.key === "F10") ||
              event.key === "ContextMenu") ||
            !hasActions
          ) {
            return;
          }
          event.preventDefault();
          const rect = event.currentTarget.getBoundingClientRect();
          onOpenMenu(rect.left + 20, rect.top + rect.height);
        }}
        title={[
          row.full,
          `${row.cadence}${row.when ? ` · ${row.when}` : ""}`,
          row.project,
        ]
          .filter(Boolean)
          .join("\n")}
      >
        <span
          className={
            "sched-glyph -mt-1" +
            (row.alert ? ` sched-warn is-${row.status.cls}` : "")
          }
          title={row.status.text}
        >
          {scheduledRunGlyph(row)}
        </span>
        <span className="scheduled-copy flex min-w-0 flex-col gap-0.5">
          <b className="min-w-0 truncate leading-5 font-semibold">
            {row.title}
          </b>
          <span
            className="sched-sub block min-w-0 truncate leading-4"
            title={[row.cadence, row.alert || row.when]
              .filter(Boolean)
              .join(" · ")}
          >
            <span className="sched-cadence">{row.cadence}</span>
            {row.alert ? (
              <>
                {" · "}
                <span
                  className={`sched-warn is-${row.status.cls}`}
                  title={row.when || undefined}
                >
                  {row.alert}
                </span>
              </>
            ) : (
              row.when && (
                <>
                  {" · "}
                  <span className={row.isNext ? "sched-next" : undefined}>
                    {row.when}
                  </span>
                </>
              )
            )}
          </span>
        </span>
        {showProjectHit && (
          <span className="sched-project-chip">{row.project}</span>
        )}
        <span className="sched-trail" aria-hidden="true">
          {pinned && (
            <PushPin className="sched-pinned" size={12} weight="fill" />
          )}
          {row.unread && <span className="sched-unread" title="New activity" />}
        </span>
      </button>
      {hasActions && (
        <IconButton
          size="lg"
          variant="ghost"
          className="sched-more absolute right-1 top-1/2 z-10 grid h-11 w-11 -translate-y-1/2 place-items-center rounded-lg border-0 bg-transparent hover:bg-panel-2"
          aria-label={`Actions for ${row.title}`}
          aria-haspopup="menu"
          title="Run actions"
          onClick={(event) => {
            event.stopPropagation();
            const rect = event.currentTarget.getBoundingClientRect();
            onOpenMenu(rect.right - 8, rect.bottom + 4);
          }}
        >
          <DotsThree size={18} weight="bold" />
        </IconButton>
      )}
    </div>
  );
}

export interface ScheduledRunActionsProps {
  row: ScheduledRunItemModel;
  x: number;
  y: number;
  pinned: boolean;
  archived: boolean;
  unread: boolean;
  onClose: () => void;
  onResume: () => void;
  onRetry: () => void;
  onPause: () => void;
  onCancel: () => void;
  onTogglePin: () => void;
  onRename: () => void;
  onToggleRead: () => void;
  onToggleArchive: () => void;
  onStop: () => void;
}

export function ScheduledRunActions({
  row,
  x,
  y,
  pinned,
  archived,
  unread,
  onClose,
  onResume,
  onRetry,
  onPause,
  onCancel,
  onTogglePin,
  onRename,
  onToggleRead,
  onToggleArchive,
  onStop,
}: ScheduledRunActionsProps) {
  return (
    <ContextMenu x={x} y={y} onClose={onClose}>
      <MenuLabel>{row.title}</MenuLabel>
      {row.kind === "session" ? (
        <>
          {row.scheduleControl && row.paused ? (
            <MenuItem onClick={onResume}>Resume</MenuItem>
          ) : row.recover ? (
            <MenuItem onClick={onResume}>Resume</MenuItem>
          ) : row.scheduleControl && !row.settled ? (
            <MenuItem onClick={onPause}>Pause</MenuItem>
          ) : null}
          {!row.paused && !row.settled && (
            <MenuItem onClick={onRetry}>Retry</MenuItem>
          )}
          {!row.paused && !row.settled && (
            <MenuItem
              danger
              title="no more iterations; the series records its cancelled terminal"
              onClick={onCancel}
            >
              Cancel series…
            </MenuItem>
          )}
          <MenuLabel>Organize</MenuLabel>
          <MenuItem onClick={onTogglePin}>
            {pinned ? "Unpin" : "Pin"}
          </MenuItem>
          <MenuItem onClick={onRename}>Rename…</MenuItem>
          <MenuItem onClick={onToggleRead}>
            {unread ? "Mark as read" : "Mark as unread"}
          </MenuItem>
          <MenuItem onClick={onToggleArchive}>
            {archived ? "Unarchive" : "Archive"}
          </MenuItem>
        </>
      ) : (
        row.running && <MenuItem onClick={onStop}>Stop</MenuItem>
      )}
    </ContextMenu>
  );
}

export interface ScheduledToolbarProps {
  query: string;
  filter: ScheduledFilter;
  unreadCount: number;
  onQueryChange: (query: string) => void;
  onFilterChange: (filter: ScheduledFilter) => void;
  onMarkAllRead: () => void;
}

export function ScheduledToolbar({
  query,
  filter,
  unreadCount,
  onQueryChange,
  onFilterChange,
  onMarkAllRead,
}: ScheduledToolbarProps) {
  return (
    <div className="sched-toolbar">
      <div className="sched-search">
        <MagnifyingGlass size={15} />
        <input
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder="Search scheduled runs"
          aria-label="Search scheduled runs"
        />
      </div>
      <div className="sched-filters">
        <div
          className="sched-tabs"
          role="tablist"
          aria-label="Filter scheduled work"
        >
          {(["all", "active", "paused"] as ScheduledFilter[]).map((value) => (
            <button
              key={value}
              role="tab"
              aria-selected={filter === value}
              className={"sched-tab" + (filter === value ? " on" : "")}
              onClick={() => onFilterChange(value)}
            >
              {value[0].toUpperCase() + value.slice(1)}
            </button>
          ))}
        </div>
        {unreadCount > 0 && (
          <Button
            size="sm"
            variant="ghost"
            className="sched-markread"
            onClick={onMarkAllRead}
            title="Mark all scheduled activity as read"
          >
            <Check size={14} /> Mark all as read
          </Button>
        )}
      </div>
    </div>
  );
}

export function ScheduledSuggestionCard({
  suggestion,
  onSelect,
}: {
  suggestion: ScheduledSuggestion;
  onSelect: (
    suggestion: ScheduledSuggestion,
    opener: HTMLButtonElement,
  ) => void;
}) {
  const IconView = suggestion.icon;
  return (
    <button
      className="sched-suggest"
      onClick={(event) => onSelect(suggestion, event.currentTarget)}
    >
      <span className="sched-suggest-icon">
        <IconView size={22} color={suggestion.color} />
      </span>
      <span
        className="sched-suggest-body flex min-w-0 flex-1 flex-col gap-1"
        style={{ display: "flex", flexDirection: "column", gap: 4 }}
      >
        <span
          className="sched-suggest-head flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5"
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "baseline",
            columnGap: 8,
            rowGap: 2,
          }}
        >
          <b className="sched-suggest-title font-semibold">
            {suggestion.title}
          </b>
          <span className="sched-suggest-cadence">
            {cadenceText(suggestion.cadence)}
          </span>
        </span>
        <span className="sched-suggest-desc block" style={{ display: "block" }}>
          {suggestion.desc}
        </span>
      </span>
    </button>
  );
}

export function ScheduledSuggestions({
  suggestions = SUGGESTIONS,
  onSelect,
}: {
  suggestions?: ScheduledSuggestion[];
  onSelect: (
    suggestion: ScheduledSuggestion,
    opener: HTMLButtonElement,
  ) => void;
}) {
  return (
    <div className="sched-suggestions" data-testid="scheduled-suggestions">
      <div className="sched-suggestions-title">Suggestions</div>
      {suggestions.map((suggestion) => (
        <ScheduledSuggestionCard
          key={suggestion.title}
          suggestion={suggestion}
          onSelect={onSelect}
        />
      ))}
    </div>
  );
}

export type ScheduledEmptyStateKind = "empty" | "filter" | "search";

export function ScheduledEmptyState({
  kind,
  query = "",
  filter = "all",
}: {
  kind: ScheduledEmptyStateKind;
  query?: string;
  filter?: ScheduledFilter;
}) {
  const copy =
    kind === "empty"
      ? "Start a repeating run when work should keep running on its own."
      : kind === "search"
        ? `No results for "${query.trim()}".`
        : filter === "all"
          ? "No work matches this view."
          : `No ${filter} work matches this view.`;
  return (
    <div className="empty-state">
      <CalendarDots size={28} />
      <b>{kind === "empty" ? "No scheduled work" : "Nothing here"}</b>
      <span>{copy}</span>
    </div>
  );
}
