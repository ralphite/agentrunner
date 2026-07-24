import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { BackgroundWork } from "../types";
import type { InspectNode } from "./Subagents";
import {
  BackgroundProcessRow,
  BackgroundProcessesSection,
  SupervisionAgentsSection,
  SupervisionCloseButton,
  SupervisionLoadingState,
  SupervisionRestingState,
  SupervisionRunDetailsButton,
} from "./SupervisionParts";

const backgroundWork: BackgroundWork[] = [
  {
    handle: "agent-browser",
    tool: "spawn_agent",
    detail: "agent=browser-reviewer prompt=Review every Supervision Story in Chromium",
  },
  {
    handle: "vite",
    tool: "exec_command",
    detail: "npm run storybook -- --port 6009",
  },
];

const agents: InspectNode[] = [
  {
    call_id: "implementation",
    agent: "implementation",
    session: "story-child-implementation",
    report: { status: "running", gen_steps: 8, usage: { billed: 18_400 } },
  },
  {
    call_id: "review",
    agent: "reviewer",
    session: "story-child-review",
    report: { status: "completed", gen_steps: 4, usage: { billed: 9_200 } },
  },
];

function StatusGallery() {
  return (
    <div className="grid gap-5 p-6 md:grid-cols-2">
      <div
        className="supervision-panel session-side"
        style={{ position: "relative", inset: "auto", width: 344 }}
      >
        <SupervisionCloseButton onClose={fn()} />
        <BackgroundProcessesSection work={backgroundWork} />
        <SupervisionLoadingState />
        <SupervisionRunDetailsButton onInspect={fn()} />
      </div>
      <div
        className="supervision-panel session-side"
        style={{ position: "relative", inset: "auto", width: 344 }}
      >
        <SupervisionAgentsSection children={agents} onOpen={fn()} />
        <SupervisionRestingState />
        <SupervisionRunDetailsButton onInspect={fn()} />
      </div>
    </div>
  );
}

const meta = {
  title: "Components/Supervision/Panel Status",
  component: SupervisionLoadingState,
  render: () => <StatusGallery />,
} satisfies Meta<typeof SupervisionLoadingState>;

export default meta;
type Story = StoryObj<typeof meta>;

export const LoadingRestingAndAgents: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Checking…")).toBeVisible();
    await expect(canvas.getByText("Nothing needs you")).toBeVisible();
    await expect(canvas.getByText("Agents")).toBeVisible();
  },
};

export const BackgroundProcessItemStates: Story = {
  render: () => (
    <div
      className="supervision-panel session-side"
      style={{ position: "relative", inset: "auto", margin: "24px auto", width: 344 }}
    >
      <section className="supervision-section">
        {backgroundWork.map((work) => <BackgroundProcessRow key={work.handle} work={work} />)}
      </section>
    </div>
  ),
};

export const CloseAndRunDetailsActions: Story = {
  render: () => {
    const onClose = fn();
    const onInspect = fn();
    return (
      <div
        className="supervision-panel session-side"
        style={{ position: "relative", inset: "auto", margin: "24px auto", width: 344 }}
      >
        <SupervisionCloseButton onClose={onClose} />
        <div className="h-12" />
        <SupervisionRunDetailsButton onInspect={onInspect} />
      </div>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Hide Environment" }));
    await userEvent.click(canvas.getByRole("button", { name: "Run details" }));
  },
};
