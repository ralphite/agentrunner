import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { createAppStore } from "../store";
import { createStoryAppServices } from "../storybook/appServices";
import type { Health, Session } from "../types";
import { AppRuntime } from "./AppRuntime";

const health: Health = {
  version: "storybook",
  daemonUp: true,
  daemonManaged: true,
  daemonExternal: false,
  manageRequested: false,
  daemonLogPath: "/tmp/agentrunner.log",
  runtimeDir: "/tmp/agentrunner",
};

const sessions: Session[] = [
  {
    id: "runtime-session",
    title: "Runtime fixture session",
    status: "completed",
    turns: 3,
  },
];

function RuntimeFixture() {
  const [runtime] = useState(() => {
    const base = createStoryAppServices();
    const harness = createStoryAppServices({
      api: {
        ...base.services.api,
        health: async () => health,
        sessions: async () => sessions,
        runs: async () => [],
        projects: async () => ({}),
      },
    });
    return {
      harness,
      store: createAppStore(harness.services),
    };
  });

  return (
    <AppRuntime services={runtime.harness.services} store={runtime.store} />
  );
}

const meta = {
  title: "Pages/AppRuntime",
  component: AppRuntime,
  parameters: {
    fullHeight: true,
    options: { layout: { showNav: false, showPanel: false } },
  },
  render: () => <RuntimeFixture />,
} satisfies Meta<typeof AppRuntime>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();
    await userEvent.tab();
    await expect(
      canvas.getByRole("link", { name: "Skip to conversation" }),
    ).toHaveFocus();
  },
};
