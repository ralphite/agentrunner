import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { SettingsGeneral } from "./SettingsGeneral";

const healthyState = {
  health: {
    version: "2.4.0",
    daemonUp: true,
    daemonManaged: false,
    daemonExternal: true,
    manageRequested: false,
    daemonLogPath: "/Users/demo/.local/share/agentrunner/daemon.log",
    runtimeDir: "/Users/demo/.local/share/agentrunner",
    sandboxBackend: "sandbox-exec",
    sandboxDetected: true,
  },
  sessions: [
    { id: "one", status: "completed", turns: 4, title: "First session" },
    { id: "two", status: "running", turns: 1, title: "Second session" },
  ],
  sessionsReady: true,
} satisfies Partial<AppState>;

const meta = {
  title: "Components/Settings/General",
  component: SettingsGeneral,
  args: {
    query: "",
    onReset: fn(),
  },
  render: (args) => (
    <StoryAppFrame
      initialState={healthyState}
      services={{
        local: {
          "arwebui.git": JSON.stringify({ commitTemplate: "feat: {summary}" }),
        },
      }}
    >
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsGeneral {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "General" })).toBeVisible();
    await expect(canvas.getByText("Connected to the daemon. 2 sessions loaded.")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Reset to defaults" })).toBeVisible();
  },
} satisfies Meta<typeof SettingsGeneral>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: {
    onReset: fn(),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Reset to defaults" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByText("Reset all settings to defaults?")).toBeVisible();

    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Reset" })).toHaveFocus();
    await userEvent.tab();
    const cancel = canvas.getByRole("button", { name: "Cancel" });
    await expect(cancel).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.queryByText("Reset all settings to defaults?")).not.toBeInTheDocument();
  },
};

export const DaemonUnavailable: Story = {
  render: (args) => (
    <StoryAppFrame
      initialState={{
        ...healthyState,
        health: {
          ...healthyState.health!,
          daemonUp: false,
          daemonExternal: false,
        },
        sessions: [healthyState.sessions![0]],
      }}
    >
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsGeneral {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Daemon unavailable. 1 session loaded.")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Reset to defaults" })).toBeVisible();
  },
};

export const NoMatches: Story = {
  args: {
    query: "audio output",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No general settings match “audio output”.")).toBeVisible();
    await expect(canvas.queryByRole("button")).not.toBeInTheDocument();
  },
};
