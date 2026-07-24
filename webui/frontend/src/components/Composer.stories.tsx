import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  expect,
  fireEvent,
  fn,
  userEvent,
  waitFor,
  within,
} from "storybook/test";
import type { ForkDraft } from "../api";
import type { AppServices } from "../app/appServices";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { buildSession, fixtureDefaults } from "../storybook/fixtures";
import { createStoryApiHandlers } from "../storybook/handlers";
import { humanPause } from "../storybook/humanPlayback";
import {
  Composer,
  GoalLoopLauncher as GoalLoopLauncherView,
  type SessionActions,
} from "./Composer";

type StoryApi = AppServices["api"];

const SID = "story-composer-session";
const SECOND_WORKSPACE = "/workspace/agent-runner-docs";
const HOME_DRAFT = "Review the Storybook component coverage";
const SESSION_DRAFT = "Please verify the responsive keyboard flow";

const sessions = [
  buildSession({
    id: SID,
    workspace: fixtureDefaults.workspace,
    title: "Build the Storybook component system",
    status: "running",
    turns: 5,
  }),
  buildSession({
    id: "story-composer-docs",
    workspace: SECOND_WORKSPACE,
    title: "Document component states",
    status: "completed",
    turns: 2,
  }),
];

const attachmentHandlers = createStoryApiHandlers({
  sessions,
  events: {
    [SID]: [],
    "story-composer-docs": [],
  },
});

function isolatedApi(overrides: Partial<StoryApi> = {}): StoryApi {
  const implemented: Partial<StoryApi> = {
    gitBranches: async () => ({
      isRepo: true,
      current: "storybook/components",
      branches: ["main", "storybook/components"],
      dirty: 1,
      hasCommits: true,
    }),
    makeWorkspace: async () => ({ path: fixtureDefaults.workspace }),
    makeWorktree: async () => ({
      path: `${fixtureDefaults.workspace}-worktree`,
      repo: fixtureDefaults.workspace,
      branch: "storybook/components",
    }),
    gitCheckout: async (_dir, branch) => ({
      status: "ok",
      branch,
    }),
    newSession: async () => ({ sid: "story-composer-created" }),
    send: async () => ({ status: "queued" }),
    startRun: async () => ({ runId: "story-composer-run" }),
    runs: async () => [],
    files: async () => ({
      workspace: fixtureDefaults.workspace,
      known: true,
      files: [
        "src/components/Composer.tsx",
        "src/components/Composer.stories.tsx",
      ],
    }),
    upload: async (file) => ({
      path: `/storybook-uploads/${file.name}`,
      name: file.name,
    }),
    optimize: async (draft) => ({
      text: `${draft.trim()} — clarified for deterministic Storybook review.`,
    }),
    switchAgent: async () => ({ status: "ok" }),
    goal: async () => ({ status: "ok" }),
    mode: async () => ({ status: "ok" }),
    compact: async () => ({ status: "ok" }),
    clear: async () => ({ status: "ok" }),
    ...overrides,
  };

  return new Proxy(implemented as StoryApi, {
    get(target, property, receiver) {
      if (Reflect.has(target, property)) {
        return Reflect.get(target, property, receiver);
      }
      return () =>
        Promise.reject(
          new Error(
            `Unexpected Composer Storybook API call: ${String(property)}`,
          ),
        );
    },
  });
}

interface ComposerFixtureProps {
  variant?: "home" | "session";
  running?: boolean;
  homeDraft?: string;
  sessionDraft?: string;
  selectedProject?: string;
  seed?: ForkDraft | null;
  onSend?: Extract<
    React.ComponentProps<typeof Composer>,
    { variant: "session" }
  >["onSend"];
  actions?: SessionActions;
  onError?: (message: string) => void;
}

