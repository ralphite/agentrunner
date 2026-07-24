import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { AppShell } from "./app/AppShell";
import { StoryAppFrame } from "./storybook/StoryAppFrame";
import { createStoryApiHandlers } from "./storybook/handlers";
import type { Health, Session } from "./types";

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
    id: "demo-review",
    title: "Review the component migration",
    status: "completed",
    turns: 4,
  },
  {
    id: "demo-storybook",
    title: "Build Storybook coverage",
    status: "running",
    turns: 2,
  },
];

const appShellApi = createStoryApiHandlers({ health, sessions });

const meta = {
  title: "Pages/AppShell",
  component: AppShell,
  parameters: {
    fullHeight: true,
    options: { layout: { showNav: false, showPanel: false } },
    msw: { handlers: appShellApi.handlers },
  },
  decorators: [
    (Story) => (
      <StoryAppFrame
        initialState={{
          health,
          sessions,
          sessionsReady: true,
          pinned: ["demo-storybook"],
          unread: ["demo-storybook"],
        }}
      >
        <Story />
      </StoryAppFrame>
    ),
  ],
} satisfies Meta<typeof AppShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // Home intentionally focuses the composer on mount. Reset to the document
    // entry point to verify the page's first keyboard stop, not the composer's
    // local tab order.
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();
    await userEvent.tab();
    await expect(
      canvas.getByRole("link", { name: "Skip to conversation" }),
    ).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    const main = canvasElement.querySelector<HTMLElement>("#main");
    await expect(main).not.toBeNull();
    await expect(main).toHaveFocus();
  },
};
