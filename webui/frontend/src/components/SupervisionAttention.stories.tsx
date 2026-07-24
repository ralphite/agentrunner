import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { pacedUserEvent as userEvent } from "../storybook/humanPlayback";
import {
  AttentionItem,
  AttentionSection,
  type AttentionNotice,
} from "./SupervisionParts";

const notices: AttentionNotice[] = [
  { id: "approval", message: <>Approval requested <b>2</b></> },
  { id: "answer", message: "Answer requested" },
  { id: "recovery", message: "Session needs recovery" },
  { id: "crash", message: "browser-reviewer — Crashed" },
  { id: "waiting", message: "implementation — waiting for approval (exec_command)" },
  {
    id: "background",
    message:
      "Background work still running — it keeps spending tokens until the background session is stopped or finishes",
  },
  {
    id: "child",
    message: "accessibility-reviewer-with-an-intentionally-long-name — answer requested",
    targetSession: "story-child-a11y",
  },
];

const onOpenChild = fn();

function SectionSurface({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="supervision-panel session-side"
      style={{ position: "relative", inset: "auto", margin: "24px auto", width: 344 }}
    >
      {children}
    </div>
  );
}

const meta = {
  title: "Components/Supervision/Attention",
  component: AttentionSection,
  args: { notices, onOpenChild },
  render: (args) => <SectionSurface><AttentionSection {...args} /></SectionSurface>,
} satisfies Meta<typeof AttentionSection>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllNoticeTypes: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Session needs recovery")).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: /accessibility-reviewer/ }));
    await expect(args.onOpenChild).toHaveBeenCalledWith("story-child-a11y");
  },
};

export const InteractiveChildItem: Story = {
  render: () => (
    <SectionSurface>
      <section className="supervision-section">
        <AttentionItem notice={notices[6]} onOpenChild={onOpenChild} />
      </section>
    </SectionSurface>
  ),
};
