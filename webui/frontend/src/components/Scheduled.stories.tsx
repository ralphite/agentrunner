import type { Meta, StoryObj } from "@storybook/react-vite";
import { delay, HttpResponse, http } from "msw";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import {
  buildRun,
  buildScheduleDetail,
  buildSession,
  fixtureDefaults,
} from "../storybook/fixtures";
import { createStoryApiHandlers } from "../storybook/handlers";
import type {
  Run,
  ScheduleDetail as ScheduleDetailData,
  Session,
} from "../types";
import {
  ScheduleDetailPanel as ScheduleDetailPanelView,
  ScheduleEditDialog as ScheduleEditDialogView,
  Scheduled,
} from "./Scheduled";

const NOW = Date.parse("2026-07-23T18:00:00Z");

const storyClock: AppServices["clock"] = {
  now: () => NOW,
  setTimeout: (callback) => {
    queueMicrotask(callback);
    return 0 as unknown as ReturnType<typeof setTimeout>;
  },
  clearTimeout: () => {},
  setInterval: () => 0 as unknown as ReturnType<typeof setInterval>,
  clearInterval: () => {},
};

const active = buildSession({
  id: "20260723-174500-story-active",
  title: "Review Storybook coverage and report gaps",
  status: "running",
  kind: "driver",
  schedule: "interval",
  cadence: "Every 30m",
  nextRunAt: "2026-07-23T18:30:00Z",
  scheduleControl: true,
  scheduleDetail: true,
  workspace: fixtureDefaults.workspace,
  attention: { approvals: 0, answers: 0 },
});

const approval = buildSession({
  id: "20260723-170000-story-approval",
  title: "Publish the verified browser evidence",
  status: "waiting:approval",
  kind: "driver",
  schedule: "cron",
  cadence: "Weekdays at 4:00 PM",
  nextRunAt: "2026-07-24T23:00:00Z",
  scheduleControl: true,
  scheduleDetail: true,
  attention: { approvals: 1, answers: 0 },
});

const paused = buildSession({
  id: "20260722-160000-story-paused",
  title: "Check component screenshots for regressions",
  status: "paused",
  kind: "driver",
  schedule: "cron",
  cadence: "Fridays at 4:00 PM",
  nextRunAt: undefined,
  scheduleControl: true,
  scheduleDetail: true,
});

const failed = buildSession({
  id: "20260722-140000-story-failed",
  title: "Monitor the visual test pipeline",
  status: "crash",
  kind: "driver",
  schedule: "interval",
  cadence: "Every 6h",
  nextRunAt: undefined,
  scheduleControl: true,
  scheduleDetail: true,
});

const completed = buildSession({
  id: "20260721-120000-story-completed",
  title: "Run the four-part accessibility audit",
  status: "max_iterations",
  kind: "driver",
  schedule: "self_paced",
  cadence: "Self-paced",
  nextRunAt: undefined,
  scheduleControl: false,
  scheduleDetail: true,
});

const runningRun = buildRun({
  id: "story-scheduled-run",
  kind: "drive",
  label: "Check pull request comments",
  status: "running",
  schedule: "interval",
  cadence: "Every 15m",
  nextRunAt: "2026-07-23T18:15:00Z",
  sessionId: undefined,
});

const sessions = [active, approval, paused, failed, completed];
const runs = [runningRun];

const details: Record<string, ScheduleDetailData> = {
  [active.id]: buildScheduleDetail({
    sessionId: active.id,
    name: "Review Storybook coverage",
    status: "active",
    prompt: "Review Storybook coverage and report every missing component state.",
    cadence: "Every 30m",
    nextRunAt: active.nextRunAt,
    interval: "30m",
    iterations: 3,
    maxIterations: 12,
    thinkingEnabled: true,
    thinkingBudgetTokens: 4096,
  }),
  [approval.id]: buildScheduleDetail({
    sessionId: approval.id,
    name: "Publish browser evidence",
    status: "active",
    schedule: "cron",
    cron: "0 16 * * 1-5",
    interval: undefined,
    cadence: "Weekdays at 4:00 PM",
    nextRunAt: approval.nextRunAt,
  }),
  [paused.id]: buildScheduleDetail({
    sessionId: paused.id,
    name: "Screenshot regression check",
    status: "paused",
    schedule: "cron",
    cron: "0 16 * * 5",
    interval: undefined,
    cadence: "Fridays at 4:00 PM",
    nextRunAt: undefined,
  }),
  [failed.id]: buildScheduleDetail({
    sessionId: failed.id,
    name: "Visual test monitor",
    status: "crash",
    cadence: "Every 6h",
    interval: "6h",
    nextRunAt: undefined,
  }),
  [completed.id]: buildScheduleDetail({
    sessionId: completed.id,
    name: "Accessibility audit",
    status: "max_iterations",
    schedule: "self_paced",
    cadence: "Self-paced",
    nextRunAt: undefined,
    scheduleControl: false,
    scheduleEdit: false,
    iterations: 4,
    maxIterations: 4,
  }),
};

