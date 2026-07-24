import { useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import type { AppState, ModalKind } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { humanPause } from "../storybook/humanPlayback";
import { Toasts } from "./Toasts";
import {
  AgentModal,
  ConfirmModal,
  ForkModal,
  MainModal,
  Modal,
  Modals,
  NewSessionModal,
  PromptModal,
  RenameModal,
  RunDetailsModal,
  RunModal,
  TrustModal,
  ViewerModal,
} from "./Modals";

type StoryApi = AppServices["api"];

function failClosedApi(overrides: Partial<StoryApi> = {}): StoryApi {
  return new Proxy(overrides as StoryApi, {
    get: (target, property) => {
      if (property in target) return target[property as keyof StoryApi];
      return () => {
        throw new Error(`Unexpected Storybook API call: ${String(property)}`);
      };
    },
  });
}

const noNetworkApi = failClosedApi();

const stillClock: AppServices["clock"] = {
  now: () => Date.parse("2026-07-23T12:00:00Z"),
  setTimeout: (callback, delay) => globalThis.setTimeout(callback, delay),
  clearTimeout: (handle) => globalThis.clearTimeout(handle),
  setInterval: () => 0 as unknown as ReturnType<typeof setInterval>,
  clearInterval: () => {},
};

const storySession = {
  id: "story-session",
  status: "running",
  turns: 4,
  title: "Storybook component audit",
  workspace: "/workspace/agentrunner",
};

const leafState = {
  sessions: [storySession],
  sessionsReady: true,
} satisfies Partial<AppState>;

function LeafFrame({
  children,
  api = noNetworkApi,
  initialState = leafState,
}: {
  children: React.ReactNode;
  api?: StoryApi;
  initialState?: Partial<AppState>;
}) {
  return (
    <StoryAppFrame
      initialState={initialState}
      services={{ api, clock: stillClock }}
    >
      {children}
      <Toasts />
    </StoryAppFrame>
  );
}

const forkReadyApi = failClosedApi({
  barriers: async () => ["bar-t3", "bar-final", "bar-t1"],
});

const trustAction = fn(async () => {});
const promptAction = fn();

const runDetails = {
  spec: "release-reviewer",
  model: "gemini-2.5-pro",
  mode: "default",
  status: "waiting",
  gen_steps: 7,
  turns: 4,
  usage: { input_tokens: 12_400, output_tokens: 2_180, billed: 14_580 },
  waiting: {
    kind: "approval",
    tool: "bash",
    args: '{"command":"git push origin main"}',
  },
  entries: [
    { kind: "llm", name: "complete" },
    { kind: "tool", name: "bash", detail: "npm test", verdict: "allow" },
    {
      kind: "tool",
      name: "bash",
      detail: "git push origin main",
      verdict: "deny",
    },
  ],
  children: [
    { session: "child-a", call_id: "review" },
    { session: "child-b", call_id: "browser-qa" },
  ],
  provider_capabilities: {
    provider: "gemini",
    input_modalities: ["Text", "Image"],
    capabilities: { thinking: true, files: true, images: true },
  },
};

function expectDialog(canvasElement: HTMLElement, name: string) {
  return expect(
    within(canvasElement).getByRole("dialog", { name }),
  ).toBeVisible();
}

const confirmAction = fn(async () => {});

const confirmModal: NonNullable<ModalKind> = {
  kind: "confirm",
  title: "Remove demo project?",
  body: "The project disappears from the sidebar, while its chats, journal, and files remain intact.",
  confirmLabel: "Remove",
  danger: true,
  details: [
    {
      icon: "files",
      title: "Files stay on disk",
      body: "No workspace contents are deleted.",
    },
    {
      icon: "terminal",
      title: "Sessions stay searchable",
      body: "Open them later from the command palette.",
    },
  ],
  note: "This Storybook action changes only isolated in-memory state.",
  onConfirm: confirmAction,
};

const modalState = {
  modal: confirmModal,
  prompt: null,
} satisfies Partial<AppState>;

function ModalsFixture({
  initialState = modalState,
}: {
  initialState?: Partial<AppState>;
}) {
  return (
    <StoryAppFrame initialState={initialState} services={{ api: noNetworkApi }}>
      <Modals />
    </StoryAppFrame>
  );
}

function StandaloneModalFixture({ startOpen = true }: { startOpen?: boolean }) {
  const [open, setOpen] = useState(startOpen);
  const opener = useRef<HTMLButtonElement>(null);
  return (
    <StoryAppFrame services={{ api: noNetworkApi }}>
      <div className="p-6">
        <button ref={opener} onClick={() => setOpen(true)}>
          Edit demo label
        </button>
        {open && (
          <Modal
            title="Edit demo label"
            onClose={() => setOpen(false)}
            returnFocus={opener.current ?? undefined}
            footer={
              <>
                <button onClick={() => setOpen(false)}>Cancel</button>
                <button className="primary" onClick={() => setOpen(false)}>
                  Save
                </button>
              </>
            }
          >
            <label className="field" htmlFor="story-modal-label">
              Label
            </label>
            <input id="story-modal-label" defaultValue="Component demo" />
          </Modal>
        )}
      </div>
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Overlays/Modals",
  component: Modals,
  render: () => <ModalsFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const dialog = canvas.getByRole("dialog", {
      name: "Remove demo project?",
    });
    await expect(dialog).toBeVisible();
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "Close dialog" }),
      ).toHaveFocus(),
    );
    await expect(canvas.getByText("Files stay on disk")).toBeVisible();
    await expect(canvas.getByText(/isolated in-memory state/)).toBeVisible();
  },
} satisfies Meta<typeof Modals>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "Close dialog" }),
      ).toHaveFocus(),
    );
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Cancel" })).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Remove" })).toHaveFocus();
    await humanPause();
    await userEvent.keyboard("{Escape}");
    await expect(
      canvas.queryByRole("dialog", { name: "Remove demo project?" }),
    ).toBeNull();
  },
};

