import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import {
  CodeBlock as CodeBlockView,
  Markdown,
  MdImage as MdImageView,
} from "./Markdown";

const richText = `# Delivery summary

The component system is **ready for review** with [documentation](https://example.com).

- Deterministic fixtures
- Keyboard coverage

| Gate | Result |
| --- | --- |
| Storybook | Passed |
| Accessibility | Passed |

> Replaying the same scenario should produce the same screen.

\`\`\`ts
const state = { phase: "completed", checks: 3 };
console.log(state);
\`\`\`

Inline math: $a^2 + b^2 = c^2$.`;

const meta = {
  title: "Components/Content/Markdown",
  component: Markdown,
  decorators: [
    (Story) => (
      <StoryAppFrame initialState={{ currentSid: "story-session" }}>
        <div className="mx-auto max-w-[720px] p-6"><Story /></div>
      </StoryAppFrame>
    ),
  ],
  args: {
    text: richText,
    sid: "story-session",
  },
} satisfies Meta<typeof Markdown>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();
    await userEvent.tab();
    await expect(canvas.getByRole("link", { name: "documentation" })).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: /Wrap/ })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByRole("button", { name: /Wrap/ })).toHaveAttribute("aria-pressed", "true");
  },
};

export const UntrustedHtml: Story = {
  args: {
    text: `<script>alert("no")</script>\n\n<img src=x onerror=alert(1)>\n\nSafe text remains visible.`,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Safe text remains visible.")).toBeVisible();
    await expect(canvasElement.querySelector("script")).toBeNull();
  },
};

export const CodeBlock: Story = {
  render: () => (
    <CodeBlockView
      raw={'const state = { phase: "completed" };'}
      lang="ts"
      className="language-ts"
    >
      {'const state = { phase: "completed" };'}
    </CodeBlockView>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const code = canvas.getByLabelText("ts code block");
    await expect(code).toBeVisible();
    const wrap = canvas.getByRole("button", { name: /Wrap/ });
    wrap.focus();
    await userEvent.keyboard("{Enter}");
    await expect(wrap).toHaveAttribute("aria-pressed", "true");
  },
};

const openImage = fn();
const pixel =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='320' height='180'%3E%3Crect width='320' height='180' fill='%230169cc'/%3E%3C/svg%3E";

export const MdImage: Story = {
  render: () => (
    <MdImageView
      sid="story-session"
      src={pixel}
      alt="Core Session browser evidence"
      onOpen={openImage}
    />
  ),
  play: async ({ canvasElement }) => {
    const image = within(canvasElement).getByRole("img", {
      name: "Core Session browser evidence",
    });
    await userEvent.click(image);
    await expect(openImage).toHaveBeenCalledWith(pixel);
  },
};
