import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  ProgressItemRow,
  ProgressSection,
  type ProgressItem,
} from "./SupervisionParts";

const progress: ProgressItem[] = [
  { id: "done", title: "Extract production leaf components", status: "done" },
  { id: "running", title: "Review every Story in the browser", status: "running" },
  { id: "pending", title: "Record final QA evidence", status: "pending" },
  {
    id: "failed",
    title: "An intentionally long failed task title that must truncate without hiding its status icon",
    status: "failed",
  },
];

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
  title: "Components/Supervision/Progress",
  component: ProgressSection,
  args: { progress },
  render: (args) => <SectionSurface><ProgressSection {...args} /></SectionSurface>,
} satisfies Meta<typeof ProgressSection>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ChecklistLifecycle: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("1/4")).toBeVisible();
    for (const item of progress) {
      await expect(canvas.getByTitle(item.title)).toBeVisible();
    }
  },
};

export const ItemStates: Story = {
  render: () => (
    <SectionSurface>
      <section className="supervision-section">
        <div className="supervision-label">Progress item states</div>
        <div className="progress-list">
          {progress.map((item) => <ProgressItemRow key={item.id} item={item} />)}
        </div>
      </section>
    </SectionSurface>
  ),
};

export const SingleCompleted: Story = {
  args: {
    progress: [{ id: "done", title: "Browser QA complete", status: "done" }],
  },
};
