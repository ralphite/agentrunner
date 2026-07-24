import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { Settings } from "./Settings";

const initialState = {
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
    {
      id: "story-active",
      status: "completed",
      turns: 4,
      title: "Review the settings experience",
      workspace: "/Users/demo/agentrunner",
    },
  ],
  sessionsReady: true,
} satisfies Partial<AppState>;

const meta = {
  title: "Pages/Settings",
  component: Settings,
  args: {
    onClose: fn(),
    initialSection: "general",
  },
  render: (args) => (
    <StoryAppFrame initialState={initialState}>
      <Settings {...args} />
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("dialog", { name: "Settings" })).toBeVisible();
    await expect(canvas.getByRole("heading", { name: "General" })).toBeVisible();
    await expect(canvas.getByRole("textbox", { name: "Search settings" })).toHaveFocus();
  },
} satisfies Meta<typeof Settings>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: {
    onClose: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const search = canvas.getByRole("textbox", { name: "Search settings" });
    await expect(search).toHaveFocus();

    await userEvent.type(search, "git");
    await expect(canvas.getByRole("heading", { name: "Git" })).toBeVisible();
    await expect(canvas.getByText("Commit message template")).toBeVisible();

    await userEvent.keyboard("{Escape}");
    await expect(args.onClose).toHaveBeenCalledTimes(1);
  },
};

export const SearchNoMatches: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(canvas.getByRole("textbox", { name: "Search settings" }), "audio output");
    await expect(canvas.getByText("No settings match")).toBeVisible();
    await expect(canvas.getByText("No general settings match “audio output”.")).toBeVisible();
  },
};

export const InitialAppearanceSection: Story = {
  args: {
    initialSection: "appearance",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("heading", { name: "Appearance" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Appearance" }),
    ).toHaveAttribute("aria-current", "true");
  },
};
