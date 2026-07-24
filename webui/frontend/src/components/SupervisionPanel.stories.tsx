import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import type { BackgroundWork, DiffResp } from "../types";
import type { InspectNode } from "./Subagents";
import {
  EnvironmentSection as EnvironmentSectionView,
  SupervisionPanel,
  type GoalState,
  type ProgressItem,
} from "./SupervisionPanel";

type StoryApi = AppServices["api"];

const SID = "story-supervision-panel";

const environmentDiff: DiffResp = {
  workspace:
    "/Users/demo/.local/share/agentrunner/worktrees/storybook-component-system-with-an-intentionally-long-name",
  known: true,
  isRepo: true,
  nested: false,
  diff: `diff --git a/src/ScenarioRunner.ts b/src/ScenarioRunner.ts
--- a/src/ScenarioRunner.ts
+++ b/src/ScenarioRunner.ts
@@ -1 +1,3 @@
-export const state = "idle";
+export const state = "running";
+export const owner = "autoplay";
`,
  numstat: "",
  untracked: ["qa/storybook-browser-results.json"],
  branch: "storybook-component-system",
};

const goal: GoalState = {
  goal:
    "Ship a deterministic, browser-tested component system without changing production behavior",
  checks: 3,
  max_checks: 7,
  verifiers: 2,
};

const progress: ProgressItem[] = [
  { id: "store", title: "Isolate runtime services and store", status: "done" },
  { id: "stories", title: "Cover public components in Storybook", status: "running" },
  { id: "browser", title: "Verify desktop and phone journeys", status: "pending" },
  {
    id: "golden",
    title: "Review the intentionally long golden-baseline description without clipping the status",
    status: "failed",
  },
];

const children: InspectNode[] = [
  {
    call_id: "call-implementation",
    agent: "implementation",
    session: "story-child-implementation",
    report: {
      status: "running",
      gen_steps: 14,
      usage: { billed: 24_800 },
    },
  },
  {
    call_id: "call-review",
    agent: "reviewer-with-an-intentionally-long-name",
    session: "story-child-review",
    report: {
      status: "waiting",
      waiting: {
        kind: "approval",
        tool: "exec_command",
      },
    },
  },
  {
    call_id: "call-future",
    agent: "future-agent",
    session: "story-child-future",
    reason: "future_terminal_state_not_yet_classified",
  },
];

const backgroundWork: BackgroundWork[] = [
  {
    handle: "storybook-browser",
    tool: "spawn_agent",
    detail:
      "agent=browser-verifier prompt=Run Chromium interaction checks while implementation continues",
  },
];

function isolatedApi(overrides: Partial<StoryApi>): StoryApi {
  return new Proxy(overrides as StoryApi, {
    get(target, property, receiver) {
      if (Reflect.has(target, property)) {
        return Reflect.get(target, property, receiver);
      }
      return () =>
        Promise.reject(
          new Error(`Unexpected Story API call: ${String(property)}`),
        );
    },
  });
}

function useStoryApi({
  diff = environmentDiff,
  branch = "storybook-component-system",
}: {
  diff?: DiffResp;
  branch?: string;
} = {}) {
  const [api] = useState(() =>
    isolatedApi({
      diff: fn(async () => diff),
      rawEvents: fn(async () => []),
      gitBranches: fn(async () => ({
        isRepo: true,
        current: branch,
        branches: ["main", "storybook-component-system"],
        dirty: 2,
        hasCommits: true,
      })),
      commit: fn(async () => ({ status: "ok" })),
      push: fn(async () => ({
        status: "ok",
        branch: "storybook-component-system",
      })),
      gitCheckout: fn(async (_dir, branch) => ({ status: "ok", branch })),
      applyWorktree: fn(async () => ({
        status: "ok",
        mainRepo: "/projects/agentrunner",
        applied: "storybook.patch",
      })),
      removeWorktree: fn(async () => ({
        status: "ok",
        mainRepo: "/projects/agentrunner",
      })),
      openIn: fn(async () => ({ status: "ok" })),
      projects: fn(async () => ({})),
    }),
  );
  return api;
}