export const PromptOverMainModal: Story = {
  render: () => (
    <ModalsFixture
      initialState={{
        modal: {
          kind: "viewer",
          title: "Generated plan",
          body: "# Demo plan\n\nReview each isolated component state.",
        },
        prompt: {
          title: "Rename artifact",
          label: "Artifact name",
          initial: "demo-plan",
          placeholder: "Artifact name",
          submitLabel: "Rename",
          onSubmit: fn(),
        },
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("dialog", { name: "Rename artifact" }),
    ).toBeVisible();
    await expect(canvas.getByDisplayValue("demo-plan")).toHaveFocus();
    await expect(
      canvas.getByRole("dialog", { name: "Generated plan" }),
    ).toBeVisible();
  },
};

export const ConfirmAction: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Remove" }));
    await expect(confirmAction).toHaveBeenCalledOnce();
    await expect(
      canvas.queryByRole("dialog", { name: "Remove demo project?" }),
    ).toBeNull();
  },
};

export const StandaloneDefault: Story = {
  render: () => <StandaloneModalFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("dialog", { name: "Edit demo label" }),
    ).toBeVisible();
    await waitFor(() =>
      expect(canvas.getByRole("textbox", { name: "Label" })).toHaveFocus(),
    );
  },
};

export const StandaloneKeyboardNavigation: Story = {
  render: () => <StandaloneModalFixture startOpen={false} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const opener = canvas.getByRole("button", { name: "Edit demo label" });
    await userEvent.click(opener);

    const field = canvas.getByRole("textbox", { name: "Label" });
    await waitFor(() => expect(field).toHaveFocus());
    const save = canvas.getByRole("button", { name: "Save" });
    save.focus();
    await userEvent.tab();
    await expect(
      canvas.getByRole("button", { name: "Close dialog" }),
    ).toHaveFocus();

    await humanPause();
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(opener).toHaveFocus());
    await expect(
      canvas.queryByRole("dialog", { name: "Edit demo label" }),
    ).toBeNull();
  },
};

// Leaf stories below intentionally render the production leaf itself. They are
// not aliases for <Modals /> and therefore keep private UI states independently
// inspectable when the router/store composition changes.

export const AgentModalDefault: Story = {
  render: () => (
    <LeafFrame>
      <AgentModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    await expectDialog(canvasElement, "Switch agent · story-session");
    await expect(
      within(canvasElement).getAllByDisplayValue(/provider: gemini/i)[0],
    ).toBeVisible();
  },
};

export const AgentModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame>
      <AgentModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const fields = canvas.getAllByRole("textbox");
    await waitFor(() => expect(fields[0]).toHaveFocus());
    await userEvent.tab();
    await expect(fields[1]).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Switch" })).toHaveFocus();
  },
};