interface ScheduledFixture {
  api: ReturnType<typeof createStoryApiHandlers>;
  initialState: Partial<AppState>;
}

function makeFixture(options: {
  sessions?: Session[];
  runs?: Run[];
  detailSid?: string | null;
  unread?: string[];
} = {}): ScheduledFixture {
  const fixtureSessions = options.sessions ?? sessions;
  const fixtureRuns = options.runs ?? runs;
  return {
    api: createStoryApiHandlers({
      sessions: fixtureSessions,
      runs: fixtureRuns,
      schedules: details,
    }),
    initialState: {
      sessions: fixtureSessions,
      sessionsReady: true,
      runs: fixtureRuns,
      unread: options.unread ?? [approval.id, failed.id],
      scheduledDetailSid: options.detailSid ?? null,
    },
  };
}

function renderFixture(fixture: ScheduledFixture) {
  return (
    <StoryAppFrame
      initialState={fixture.initialState}
      services={{ clock: storyClock }}
    >
      <div className="h-screen min-h-[640px]">
        <Scheduled />
      </div>
    </StoryAppFrame>
  );
}

const defaultFixture = makeFixture();
const meta = {
  title: "Pages/Scheduled",
  component: Scheduled,
  parameters: {
    layout: "fullscreen",
    msw: { handlers: defaultFixture.api.handlers },
  },
  render: () => renderFixture(defaultFixture),
} satisfies Meta<typeof Scheduled>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "Scheduled runs" })).toBeVisible();
    await expect(canvas.getByText("Every 30m")).toBeVisible();
    await expect(canvas.getByTitle("Needs approval")).toBeVisible();
    await expect(canvas.getByText("Failed")).toBeVisible();
    await expect(canvas.getByText("Paused", { selector: ".sched-sub span" })).toBeVisible();
  },
};

const keyboardFixture = makeFixture();
export const KeyboardNavigation: Story = {
  parameters: { msw: { handlers: keyboardFixture.api.handlers } },
  render: () => renderFixture(keyboardFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Create scheduled work" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByRole("menu")).toBeVisible();
    await expect(canvas.getByRole("menuitem", { name: /Repeating/ })).toBeVisible();
    await userEvent.keyboard("{Escape}");
    await expect(canvas.getByRole("button", { name: "Create scheduled work" })).toHaveFocus();
  },
};

const emptyFixture = makeFixture({ sessions: [], runs: [], unread: [] });
export const Empty: Story = {
  parameters: { msw: { handlers: emptyFixture.api.handlers } },
  render: () => renderFixture(emptyFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No scheduled work")).toBeVisible();
    await expect(canvas.getByRole("button", { name: /Daily brief/ })).toBeVisible();
  },
};

const detailFixture = makeFixture({ detailSid: active.id });
export const ScheduleDetail: Story = {
  parameters: { msw: { handlers: detailFixture.api.handlers } },
  render: () => renderFixture(detailFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("complementary", {
        name: "Schedule details for Review Storybook coverage and report gaps",
      }),
    ).toBeVisible();
    await expect(await canvas.findByText("4,096 token budget")).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: "Pause" }));
    await expect(await canvas.findByRole("button", { name: "Resume" })).toBeVisible();
  },
};

const pausedDetailFixture = makeFixture({ detailSid: paused.id });
export const PausedScheduleDetail: Story = {
  parameters: { msw: { handlers: pausedDetailFixture.api.handlers } },
  render: () => renderFixture(pausedDetailFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("complementary", {
        name: "Schedule details for Check component screenshots for regressions",
      }),
    ).toBeVisible();
    await expect(await canvas.findByRole("button", { name: "Resume" })).toBeVisible();
  },
};

