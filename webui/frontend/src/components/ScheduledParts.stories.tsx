import { useState, type ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
  ScheduledEmptyState,
  ScheduledRunActions,
  ScheduledRunItem,
  ScheduledSuggestions,
  ScheduledToolbar,
  SUGGESTIONS,
  type ScheduledFilter,
  type ScheduledRunItemModel,
} from "./ScheduledParts";

const noOp = () => {};

function row(
  overrides: Partial<ScheduledRunItemModel> = {},
): ScheduledRunItemModel {
  return {
    key: "schedule-story",
    id: "schedule-story",
    kind: "session",
    title: "Daily component review",
    full: "Review every component state and interaction",
    cadence: "Weekdays at 8:00 AM",
    when: "Next run in 12h",
    isNext: true,
    alert: "",
    project: "agentrunner",
    workspace: "/projects/agentrunner",
    raw: "waiting:input",
    meta: "Weekdays at 8:00 AM · agentrunner",
    status: { text: "Ready", cls: "idle" },
    active: true,
    paused: false,
    scheduleControl: true,
    scheduleDetail: true,
    running: false,
    settled: false,
    recover: false,
    unread: false,
    sortTs: Date.parse("2026-07-23T12:00:00Z"),
    onClick: noOp,
    ...overrides,
  };
}

function Matrix({
  children,
}: {
  children: ReactNode;
}) {
  return (
    <div className="mx-auto grid w-full max-w-[720px] gap-3 p-6">
      {children}
    </div>
  );
}

const meta = {
  title: "Components/Scheduled/Parts",
  component: ScheduledRunItem,
  parameters: { layout: "fullscreen" },
  args: {
    row: row(),
    onOpenMenu: fn(),
  },
} satisfies Meta<typeof ScheduledRunItem>;

export default meta;
type Story = StoryObj<typeof meta>;

export const RunItemLifecycleMatrix: Story = {
  render: () => (
    <Matrix>
      <ScheduledRunItem row={row()} onOpenMenu={noOp} />
      <ScheduledRunItem
        row={row({
          id: "running",
          key: "running",
          title: "Running browser review",
          running: true,
          raw: "running",
          status: { text: "Running", cls: "run" },
          when: "Next run due now",
        })}
        onOpenMenu={noOp}
      />
      <ScheduledRunItem
        row={row({
          id: "approval",
          key: "approval",
          title: "Approval required",
          raw: "waiting:approval",
          status: { text: "Needs approval", cls: "appr" },
          when: "Waiting for approval",
        })}
        onOpenMenu={noOp}
      />
      <ScheduledRunItem
        row={row({
          id: "paused",
          key: "paused",
          title: "Paused weekly review",
          active: false,
          paused: true,
          raw: "paused",
          status: { text: "Paused", cls: "idle" },
          when: "Paused",
          isNext: false,
        })}
        onOpenMenu={noOp}
      />
      <ScheduledRunItem
        row={row({
          id: "completed",
          key: "completed",
          title: "Configured run completed",
          active: false,
          settled: true,
          raw: "max_iterations",
          status: { text: "Iteration limit reached", cls: "stranded" },
          when: "Ran 2h ago",
          isNext: false,
        })}
        onOpenMenu={noOp}
      />
      <ScheduledRunItem
        row={row({
          id: "recovery",
          key: "recovery",
          title: "Needs recovery",
          active: true,
          recover: true,
          alert: "Needs recovery",
          raw: "stranded",
          status: { text: "Needs recovery", cls: "stranded" },
          when: "Ran 4h ago",
          isNext: false,
        })}
        onOpenMenu={noOp}
      />
      <ScheduledRunItem
        row={row({
          id: "failed",
          key: "failed",
          title: "Failed scheduled run",
          active: true,
          recover: true,
          alert: "Failed",
          raw: "crash",
          status: { text: "Failed", cls: "crash" },
          when: "Ran 8m ago",
          isNext: false,
        })}
        onOpenMenu={noOp}
      />
    </Matrix>
  ),
};

