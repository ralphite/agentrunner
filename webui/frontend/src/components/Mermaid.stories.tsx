import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { MermaidBlock } from "./Mermaid";

const diagram = `flowchart LR
  Plan[Plan] --> Build[Build]
  Build --> Review{Review}
  Review -->|pass| Done[Done]
  Review -->|fix| Build`;

const meta = {
  title: "Components/Content/MermaidBlock",
  component: MermaidBlock,
  decorators: [
    (Story) => (
      <StoryAppFrame>
        <div className="mx-auto max-w-[720px] p-6"><Story /></div>
      </StoryAppFrame>
    ),
  ],
  args: {
    raw: diagram,
    fallback: <pre>Rendering diagram…</pre>,
  },
} satisfies Meta<typeof MermaidBlock>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole(
        "img",
        { name: "Mermaid diagram" },
        { timeout: 5_000 },
      ),
    ).toBeVisible();
  },
};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const rendered = await canvas.findByRole("img", { name: "Mermaid diagram" });
    rendered.focus();
    await expect(rendered).toHaveFocus();
  },
};

export const InvalidSourceFallback: Story = {
  args: {
    raw: "flowchart LR\n  broken[",
    fallback: <pre>flowchart LR{"\n"}  broken[</pre>,
  },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByText(/broken\[/)).toBeVisible();
  },
};