const editFixture = makeFixture({ detailSid: active.id });
export const EditSchedule: Story = {
  parameters: { msw: { handlers: editFixture.api.handlers } },
  render: () => renderFixture(editFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button", { name: "Edit" }));
    await expect(canvas.getByRole("dialog", { name: "Edit schedule" })).toBeVisible();
    const interval = canvas.getByRole("textbox", { name: "Interval" });
    await userEvent.clear(interval);
    await userEvent.type(interval, "45m");
    await expect(canvas.getByRole("button", { name: "Save" })).toBeEnabled();
  },
};

const loadingFixture = makeFixture({ detailSid: active.id });
export const DetailLoading: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get("/api/sessions/:sid/schedule", async () => {
          await delay("infinite");
          return HttpResponse.json({});
        }),
        ...loadingFixture.api.handlers,
      ],
    },
  },
  render: () => renderFixture(loadingFixture),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("status"),
    ).toHaveTextContent("Loading schedule details…");
  },
};

const errorFixture = makeFixture({ detailSid: active.id });
export const DetailError: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get("/api/sessions/:sid/schedule", () =>
          HttpResponse.json(
            { error: "Fixture schedule service is unavailable." },
            { status: 503 },
          )),
        ...errorFixture.api.handlers,
      ],
    },
  },
  render: () => renderFixture(errorFixture),
  play: async ({ canvasElement }) => {
    const alert = await within(canvasElement).findByRole("alert");
    await expect(alert).toHaveTextContent("Schedule details unavailable");
    await expect(alert).toHaveTextContent("Fixture schedule service is unavailable.");
  },
};

const searchFixture = makeFixture();
export const FilterAndNoResults: Story = {
  parameters: { msw: { handlers: searchFixture.api.handlers } },
  render: () => renderFixture(searchFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const search = canvas.getByRole("textbox", { name: "Search scheduled runs" });
    await userEvent.type(search, "does-not-exist");
    await expect(canvas.getByText('No results for "does-not-exist".')).toBeVisible();
    await userEvent.clear(search);
    await userEvent.click(canvas.getByRole("tab", { name: "Paused" }));
    await expect(canvas.getByText("Check component screenshots for regressions")).toBeVisible();
    await waitFor(() => {
      expect(canvas.queryByText("Review Storybook coverage and report gaps")).toBeNull();
    });
  },
};

const closeDetail = fn();
const retryDetail = fn();
const openHistory = fn();
const changeCadence = fn();
const editDetail = fn();

export const ScheduleDetailPanel: Story = {
  render: () => (
    <StoryAppFrame>
      <div className="mx-auto h-[720px] max-w-[520px] bg-bg">
        <ScheduleDetailPanelView
          title="Review Storybook coverage"
          detail={details[active.id]}
          loading={false}
          error=""
          acting={false}
          onClose={closeDetail}
          onRetry={retryDetail}
          onHistory={openHistory}
          onCadence={changeCadence}
          onEdit={editDetail}
        />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("complementary", {
        name: "Schedule details for Review Storybook coverage",
      }),
    ).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: "Pause" }));
    await expect(changeCadence).toHaveBeenCalledWith("pause");
    await userEvent.click(canvas.getByRole("button", { name: "Edit" }));
    await expect(editDetail).toHaveBeenCalled();
  },
};

const leafEditFixture = makeFixture({ sessions: [active], runs: [] });
const scheduleSaved = fn(async () => {});

export const ScheduleEditDialog: Story = {
  parameters: {
    msw: { handlers: leafEditFixture.api.handlers },
  },
  render: () => (
    <StoryAppFrame
      initialState={leafEditFixture.initialState}
      services={{ clock: storyClock }}
    >
      <ScheduleEditDialogView
        detail={details[active.id]}
        onClose={fn()}
        onSaved={scheduleSaved}
      />
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("dialog", { name: "Edit schedule" })).toBeVisible();
    const interval = canvas.getByRole("textbox", { name: "Interval" });
    await userEvent.clear(interval);
    await userEvent.type(interval, "not-a-duration");
    await expect(canvas.getByRole("alert")).toBeVisible();
    await userEvent.clear(interval);
    await userEvent.type(interval, "45m");
    await userEvent.click(canvas.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(scheduleSaved).toHaveBeenCalled());
  },
};
