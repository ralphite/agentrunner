import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import {
  CoreSessionPlayback,
  coreSessionDemoHandlers,
} from "../demos/CoreSessionPlayback.stories";

const meta = {
  title: "CUJs/Core Session Journeys",
  component: CoreSessionPlayback,
  parameters: {
    layout: "fullscreen",
    msw: { handlers: coreSessionDemoHandlers },
  },
  args: {
    autoPlay: true,
    label: "Core session journey",
    stepLimit: 19,
  },
} satisfies Meta<typeof CoreSessionPlayback>;

export default meta;
type Story = StoryObj<typeof meta>;

async function expectCompleted(
  canvasElement: HTMLElement,
  label: string,
  total: number,
) {
  const canvas = within(canvasElement);
  const controls = canvas.getByRole("region", { name: label });
  const status = within(controls).getByRole("status");
  await waitFor(() => expect(status).toHaveTextContent("completed"), {
    timeout: 10_000,
  });
  await expect(status).toHaveTextContent(`Step ${total} / ${total}`);
  return canvas;
}

export const ConfigureNewSession: Story = {
  args: {
    label: "Configure a new session",
    stepLimit: 10,
  },
  play: async ({ canvasElement }) => {
    const canvas = await expectCompleted(
      canvasElement,
      "Configure a new session",
      10,
    );
    await expect(
      canvas.getByDisplayValue(
        "Build a deterministic Storybook demo for the core Agent Runner session journey.",
      ),
    ).toBeVisible();
  },
};

export const StartNewSession: Story = {
  args: {
    label: "Start a new session",
    stepLimit: 11,
  },
  play: async ({ canvasElement }) => {
    const canvas = await expectCompleted(
      canvasElement,
      "Start a new session",
      11,
    );
    await expect(
      canvasElement.querySelector(".session-topbar"),
    ).not.toBeNull();
    await expect(canvas.getByLabelText("Stop active turn")).toBeVisible();
  },
};

export const StreamAndPersistResponse: Story = {
  args: {
    label: "Stream and persist a response",
    stepLimit: 14,
  },
  play: async ({ canvasElement }) => {
    const canvas = await expectCompleted(
      canvasElement,
      "Stream and persist a response",
      14,
    );
    await expect(
      canvas.getByText(/while the session remains active/),
    ).toBeVisible();
  },
};

export const InspectEnvironmentAndCompletion: Story = {
  args: {
    label: "Inspect environment and completion",
    stepLimit: 16,
  },
  play: async ({ canvasElement }) => {
    const canvas = await expectCompleted(
      canvasElement,
      "Inspect environment and completion",
      16,
    );
    await expect(
      canvas.getByRole("complementary", { name: "Environment" }),
    ).toBeVisible();
    await expect(
      canvas.getByText(
        /Implemented the deterministic Core Session Playback demo/,
      ),
    ).toBeVisible();
  },
};

export const ReviewChangesAndReturn: Story = {
  args: {
    label: "Review changes and return",
    stepLimit: 19,
  },
  play: async ({ canvasElement }) => {
    const canvas = await expectCompleted(
      canvasElement,
      "Review changes and return",
      19,
    );
    await expect(
      canvas.queryByLabelText("Close changes"),
    ).not.toBeInTheDocument();
    await expect(canvas.getByLabelText("Send message")).toBeVisible();
  },
};