export const RunItemOrganizationStates: Story = {
  render: () => (
    <Matrix>
      <ScheduledRunItem
        row={row({ unread: true })}
        pinned
        onOpenMenu={noOp}
      />
      <ScheduledRunItem
        row={row({
          id: "archived",
          key: "archived",
          title:
            "An intentionally long archived scheduled run title that must remain scannable",
        })}
        archived
        onOpenMenu={noOp}
      />
      <ScheduledRunItem
        row={row({
          id: "project-hit",
          key: "project-hit",
          project: "agentrunner-worktree",
        })}
        showProjectHit
        onOpenMenu={noOp}
      />
      <ScheduledRunItem
        row={row({
          id: "transient-run",
          key: "transient-run",
          kind: "run",
          title: "Transient scheduled iteration",
          running: true,
          raw: "running",
          status: { text: "Running", cls: "run" },
        })}
        onOpenMenu={noOp}
      />
    </Matrix>
  ),
};

export const RunItemKeyboardActions: Story = {
  args: { onOpenMenu: fn() },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const item = canvas.getByTitle(
      "Review every component state and interaction " +
        "Weekdays at 8:00 AM · Next run in 12h agentrunner",
    );
    item.focus();
    await userEvent.keyboard("{Shift>}{F10}{/Shift}");
    await expect(args.onOpenMenu).toHaveBeenCalledOnce();
  },
};

export const RunItemActionVisibilityStates: Story = {
  parameters: {
    pseudo: {
      rootSelector: "body",
      hover: '[data-testid="interactive-row"] .scheduled-row-wrap',
    },
  },
  render: () => (
    <Matrix>
      <div className="pseudo-hover" data-testid="interactive-row">
        <ScheduledRunItem
          row={row({
            id: "interactive",
            key: "interactive",
            title: "Hover or focus reveals actions",
          })}
          onOpenMenu={noOp}
        />
      </div>
    </Matrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const interactive = canvas
      .getByTestId("interactive-row")
      .querySelector(".scheduled-row-wrap") as HTMLElement;
    const more = within(interactive).getByRole("button", {
      name: "Actions for Hover or focus reveals actions",
    });

    await waitFor(() => {
      expect(interactive).toBeVisible();
      expect(interactive).toHaveClass("pseudo-hover");
    }, { timeout: 3_000 });
    await waitFor(() => {
      expect(getComputedStyle(more).opacity).toBe("1");
    }, { timeout: 3_000 });
  },
};

export const RunItemFocusAndMenuOpen: Story = {
  render: () => (
    <Matrix>
      <div data-testid="interactive-row">
        <ScheduledRunItem
          row={row({
            id: "interactive",
            key: "interactive",
            title: "Focus reveals actions",
          })}
          onOpenMenu={noOp}
        />
      </div>
      <div data-testid="menu-open-row">
        <ScheduledRunItem
          row={row({
            id: "menu-open",
            key: "menu-open",
            title: "Open menu keeps actions visible",
          })}
          menuOpen
          onOpenMenu={noOp}
        />
      </div>
      <div data-testid="settled-transient-row">
        <ScheduledRunItem
          row={row({
            id: "settled-transient",
            key: "settled-transient",
            kind: "run",
            title: "Settled transient run",
            active: false,
            running: false,
            settled: true,
            raw: "done",
            status: { text: "Completed", cls: "idle" },
          })}
          onOpenMenu={noOp}
        />
      </div>
    </Matrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const interactive = canvas
      .getByTestId("interactive-row")
      .querySelector(".scheduled-row-wrap") as HTMLElement;
    const more = within(interactive).getByRole("button", {
      name: "Actions for Focus reveals actions",
    });

    await waitFor(() => {
      expect(interactive).toBeVisible();
      expect(getComputedStyle(more).opacity).toBe("0");
    });
    const rowButton = within(interactive).getByTitle(
      "Review every component state and interaction " +
        "Weekdays at 8:00 AM · Next run in 12h agentrunner",
    );
    rowButton.focus();
    await expect(rowButton).toHaveFocus();
    await waitFor(() => expect(getComputedStyle(more).opacity).toBe("1"));

    const menuOpenMore = within(
      canvas.getByTestId("menu-open-row"),
    ).getByRole("button", {
      name: "Actions for Open menu keeps actions visible",
    });
    await expect(getComputedStyle(menuOpenMore).opacity).toBe("1");
    await expect(
      within(canvas.getByTestId("settled-transient-row")).queryByRole(
        "button",
        { name: /Actions for/ },
      ),
    ).toBeNull();
  },
};