export const AgentModalBusy: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        switchAgent: () => new Promise<void>(() => {}),
      })}
    >
      <AgentModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const action = canvas.getByRole("button", { name: "Switch" });
    await userEvent.click(action);
    await expect(action).toBeDisabled();
    await expect(action).toHaveAttribute("aria-busy", "true");
  },
};

export const AgentModalFailure: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        switchAgent: async () => {
          throw new Error("The agent spec could not be switched.");
        },
      })}
    >
      <AgentModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Switch" }));
    await expect(
      await canvas.findByText("The agent spec could not be switched."),
    ).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Switch" })).toBeEnabled();
  },
};

const safeConfirmModal: Extract<NonNullable<ModalKind>, { kind: "confirm" }> = {
  kind: "confirm",
  title: "Apply reviewed changes?",
  body: "The selected patch will be applied to the current workspace.",
  confirmLabel: "Apply",
  onConfirm: fn(async () => {}),
};

export const ConfirmModalDefault: Story = {
  render: () => (
    <LeafFrame>
      <ConfirmModal modal={confirmModal} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expectDialog(canvasElement, "Remove demo project?");
    const remove = canvas.getByRole("button", { name: "Remove" });
    await expect(remove).toHaveAttribute("data-tone", "danger");
    await expect(remove).toHaveAttribute("data-variant", "outline");
    await expect(canvas.getByText("Files stay on disk")).toBeVisible();
  },
};

export const ConfirmModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame>
      <ConfirmModal modal={confirmModal} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "Close dialog" }),
      ).toHaveFocus(),
    );
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Cancel" })).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Remove" })).toHaveFocus();
    await userEvent.tab();
    await expect(
      canvas.getByRole("button", { name: "Close dialog" }),
    ).toHaveFocus();
  },
};

export const ConfirmModalBusy: Story = {
  render: () => (
    <LeafFrame>
      <ConfirmModal
        modal={{
          ...safeConfirmModal,
          onConfirm: () => new Promise<void>(() => {}),
        }}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Apply" }));
    const apply = canvas.getByRole("button", { name: "Apply" });
    await expect(apply).toBeDisabled();
    await expect(apply).toHaveAttribute("aria-busy", "true");
  },
};

export const ConfirmModalFailure: Story = {
  render: () => (
    <LeafFrame>
      <ConfirmModal
        modal={{
          ...safeConfirmModal,
          onConfirm: async () => {
            throw new Error("The reviewed patch could not be applied.");
          },
        }}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Apply" }));
    await expect(
      await canvas.findByText("The reviewed patch could not be applied."),
    ).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Apply" })).toBeEnabled();
  },
};

export const ForkModalDefault: Story = {
  render: () => (
    <LeafFrame api={forkReadyApi}>
      <ForkModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expectDialog(canvasElement, "Continue in new session");
    await expect(
      await canvas.findByText("Latest — end of the conversation"),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Continue" }),
    ).toBeEnabled();
  },
};

export const ForkModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame api={forkReadyApi}>
      <ForkModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const earlier = await canvas.findByRole("button", {
      name: "Choose an earlier checkpoint",
    });
    earlier.focus();
    await expect(earlier).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    const checkpoint = canvas.getByTitle(
      "the checkpoint to branch the new session from",
    );
    await expect(checkpoint).toBeVisible();
    checkpoint.focus();
    await expect(checkpoint).toHaveFocus();
    await expect(
      canvas.getByRole("option", { name: "After agent step 3" }),
    ).toBeVisible();
  },
};

export const ForkModalBusy: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        barriers: async () => ["bar-final"],
        fork: () => new Promise<{ sid: string }>(() => {}),
      })}
    >
      <ForkModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const action = await canvas.findByRole("button", { name: "Continue" });
    await userEvent.click(action);
    await expect(action).toBeDisabled();
    await expect(action).toHaveAttribute("aria-busy", "true");
  },
};

export const ForkModalFailure: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        barriers: async () => ["bar-final"],
        fork: async () => {
          throw new Error("The checkpoint could not be continued.");
        },
      })}
    >
      <ForkModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Continue" }),
    );
    await expect(
      await canvas.findByText("The checkpoint could not be continued."),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Continue" }),
    ).toBeEnabled();
  },
};

