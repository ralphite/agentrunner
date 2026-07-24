import type { Meta, StoryObj } from "@storybook/react-vite";
import { Fragment } from "react";
import { expect, within } from "storybook/test";
import {
  StatusIndicator,
  type StatusIndicatorDisplay,
  type StatusIndicatorTone,
} from "./StatusIndicator";
import { Spinner } from "./Spinner";

const tones: StatusIndicatorTone[] = [
  "neutral",
  "info",
  "success",
  "warning",
  "danger",
];
const displays: StatusIndicatorDisplay[] = ["dot", "pill", "text"];

const meta = {
  title: "Foundations/Feedback/Status and Loading",
  component: StatusIndicator,
  parameters: { layout: "centered" },
  args: {
    label: "Ready",
  },
} satisfies Meta<typeof StatusIndicator>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => <StatusIndicator label="Ready" tone="info" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const status = canvas.getByRole("status", { name: "Ready" });
    await expect(status).toHaveAttribute("data-tone", "info");
    await expect(status).toHaveAttribute("data-display", "dot");
  },
};

export const ToneAndDisplayMatrix: Story = {
  render: () => (
    <div className="grid w-[520px] max-w-[calc(100vw-32px)] grid-cols-[100px_repeat(3,minmax(0,1fr))] items-center gap-x-5 gap-y-4 p-2">
      <span />
      {displays.map((display) => (
        <b
          className="text-[11px] font-semibold uppercase tracking-[0.04em] text-dim"
          key={display}
        >
          {display}
        </b>
      ))}
      {tones.map((tone) => (
        <Fragment key={tone}>
          <span className="text-[12px] font-medium text-ink-2" key={`${tone}-label`}>
            {tone}
          </span>
          {displays.map((display) => (
            <StatusIndicator
              key={`${tone}-${display}`}
              display={display}
              label={`${tone} status`}
              tone={tone}
            />
          ))}
        </Fragment>
      ))}
    </div>
  ),
};

export const LongLabel: Story = {
  render: () => (
    <div className="grid max-w-[320px] gap-3">
      <StatusIndicator
        display="pill"
        label="Waiting for an unusually long external approval status"
        tone="warning"
      />
      <StatusIndicator
        display="text"
        label="Waiting for an unusually long external approval status"
        tone="warning"
      />
    </div>
  ),
};

export const SpinnerSizes: Story = {
  render: () => (
    <div className="flex items-center gap-6">
      <Spinner size="sm" label="Small" />
      <Spinner size="md" label="Medium" />
      <Spinner size="lg" label="Large" />
    </div>
  ),
};

export const SpinnerInlineAndStandalone: Story = {
  render: () => (
    <div className="grid w-[360px] max-w-[calc(100vw-32px)] gap-5">
      <p className="m-0 text-[13px] text-ink-2">
        Loading history <Spinner size="sm" />
      </p>
      <Spinner label="Checking agent status…" />
      <div className="rounded-[10px] border border-line bg-panel">
        <Spinner
          display="standalone"
          label="Loading session details…"
          size="lg"
        />
      </div>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("status", { name: "Loading" }),
    ).toHaveAttribute("aria-busy", "true");
    await expect(
      canvas.getByRole("status", { name: "Loading session details…" }),
    ).toHaveAttribute("data-display", "standalone");
  },
};

export const SpinnerReducedMotion: Story = {
  render: () => (
    <Spinner
      className="[&>svg]:!animate-none"
      display="standalone"
      label="Motion stops; loading remains announced"
      size="lg"
    />
  ),
};
