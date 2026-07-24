import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { Lightbox } from "./Lightbox";

const images = [
  "/fixtures/architecture.svg",
  "/fixtures/browser-result.svg",
];

function imageData(label: string, background: string) {
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="1200" height="720" viewBox="0 0 1200 720">
      <rect width="1200" height="720" rx="36" fill="${background}" />
      <rect x="120" y="140" width="960" height="440" rx="28" fill="#ffffff" fill-opacity=".12" />
      <text x="600" y="340" text-anchor="middle" fill="#ffffff" font-size="60" font-family="system-ui">${label}</text>
      <text x="600" y="415" text-anchor="middle" fill="#ffffff" fill-opacity=".75" font-size="30" font-family="system-ui">AgentRunner Storybook fixture</text>
    </svg>
  `;
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}

const imageUrls: Record<string, string> = {
  [images[0]]: imageData("Component architecture", "#3451b2"),
  [images[1]]: imageData("Browser verification", "#167d61"),
};

function resolveImage(path: string) {
  return imageUrls[path] ?? imageData("Image fixture", "#5f6575");
}

function LightboxFixture({
  index: initialIndex,
  onIndex,
  ...args
}: React.ComponentProps<typeof Lightbox>) {
  const [index, setIndex] = useState(initialIndex);
  return (
    <Lightbox
      {...args}
      index={index}
      onIndex={(next) => {
        setIndex(next);
        onIndex(next);
      }}
    />
  );
}

const meta = {
  title: "Components/Media/Lightbox",
  component: Lightbox,
  parameters: {
    layout: "fullscreen",
  },
  args: {
    images,
    index: 0,
    resolve: resolveImage,
    onIndex: fn(),
    onClose: fn(),
  },
  render: (args) => <LightboxFixture {...args} />,
} satisfies Meta<typeof Lightbox>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: {
    onIndex: fn(),
    onClose: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    const dialog = page.getByRole("dialog", { name: "Image viewer" });
    await expect(dialog).toHaveFocus();

    await userEvent.keyboard("{ArrowRight}");
    await expect(page.getByText("2 / 2")).toBeVisible();
    await expect(page.getByRole("img", { name: "browser-result.svg" })).toBeVisible();
    await expect(args.onIndex).toHaveBeenCalledWith(1);

    await userEvent.keyboard("=");
    await expect(page.getByText("125%")).toBeVisible();
    await userEvent.keyboard("{Escape}");
    await expect(args.onClose).toHaveBeenCalledOnce();
  },
};

export const ZoomLimits: Story = {
  args: {
    images: [images[0]],
  },
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    const zoomOut = page.getByRole("button", { name: "Zoom out" });
    await userEvent.click(zoomOut);
    await userEvent.click(zoomOut);
    await expect(page.getByText("50%")).toBeVisible();
    await expect(zoomOut).toBeDisabled();
  },
};