export const MainModalDefault: Story = {
  render: () => (
    <LeafFrame>
      <MainModal
        modal={{
          kind: "viewer",
          title: "Generated implementation plan",
          body: "# Plan\n\n1. Extract components\n2. Verify in Chromium",
        }}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    await expectDialog(canvasElement, "Generated implementation plan");
    await expect(
      within(canvasElement).getByText(/Extract components/),
    ).toBeVisible();
  },
};

export const MainModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame>
      <MainModal modal={safeConfirmModal} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "Close dialog" }),
      ).toHaveFocus(),
    );
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Cancel" })).toHaveFocus();
  },
};

export const NewSessionModalDefault: Story = {
  render: () => (
    <LeafFrame>
      <NewSessionModal initialMessage="Build the Storybook component matrix" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expectDialog(canvasElement, "Advanced session setup");
    await expect(
      canvas.getByDisplayValue("Build the Storybook component matrix"),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Start session" }),
    ).toBeEnabled();
  },
};

export const NewSessionModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame>
      <NewSessionModal initialMessage="Keyboard-first setup" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const message = canvas.getByDisplayValue("Keyboard-first setup");
    await waitFor(() => expect(message).toHaveFocus());
    await userEvent.tab();
    await expect(
      canvas.getByPlaceholderText("Leave blank for a new scratch workspace"),
    ).toHaveFocus();
  },
};

export const NewSessionModalBusy: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        makeWorkspace: async () => ({ path: "/workspace/storybook" }),
        newSession: () => new Promise<{ sid: string }>(() => {}),
      })}
    >
      <NewSessionModal initialMessage="Start a deterministic busy session" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const action = canvas.getByRole("button", { name: "Start session" });
    await userEvent.click(action);
    await expect(action).toBeDisabled();
    await expect(action).toHaveAttribute("aria-busy", "true");
  },
};

export const NewSessionModalFailure: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        makeWorkspace: async () => ({ path: "/workspace/storybook" }),
        newSession: async () => {
          throw new Error("The session could not be created.");
        },
      })}
    >
      <NewSessionModal initialMessage="Start a deterministic failed session" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Start session" }),
    );
    await expect(
      await canvas.findByText("The session could not be created."),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Start session" }),
    ).toBeEnabled();
  },
};

export const PromptModalDefault: Story = {
  render: () => (
    <LeafFrame>
      <PromptModal
        title="Rename artifact"
        label="Artifact name"
        initial="release-notes"
        submitLabel="Rename"
        onSubmit={promptAction}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expectDialog(canvasElement, "Rename artifact");
    await expect(
      canvas.getByRole("textbox", { name: "Artifact name" }),
    ).toHaveValue("release-notes");
  },
};

export const PromptModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame>
      <PromptModal
        title="Commit changes"
        label="Commit message"
        initial="Cover modal leaves"
        submitLabel="Commit"
        onSubmit={promptAction}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const field = canvas.getByRole("textbox", { name: "Commit message" });
    await waitFor(() => expect(field).toHaveFocus());
    await userEvent.keyboard("{Enter}");
    await expect(promptAction).toHaveBeenCalledWith("Cover modal leaves");
  },
};

export const RenameModalDefault: Story = {
  render: () => (
    <LeafFrame initialState={leafState}>
      <RenameModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expectDialog(canvasElement, "Rename session");
    await expect(
      canvas.getByDisplayValue("Storybook component audit"),
    ).toBeVisible();
  },
};

export const RenameModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        rename: async () => ({ status: "renamed" }),
        sessions: async () => [storySession],
      })}
      initialState={leafState}
    >
      <RenameModal sid={storySession.id} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const field = canvas.getByDisplayValue("Storybook component audit");
    await waitFor(() => expect(field).toHaveFocus());
    await userEvent.clear(field);
    await userEvent.type(field, "Modal audit{Enter}");
    await expect(await canvas.findByText("renamed")).toBeVisible();
  },
};

export const RunDetailsModalDefault: Story = {
  render: () => (
    <LeafFrame>
      <RunDetailsModal data={runDetails} status="waiting:approval" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expectDialog(canvasElement, "Run details");
    await expect(canvas.getByText("Approval required")).toBeVisible();
    await expect(canvas.getByText("14.6K")).toBeVisible();
    await expect(canvas.getAllByText("git push origin main")[0]).toBeVisible();
  },
};

