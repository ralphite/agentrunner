import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { AR } from "../api";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import type { Health } from "../types";
import { DaemonAlert } from "./DaemonAlert";

const offline: Health = {
  version: "storybook",
  daemonUp: false,
  daemonManaged: true,
  daemonExternal: false,
  manageRequested: true,
  daemonLogPath: "/tmp/agentrunner.log",
  runtimeDir: "/tmp/agentrunner",
};

const daemonStart = fn(async () => ({}));
const health = fn(async () => offline);
const api = {
  ...AR,
  daemonStart,
  health,
};

function DaemonAlertFixture({ currentHealth = offline }: { currentHealth?: Health | null }) {
  return (
    <StoryAppFrame
      initialState={{ health: currentHealth }}
      services={{ api }}
    >
      <DaemonAlert />
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Attention/DaemonAlert",
  component: DaemonAlert,
  parameters: {
    layout: "centered",
  },
  render: () => <DaemonAlertFixture />,
} satisfies Meta<typeof DaemonAlert>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardRetry: Story = {
  play: async ({ canvasElement }) => {
    daemonStart.mockClear();
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("alert")).toHaveTextContent("Daemon offline");
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    const retry = canvas.getByRole("button", { name: "Retry" });
    await expect(retry).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(daemonStart).toHaveBeenCalledOnce();
    await expect(canvas.getByRole("button", { name: "Retrying…" })).toBeDisabled();
  },
};

export const HealthyHidden: Story = {
  render: () => (
    <DaemonAlertFixture
      currentHealth={{
        ...offline,
        daemonUp: true,
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).queryByRole("alert")).not.toBeInTheDocument();
  },
};