function ComposerFixture({
  variant = "home",
  running = false,
  homeDraft = "",
  sessionDraft = "",
  selectedProject = fixtureDefaults.workspace,
  seed = null,
  onSend = async () => {},
  actions,
  onError = () => {},
}: ComposerFixtureProps) {
  const [api] = useState(() => isolatedApi());
  const initialState = {
    sessions,
    sessionsReady: true,
    currentSid: variant === "session" ? SID : null,
    runs: [],
  } satisfies Partial<AppState>;
  const local = {
    "arwebui.lastProject": selectedProject,
    "arwebui.lastAccess": "ask",
    "arwebui.sessAccess": JSON.stringify({ [SID]: "ask" }),
  };
  const session = {
    ...(homeDraft ? { "arwebui.draft.~home": homeDraft } : {}),
    ...(sessionDraft ? { [`arwebui.draft.${SID}`]: sessionDraft } : {}),
  };

  return (
    <StoryAppFrame
      initialState={initialState}
      services={{ api, local, session }}
    >
      <div className="mx-auto flex min-h-[360px] w-full max-w-4xl items-end p-6">
        <div className="w-full">
          {variant === "home" ? (
            <Composer variant="home" onError={onError} />
          ) : (
            <Composer
              variant="session"
              sid={SID}
              workspace={fixtureDefaults.workspace}
              mode="default"
              running={running}
              seed={seed}
              onSend={onSend}
              actions={actions}
              onError={onError}
            />
          )}
        </div>
      </div>
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Input/Composer",
  component: Composer,
  parameters: {
    layout: "fullscreen",
  },
  args: {
    variant: "home",
    onError: fn(),
  },
  render: () => <ComposerFixture />,
} satisfies Meta<typeof Composer>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText("Do anything");
    await expect(input).toHaveFocus();
    await expect(
      canvas.getByRole("button", { name: "Ask to approve" }),
    ).toBeVisible();
    await expect(canvas.getByTitle("Model & effort")).toHaveTextContent(
      "Gemini Flash",
    );
    await expect(canvas.getByTitle("Send (Enter)")).toBeDisabled();
  },
};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText("Do anything");
    await expect(input).toHaveFocus();

    await userEvent.type(input, "Keyboard-only component review");
    await userEvent.tab();
    await expect(
      canvas.getByRole("button", { name: "Add and advanced options" }),
    ).toHaveFocus();
    await userEvent.keyboard("{ArrowDown}");
    const page = within(canvasElement.ownerDocument.body);
    await waitFor(() =>
      expect(
        page.getByRole("menuitem", { name: /Files and folders/ }),
      ).toHaveFocus(),
    );
    await userEvent.keyboard("{Escape}");
    await expect(
      canvas.getByRole("button", { name: "Add and advanced options" }),
    ).toHaveFocus();
  },
};

export const Draft: Story = {
  render: () => <ComposerFixture homeDraft={HOME_DRAFT} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText("Do anything");
    await expect(input).toHaveValue(HOME_DRAFT);
    await expect(
      canvas.getByTitle("Optimize prompt — rewrite this draft to be clearer"),
    ).toBeVisible();

    await userEvent.click(
      canvas.getByTitle("Optimize prompt — rewrite this draft to be clearer"),
    );
    await expect(input).toHaveValue(
      `${HOME_DRAFT} — clarified for deterministic Storybook review.`,
    );
    await expect(
      canvas.getByTitle("Undo optimize — restore your original draft"),
    ).toBeVisible();
  },
};

const queuedSend = fn(async () => {});

export const RunningQueued: Story = {
  render: () => (
    <ComposerFixture
      variant="session"
      running
      sessionDraft={SESSION_DRAFT}
      onSend={queuedSend}
    />
  ),
  play: async ({ canvasElement }) => {
    queuedSend.mockClear();
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("group", { name: "Delivery mode" }),
    ).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Queue" })).toHaveClass(
      "on",
    );

    await userEvent.click(canvas.getByTitle(/Send · queue/));
    await expect(queuedSend).toHaveBeenCalledWith(
      SESSION_DRAFT,
      [],
      [],
      "queue",
      undefined,
    );
    await userEvent.type(
      canvas.getByPlaceholderText("Ask for follow-up changes"),
      SESSION_DRAFT,
    );
  },
};

const steerSend = fn(async () => {});

export const RunningSteer: Story = {
  render: () => (
    <ComposerFixture
      variant="session"
      running
      sessionDraft={SESSION_DRAFT}
      onSend={steerSend}
    />
  ),
  play: async ({ canvasElement }) => {
    steerSend.mockClear();
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Steer" }));
    await expect(canvas.getByRole("button", { name: "Steer" })).toHaveClass(
      "on",
    );

    await userEvent.click(canvas.getByTitle(/Send · steer/));
    await expect(steerSend).toHaveBeenCalledWith(
      SESSION_DRAFT,
      [],
      [],
      "steer",
      undefined,
    );
    await userEvent.type(
      canvas.getByPlaceholderText("Ask for follow-up changes"),
      SESSION_DRAFT,
    );
  },
};