export const RunDetailsModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame>
      <RunDetailsModal data={runDetails} status="waiting:approval" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const raw = canvas.getByText("Raw run data");
    raw.focus();
    await expect(raw).toHaveFocus();
    await userEvent.click(raw);
    await waitFor(() => expect(raw.closest("details")).toHaveAttribute("open"));
    await expect(canvas.getByText(/"release-reviewer"/)).toBeVisible();
  },
};

export const RunModalDefault: Story = {
  render: () => (
    <LeafFrame>
      <RunModal initialPrompt="Run the browser verification suite" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expectDialog(canvasElement, "Start a run");
    await expect(
      canvas.getByDisplayValue("Run the browser verification suite"),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Start run" }),
    ).toBeEnabled();
  },
};

export const RunModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame>
      <RunModal initialPrompt="Keyboard run" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const prompt = canvas.getByRole("textbox", { name: "Prompt" });
    await waitFor(() => expect(prompt).toHaveFocus());
    await userEvent.tab();
    await expect(
      canvas.getByRole("textbox", { name: "Workspace" }),
    ).toHaveFocus();
  },
};

export const RunModalBusy: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        makeWorkspace: async () => ({ path: "/workspace/storybook" }),
        startRun: () => new Promise<{ runId: string }>(() => {}),
      })}
    >
      <RunModal initialPrompt="Run deterministic browser checks" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const action = canvas.getByRole("button", { name: "Start run" });
    await userEvent.click(action);
    await expect(action).toBeDisabled();
    await expect(action).toHaveAttribute("aria-busy", "true");
  },
};

export const RunModalFailure: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        makeWorkspace: async () => ({ path: "/workspace/storybook" }),
        startRun: async () => {
          throw new Error("The run could not be started.");
        },
      })}
    >
      <RunModal initialPrompt="Run deterministic failed checks" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Start run" }));
    await expect(
      await canvas.findByText("The run could not be started."),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Start run" }),
    ).toBeEnabled();
  },
};

export const TrustModalDefault: Story = {
  render: () => (
    <LeafFrame>
      <TrustModal />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expectDialog(canvasElement, "Trust workspace directory");
    await expect(
      canvas.getByRole("button", { name: "Trust directory" }),
    ).toBeDisabled();
  },
};

export const TrustModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame api={failClosedApi({ trust: trustAction })}>
      <TrustModal />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const field = canvas.getByPlaceholderText("/path/to/workspace");
    await waitFor(() => expect(field).toHaveFocus());
    await userEvent.type(field, "/workspace/agentrunner");
    await userEvent.tab();
    const trust = canvas.getByRole("button", { name: "Trust directory" });
    await expect(trust).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(trustAction).toHaveBeenCalledWith("/workspace/agentrunner");
  },
};

export const TrustModalBusy: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        trust: () => new Promise<void>(() => {}),
      })}
    >
      <TrustModal />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      canvas.getByPlaceholderText("/path/to/workspace"),
      "/workspace/agentrunner",
    );
    const action = canvas.getByRole("button", { name: "Trust directory" });
    await userEvent.click(action);
    await expect(action).toBeDisabled();
    await expect(action).toHaveAttribute("aria-busy", "true");
  },
};

export const TrustModalFailure: Story = {
  render: () => (
    <LeafFrame
      api={failClosedApi({
        trust: async () => {
          throw new Error("The workspace directory could not be trusted.");
        },
      })}
    >
      <TrustModal />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      canvas.getByPlaceholderText("/path/to/workspace"),
      "/workspace/agentrunner",
    );
    await userEvent.click(
      canvas.getByRole("button", { name: "Trust directory" }),
    );
    await expect(
      await canvas.findByText("The workspace directory could not be trusted."),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Trust directory" }),
    ).toBeEnabled();
  },
};

export const ViewerModalDefault: Story = {
  render: () => (
    <LeafFrame>
      <ViewerModal
        title="Generated plan"
        body={"# Component plan\n\n- Foundations\n- Features\n- Pages"}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    await expectDialog(canvasElement, "Generated plan");
    await expect(within(canvasElement).getByText(/Foundations/)).toBeVisible();
  },
};

export const ViewerModalKeyboardNavigation: Story = {
  render: () => (
    <LeafFrame>
      <ViewerModal title="Keyboard output" body="Read-only output" />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "Close dialog" }),
      ).toHaveFocus(),
    );
    await userEvent.tab();
    await expect(
      canvas.getByRole("button", { name: "Close dialog" }),
    ).toHaveFocus();
  },
};