function SupervisionFixture(
  props: React.ComponentProps<typeof SupervisionPanel>,
) {
  const api = useStoryApi();
  return (
    <StoryAppFrame
      initialState={{ currentSid: SID }}
      services={{ api }}
    >
      <div className="session-view h-[720px] min-h-[520px]">
        <div
          aria-hidden="true"
          className="mx-auto mt-20 w-full max-w-[720px] px-8 text-sm text-dim"
        >
          The production conversation remains in place beneath the Environment
          inspection card.
        </div>
        <SupervisionPanel {...props} />
      </div>
    </StoryAppFrame>
  );
}

function EnvironmentFixture({
  diff = environmentDiff,
  sid = SID,
  branch = "storybook-component-system",
}: {
  diff?: DiffResp;
  sid?: string;
  branch?: string;
} = {}) {
  const api = useStoryApi({ diff, branch });
  return (
    <StoryAppFrame initialState={{ currentSid: sid }} services={{ api }}>
      <div className="panel-body mx-auto w-full max-w-[520px] p-4">
        <EnvironmentSectionView onOpenChanges={fn()} refreshKey={1} />
      </div>
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Supervision/SupervisionPanel",
  component: SupervisionPanel,
  parameters: {
    layout: "fullscreen",
  },
  args: {
    loading: false,
    goal,
    goalEdit: null,
    progress,
    artifacts: [
      { stream: "reports/INC-99-browser-verification.md", version: 3 },
      {
        stream:
          "screenshots/mobile-storybook-component-system-with-a-very-long-file-name.png",
        version: 12,
      },
    ],
    children,
    backgroundWork,
    approvals: 2,
    answers: 1,
    childAnswers: [
      {
        agent: "accessibility-reviewer",
        session: "story-child-a11y-question",
      },
    ],
    sessionIdle: true,
    recovery: false,
    refreshKey: 1,
    onOpenChanges: fn(),
    onGoalEdit: fn(),
    onGoalSave: fn(),
    onGoalDiscard: fn(),
    onGoalAction: fn(),
    onOpenArtifact: fn(),
    onOpenChild: fn(),
    onInspect: fn(),
    onClose: fn(),
  },
  render: (args) => <SupervisionFixture {...args} />,
} satisfies Meta<typeof SupervisionPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.findByText("Environment")).resolves.toBeVisible();
    await expect(canvas.getByText("Background processes")).toBeVisible();
    await expect(canvas.getByText("Progress")).toBeVisible();
    await expect(canvas.getByText("Attention")).toBeVisible();
  },
};

export const KeyboardNavigation: Story = {
  args: {
    onClose: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Environment");
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    const close = canvas.getByRole("button", { name: "Hide Environment" });
    await expect(close).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onClose).toHaveBeenCalledOnce();
  },
};