const interrupt = fn();

export const StopActiveTurn: Story = {
  render: () => (
    <ComposerFixture variant="session" running actions={{ interrupt }} />
  ),
  play: async ({ canvasElement }) => {
    interrupt.mockClear();
    const canvas = within(canvasElement);
    const stop = canvas.getByRole("button", { name: "Stop active turn" });
    await expect(stop).toBeVisible();
    await userEvent.click(stop);
    await expect(interrupt).toHaveBeenCalledOnce();
  },
};

const forkDraft: ForkDraft = {
  draft_id: "story-fork-draft",
  text: "Continue from this reviewed checkpoint",
  content: [
    {
      kind: "image",
      ref: "story-screenshot",
      media_type: "image/png",
      name: "component-preview.png",
      part_id: "story-part-image",
    },
    {
      kind: "file",
      ref: "story-notes",
      media_type: "text/markdown",
      name: "review-notes.md",
      part_id: "story-part-file",
    },
  ],
};

export const ForkDraftWithAttachments: Story = {
  parameters: {
    msw: {
      handlers: attachmentHandlers.groups.sessions,
    },
  },
  render: () => <ComposerFixture variant="session" seed={forkDraft} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText("Ask for follow-up changes");
    await waitFor(() =>
      expect(input).toHaveValue("Continue from this reviewed checkpoint"),
    );
    const imageAttachment = canvas.getByRole("button", {
      name: "Remove attachment component-preview.png",
    });
    await expect(imageAttachment.querySelector("img")).toBeVisible();
    await expect(canvas.getByText("review-notes.md")).toBeVisible();
    await expect(canvas.getAllByTitle("Remove attachment")).toHaveLength(2);
  },
};

const longAttachmentSeed: ForkDraft = {
  draft_id: "story-long-attachment-draft",
  text: "Review this intentionally long multiline draft.\nConfirm that the composer grows without pushing its primary controls outside the card.\nThen summarize every attached artifact.",
  content: Array.from({ length: 9 }, (_, index) => ({
    kind: "file" as const,
    ref: `story-long-file-${index}`,
    media_type: "text/markdown",
    name:
      index === 0
        ? "an-extremely-long-review-artifact-filename-that-must-truncate-inside-the-composer.md"
        : `review-artifact-${index}.md`,
    part_id: `story-long-part-${index}`,
  })),
};

export const LongDraftAndAttachments: Story = {
  render: () => <ComposerFixture variant="session" seed={longAttachmentSeed} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText("Ask for follow-up changes");
    await waitFor(() => expect(input).toHaveValue(longAttachmentSeed.text));
    const attachments = canvas.getByRole("group", { name: "Attachments" });
    const longName = canvas.getByText(
      "an-extremely-long-review-artifact-filename-that-must-truncate-inside-the-composer.md",
    );
    await expect(canvas.getAllByTitle("Remove attachment")).toHaveLength(9);
    await expect(longName.scrollWidth).toBeGreaterThan(longName.clientWidth);
    await expect(attachments.getBoundingClientRect().height).toBeGreaterThan(
      40,
    );
  },
};

export const DraggingFiles: Story = {
  play: async ({ canvasElement }) => {
    const card = canvasElement.querySelector<HTMLElement>(".cx-card")!;
    const dataTransfer = new DataTransfer();
    dataTransfer.items.add(
      new File(["storybook"], "component-state.txt", {
        type: "text/plain",
      }),
    );
    Object.defineProperty(dataTransfer, "types", { value: ["Files"] });
    fireEvent.dragEnter(card, { dataTransfer });
    const overlay = within(canvasElement).getByText("Drop files to attach");
    await expect(overlay).toBeVisible();
    await expect(card).toHaveClass("dropping");
  },
};

const pendingSend = fn(() => new Promise<void>(() => {}));

export const BusyPendingSend: Story = {
  render: () => (
    <ComposerFixture
      variant="session"
      sessionDraft="Hold this draft while the send request is pending"
      onSend={pendingSend}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText("Ask for follow-up changes");
    const send = canvas.getByRole("button", { name: "Send message" });
    await userEvent.click(send);
    await waitFor(() => expect(send).toBeDisabled());
    await expect(input).toHaveValue(
      "Hold this draft while the send request is pending",
    );
  },
};