function ActionsFixture({
  state,
}: {
  state: "active" | "paused" | "recoverable" | "settled" | "run";
}) {
  const model =
    state === "paused"
      ? row({ paused: true, active: false, raw: "paused" })
      : state === "recoverable"
        ? row({
            recover: true,
            alert: "Needs recovery",
            status: { text: "Needs recovery", cls: "stranded" },
          })
        : state === "settled"
          ? row({ active: false, settled: true, raw: "done" })
          : state === "run"
            ? row({ kind: "run", running: true, raw: "running" })
            : row();
  return (
    <div className="min-h-[420px] p-6">
      <ScheduledRunActions
        row={model}
        x={32}
        y={32}
        pinned={state === "settled"}
        archived={state === "settled"}
        unread={state === "active"}
        onClose={fn()}
        onResume={fn()}
        onRetry={fn()}
        onPause={fn()}
        onCancel={fn()}
        onTogglePin={fn()}
        onRename={fn()}
        onToggleRead={fn()}
        onToggleArchive={fn()}
        onStop={fn()}
      />
    </div>
  );
}

export const RunActionsActive: Story = {
  render: () => <ActionsFixture state="active" />,
};

export const RunActionsPaused: Story = {
  render: () => <ActionsFixture state="paused" />,
};

export const RunActionsRecoverable: Story = {
  render: () => <ActionsFixture state="recoverable" />,
};

export const RunActionsSettled: Story = {
  render: () => <ActionsFixture state="settled" />,
};

export const RunActionsTransientRun: Story = {
  render: () => <ActionsFixture state="run" />,
};

export const ToolbarStateMatrix: Story = {
  render: () => (
    <Matrix>
      {(["all", "active", "paused"] as ScheduledFilter[]).map((filter) => (
        <ScheduledToolbar
          key={filter}
          query={filter === "active" ? "storybook" : ""}
          filter={filter}
          unreadCount={filter === "all" ? 3 : 0}
          onQueryChange={noOp}
          onFilterChange={noOp}
          onMarkAllRead={noOp}
        />
      ))}
    </Matrix>
  ),
};

function ToolbarFixture() {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ScheduledFilter>("all");
  const [unreadCount, setUnreadCount] = useState(2);
  return (
    <ScheduledToolbar
      query={query}
      filter={filter}
      unreadCount={unreadCount}
      onQueryChange={setQuery}
      onFilterChange={setFilter}
      onMarkAllRead={() => setUnreadCount(0)}
    />
  );
}

export const ToolbarInteraction: Story = {
  render: () => (
    <Matrix>
      <ToolbarFixture />
    </Matrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      canvas.getByRole("textbox", { name: "Search scheduled runs" }),
      "daily",
    );
    await userEvent.click(canvas.getByRole("tab", { name: "Paused" }));
    await expect(canvas.getByRole("tab", { name: "Paused" }))
      .toHaveAttribute("aria-selected", "true");
    await userEvent.click(
      canvas.getByRole("button", { name: /Mark all as read/ }),
    );
    await expect(
      canvas.queryByRole("button", { name: /Mark all as read/ }),
    ).not.toBeInTheDocument();
  },
};

export const SuggestionsAllCadences: Story = {
  render: () => (
    <Matrix>
      <ScheduledSuggestions onSelect={fn()} />
    </Matrix>
  ),
};

export const SuggestionSelection: Story = {
  render: () => (
    <Matrix>
      <ScheduledSuggestions onSelect={fn()} />
    </Matrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    for (const suggestion of SUGGESTIONS) {
      await expect(
        canvas.getByRole("button", { name: new RegExp(suggestion.title) }),
      ).toBeVisible();
    }
  },
};

export const EmptyStateMatrix: Story = {
  render: () => (
    <Matrix>
      <ScheduledEmptyState kind="empty" />
      <ScheduledEmptyState kind="filter" filter="paused" />
      <ScheduledEmptyState kind="search" query="missing project" />
    </Matrix>
  ),
};
