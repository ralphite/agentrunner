import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { SettingsConfiguration } from "./SettingsConfiguration";

const configuredState = {
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
} satisfies Partial<AppState>;

const unavailableState = {
  health: {
    ...configuredState.health,
    daemonUp: false,
    daemonExternal: false,
    sandboxDetected: false,
  },
} satisfies Partial<AppState>;

const meta = {
  title: "Components/Settings/Configuration",
  component: SettingsConfiguration,
  args: {
    query: "",
  },
  render: (args) => (
    <StoryAppFrame initialState={configuredState}>
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsConfiguration {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "Configuration" })).toBeVisible();
    await expect(canvas.getByText("External (shared)")).toBeVisible();
    await expect(canvas.getByText(/sandbox-exec — detected\./)).toBeVisible();
  },
} satisfies Meta<typeof SettingsConfiguration>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: {
    query: "approval",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "Configuration" })).toBeVisible();
    await expect(canvas.getByText("Approval policy & sandbox")).toBeVisible();
    await expect(canvas.queryAllByRole("button")).toHaveLength(0);
    await expect(canvas.queryAllByRole("link")).toHaveLength(0);
  },
};

export const DaemonUnavailable: Story = {
  render: (args) => (
    <StoryAppFrame initialState={unavailableState}>
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsConfiguration {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Unavailable")).toBeVisible();
    await expect(canvas.getByText(/sandbox-exec — not detected/)).toBeVisible();
  },
};

export const LoadingUnknown: Story = {
  render: (args) => (
    <StoryAppFrame>
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsConfiguration {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("unknown", { selector: "dd" })).toBeVisible();
    await expect(canvas.getAllByText("—", { selector: "dd" })).toHaveLength(3);
  },
};

export const LongPaths: Story = {
  render: (args) => (
    <StoryAppFrame
      initialState={{
        health: {
          ...configuredState.health,
          daemonLogPath:
            "/Users/demo/.local/share/agentrunner/a/very/deep/runtime/location/with-a-deliberately-long-daemon-log-file-name.log",
          runtimeDir:
            "/Users/demo/.local/share/agentrunner/a/very/deep/runtime/location/that-must-wrap-without-expanding-settings",
        },
      }}
    >
      <div className="mx-auto max-w-[440px] p-6">
        <SettingsConfiguration {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(/deliberately-long-daemon-log-file-name\.log/),
    ).toBeVisible();
    await expect(
      canvas.getByText(/that-must-wrap-without-expanding-settings/),
    ).toBeVisible();
  },
};

export const NoMatches: Story = {
  args: {
    query: "audio output",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No configuration matches “audio output”.")).toBeVisible();
    await expect(canvas.queryByRole("button")).not.toBeInTheDocument();
  },
};