export const ProjectPicker: Story = {
  render: () => <ComposerFixture selectedProject="" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(canvas.getByTitle("Select project"));
    const search = page.getByLabelText("Search projects");
    await expect(search).toBeInTheDocument();
    search.focus();
    await userEvent.type(search, "docs", { skipClick: true });
    const docs = page.getByRole("button", { name: /agent-runner-docs/ });
    await expect(docs).toBeVisible();
    await humanPause();
    await userEvent.click(docs);
    await expect(canvas.getByTitle("Select project")).toHaveTextContent(
      "agent-runner-docs",
    );
  },
};

export const ModelAndEffort: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    const trigger = canvas.getByTitle("Model & effort");

    await userEvent.click(trigger);
    await userEvent.click(
      page.getByRole("menuitem", { name: /Model Gemini Flash/ }),
    );
    await humanPause();
    await userEvent.click(page.getByRole("menuitem", { name: /Gemini Pro/ }));
    await expect(trigger).toHaveTextContent("Gemini Pro");

    await userEvent.click(trigger);
    await userEvent.click(
      page.getByRole("menuitem", { name: /Effort Medium/ }),
    );
    await humanPause();
    await userEvent.click(
      page.getByRole("menuitem", {
        name: /^High Thorough reasoning on hard problems$/,
      }),
    );
    await expect(trigger).toHaveTextContent("High");
  },
};

export const AccessAndApproval: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      canvas.getByRole("button", { name: "Ask to approve" }),
    );

    await expect(
      page.getByRole("menuitem", {
        name: /Ask to approve Reads run freely; edits, shell & network ask first/,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("menuitem", {
        name: /Full access Nothing is gated/,
      }),
    ).toBeVisible();
    await humanPause();
    await userEvent.click(
      page.getByRole("menuitem", {
        name: /Auto-accept edits File edits apply automatically/,
      }),
    );
    await expect(
      canvas.getByRole("button", { name: "Auto-accept edits" }),
    ).toBeVisible();
  },
};

export const GoalLauncher: Story = {
  render: () => (
    <ComposerFixture homeDraft="Ship complete Storybook coverage" />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      canvas.getByRole("button", { name: "Add and advanced options" }),
    );
    await humanPause();
    await userEvent.click(page.getByRole("menuitem", { name: /^Goal / }));

    const input = canvas.getByPlaceholderText(
      "Describe your goal, define measurable outcomes for best results",
    );
    await expect(input).toHaveValue("Ship complete Storybook coverage");
    await expect(canvas.getByRole("button", { name: "Goal" })).toBeVisible();
  },
};

const startLoop = fn();

export const GoalLoopLauncher: Story = {
  render: () => (
    <StoryAppFrame>
      <div className="mx-auto max-w-[760px] p-6">
        <GoalLoopLauncherView
          mode="loop"
          initialPrompt="Re-run the Storybook browser audit"
          busy={false}
          onCancel={fn()}
          onStart={startLoop}
        />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const cadence = canvas.getByRole("textbox", { name: "Every" });
    await userEvent.clear(cadence);
    await userEvent.type(cadence, "not-a-duration");
    await expect(canvas.getByRole("alert")).toBeVisible();
    await humanPause();
    await userEvent.clear(cadence);
    await userEvent.type(cadence, "10m");
    await userEvent.click(canvas.getByRole("button", { name: "Start loop" }));
    await expect(startLoop).toHaveBeenCalledWith(
      "Re-run the Storybook browser audit",
      "10m",
      5,
    );
  },
};

export const GoalLoopModeMatrix: Story = {
  render: () => (
    <StoryAppFrame>
      <div className="mx-auto grid max-w-[760px] gap-4 p-6">
        <GoalLoopLauncherView
          mode="goal"
          initialPrompt="Keep improving coverage until every visible state is represented"
          busy={false}
          onCancel={fn()}
          onStart={fn()}
        />
        <GoalLoopLauncherView
          mode="loop"
          initialPrompt="Repeat the focused Storybook checks"
          busy={false}
          onCancel={fn()}
          onStart={fn()}
        />
        <GoalLoopLauncherView
          mode="best"
          initialPrompt="Try several fixture designs and keep the clearest result"
          busy={false}
          onCancel={fn()}
          onStart={fn()}
        />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("button", { name: "Start goal" }),
    ).toBeEnabled();
    await expect(
      canvas.getByRole("button", { name: "Start loop" }),
    ).toBeEnabled();
    await expect(
      canvas.getByRole("button", { name: "Start best-of-N" }),
    ).toBeEnabled();
    await expect(canvas.getByText("Attempts")).toBeVisible();
    await expect(canvas.getAllByText("Max rounds")).toHaveLength(2);
  },
};