export const GoalEditing: Story = {
  args: {
    goalEdit:
      "Ship the component system after independent review and browser evidence",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = await canvas.findByRole("textbox", { name: "Goal" });
    await expect(input).toHaveFocus();
    await expect(canvas.getByRole("button", { name: "Save" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Discard" })).toBeVisible();
  },
};

export const FailureUnknownAndOverflow: Story = {
  args: {
    goal: null,
    progress,
    children,
    backgroundWork: [],
    approvals: 0,
    answers: 0,
    childAnswers: [],
    recovery: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(
        "Review the intentionally long golden-baseline description without clipping the status",
      ),
    ).toBeVisible();
    await expect(canvas.getByText("Session needs recovery")).toBeVisible();
    await expect(canvas.getByText("future-agent")).toBeVisible();
  },
};

export const EnvironmentSection: Story = {
  render: () => (
    <EnvironmentFixture
      diff={{
        ...environmentDiff,
        worktree: true,
        mainRepo: "/Users/demo/projects/agentrunner",
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const changes = await canvas.findByRole("button", {
      name: /Changes/,
    });
    await expect(changes).toHaveTextContent("1 file");
    await expect(changes).toHaveTextContent("+2");
    await expect(changes).toHaveTextContent("-1");
    await expect(changes).toHaveTextContent("1 new");

    const worktree = canvas.getByRole("button", { name: /Worktree/ });
    worktree.focus();
    await userEvent.keyboard("{Enter}");
    await expect(worktree).toHaveAttribute("aria-expanded", "true");
    await expect(canvas.getByRole("button", { name: /Copy path/ })).toBeVisible();
    await expect(canvas.getByRole("button", { name: /Apply to project/ })).toBeVisible();
    await expect(canvas.getByRole("button", { name: /Open in VS Code/ })).toBeVisible();
    await expect(canvas.getByRole("button", { name: /Remove worktree/ })).toBeVisible();
  },
};

export const EnvironmentCleanWorktree: Story = {
  render: () => (
    <EnvironmentFixture
      diff={{
        ...environmentDiff,
        diff: "",
        untracked: [],
        worktree: true,
        mainRepo: "/Users/demo/projects/agentrunner",
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Environment");
    await expect(canvas.queryByRole("button", { name: /Changes/ })).not.toBeInTheDocument();
    await expect(canvas.queryByRole("button", { name: /Commit or push/ })).not.toBeInTheDocument();
    const worktree = canvas.getByRole("button", { name: /Worktree/ });
    await userEvent.click(worktree);
    await expect(canvas.getByRole("button", { name: /Apply to project/ })).toBeDisabled();
    await expect(canvas.getByRole("button", { name: /Remove worktree/ })).toBeVisible();
  },
};

export const EnvironmentInPlaceWorkspace: Story = {
  render: () => (
    <EnvironmentFixture
      diff={{
        ...environmentDiff,
        worktree: false,
        mainRepo: "",
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const worktree = await canvas.findByRole("button", { name: /Worktree/ });
    await userEvent.click(worktree);
    await expect(canvas.getByRole("button", { name: /Open in VS Code/ })).toBeVisible();
    await expect(canvas.queryByRole("button", { name: /Apply to project/ })).not.toBeInTheDocument();
    await expect(canvas.queryByRole("button", { name: /Remove worktree/ })).not.toBeInTheDocument();
    await expect(canvas.getByRole("button", { name: /Commit or push/ })).toBeVisible();
  },
};

export const EnvironmentSubagent: Story = {
  render: () => (
    <EnvironmentFixture
      sid="story-supervision-panel-sub-browser-reviewer"
      diff={{
        ...environmentDiff,
        worktree: true,
        mainRepo: "/Users/demo/projects/agentrunner",
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Environment");
    await expect(canvas.getByRole("button", { name: /Changes/ })).toBeVisible();
    await expect(canvas.queryByRole("button", { name: /Commit or push/ })).not.toBeInTheDocument();
  },
};

export const EnvironmentCommitMenu: Story = {
  render: () => <EnvironmentFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByRole("button", { name: /Commit or push/ });
    await userEvent.click(trigger);
    const body = within(canvasElement.ownerDocument.body);
    await expect(body.getByRole("menuitem", { name: /Commit locally/ })).toBeVisible();
    await expect(body.getByRole("menuitem", { name: /Commit & push/ })).toBeVisible();
    await expect(body.getByRole("menuitem", { name: /Push existing commits/ })).toBeVisible();
  },
};

export const Loading: Story = {
  args: {
    loading: true,
    goal: null,
    progress: [],
    artifacts: [],
    children: [],
    backgroundWork: [],
    approvals: 0,
    answers: 0,
    childAnswers: [],
    sessionIdle: false,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Checking…")).toBeVisible();
    await expect(canvas.queryByText("Nothing needs you")).not.toBeInTheDocument();
  },
};

export const Resting: Story = {
  args: {
    goal: null,
    progress: [],
    artifacts: [],
    children: [],
    backgroundWork: [],
    approvals: 0,
    answers: 0,
    childAnswers: [],
    sessionIdle: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Nothing needs you")).toBeVisible();
    await expect(canvas.queryByText("Goal")).not.toBeInTheDocument();
    await expect(canvas.queryByText("Agents")).not.toBeInTheDocument();
    await expect(canvas.queryByText("Attention")).not.toBeInTheDocument();
  },
};