export const GoalLoopInvalidInterval: Story = {
  render: () => (
    <StoryAppFrame>
      <div className="mx-auto max-w-[760px] p-6">
        <GoalLoopLauncherView
          mode="loop"
          initialPrompt="Repeat the focused Storybook checks"
          busy={false}
          onCancel={fn()}
          onStart={fn()}
        />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const cadence = canvas.getByRole("textbox", { name: "Every" });
    await userEvent.clear(cadence);
    await userEvent.type(cadence, "not-a-duration");
    await expect(canvas.getByRole("alert")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Start loop" }),
    ).toBeDisabled();
  },
};

export const GoalLoopEmptyAndBusy: Story = {
  render: () => (
    <StoryAppFrame>
      <div className="mx-auto grid max-w-[760px] gap-4 p-6">
        <GoalLoopLauncherView
          mode="goal"
          initialPrompt=""
          busy={false}
          onCancel={fn()}
          onStart={fn()}
        />
        <GoalLoopLauncherView
          mode="best"
          initialPrompt="Compare three complete attempts"
          busy
          onCancel={fn()}
          onStart={fn()}
        />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("button", { name: "Start goal" }),
    ).toBeDisabled();
    await expect(
      canvas.getByRole("button", { name: "Start best-of-N" }),
    ).toBeDisabled();
  },
};

export const FileMentionKeyboard: Story = {
  render: () => <ComposerFixture variant="session" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText("Ask for follow-up changes");
    await userEvent.type(input, "@");
    const first = await canvas.findByRole("option", {
      name: "src/components/Composer.tsx",
    });
    const second = canvas.getByRole("option", {
      name: "src/components/Composer.stories.tsx",
    });
    await expect(first).toHaveAttribute("aria-selected", "true");
    await userEvent.keyboard("{ArrowUp}");
    await expect(second).toHaveAttribute("aria-selected", "true");
    await humanPause();
    await userEvent.keyboard("{Escape}");
    await expect(
      canvas.queryByRole("listbox", { name: "Workspace files" }),
    ).not.toBeInTheDocument();
    await expect(input).toHaveFocus();

    await userEvent.clear(input);
    await userEvent.type(input, "@Comp");
    await canvas.findByRole("option", { name: "src/components/Composer.tsx" });
    await userEvent.keyboard("{ArrowDown}{Tab}");
    await expect(input).toHaveValue("src/components/Composer.stories.tsx ");
    await expect(input).toHaveFocus();
  },
};

export const SlashCommands: Story = {
  render: () => <ComposerFixture variant="session" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText("Ask for follow-up changes");
    await userEvent.type(input, "/");
    await expect(canvas.getByText("/goal")).toBeVisible();
    await expect(canvas.getByText("/interrupt")).toBeVisible();

    await userEvent.type(input, "mo");
    await expect(canvas.getByText("/mode")).toBeVisible();
    await expect(canvas.getByText("/model")).toBeVisible();
    await humanPause();
    await userEvent.keyboard("{ArrowDown}{Enter}");
    await expect(input).toHaveValue("/model ");
  },
};

export const SlashCommandKeyboardWrapAndEscape: Story = {
  render: () => <ComposerFixture variant="session" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText("Ask for follow-up changes");
    await userEvent.type(input, "/");
    const options = await canvas.findAllByRole("option");
    await userEvent.keyboard("{ArrowUp}");
    await expect(options[options.length - 1]).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await humanPause();
    await userEvent.keyboard("{Escape}");
    await expect(
      canvas.queryByRole("listbox", { name: "Slash commands" }),
    ).not.toBeInTheDocument();
    await expect(input).toHaveFocus();

    await userEvent.clear(input);
    await userEvent.type(input, "/mo");
    await canvas.findByText("/mode");
    await userEvent.keyboard("{ArrowDown}{Tab}");
    await expect(input).toHaveValue("/model ");
  },
};
