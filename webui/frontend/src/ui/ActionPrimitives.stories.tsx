import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  ArrowLeft,
  DownloadSimple,
  Plus,
  Trash,
  X,
} from "@phosphor-icons/react";
import { expect, within } from "storybook/test";
import { Button } from "./Button";
import { IconButton } from "./IconButton";
import { IconLink } from "./IconLink";

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className="grid gap-2">
      <h2 className="text-[12px] font-semibold uppercase tracking-[0.04em] text-dim">
        {title}
      </h2>
      <div className="flex flex-wrap items-center gap-3">{children}</div>
    </section>
  );
}

const meta = {
  title: "Foundations/Actions/Button and IconButton",
  component: Button,
  parameters: {
    layout: "centered",
  },
  args: {
    children: "Action",
  },
} satisfies Meta<typeof Button>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <div className="flex items-center gap-3">
      <Button>Action</Button>
      <IconButton aria-label="Add action">
        <Plus size={16} />
      </IconButton>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Action" })).toHaveAttribute(
      "type",
      "button",
    );
    await expect(
      canvas.getByRole("button", { name: "Add action" }),
    ).toHaveAttribute("title", "Add action");
  },
};

export const ButtonSizesVariantsAndTones: Story = {
  render: () => (
    <div className="grid w-[560px] max-w-[calc(100vw-32px)] gap-5 p-2">
      {(["sm", "md", "lg"] as const).map((size) => (
        <Section key={size} title={`${size} · 24 / 32 / 40px`}>
          <Button size={size} variant="ghost">
            Ghost
          </Button>
          <Button size={size} variant="outline">
            Outline
          </Button>
          <Button size={size} variant="solid">
            Solid
          </Button>
          <Button size={size} variant="ghost" tone="danger">
            Remove
          </Button>
          <Button size={size} variant="outline" tone="danger">
            Remove
          </Button>
          <Button size={size} variant="solid" tone="danger">
            Remove
          </Button>
        </Section>
      ))}
    </div>
  ),
};

export const IconButtonSizesVariantsAndTones: Story = {
  render: () => (
    <div className="grid w-[560px] max-w-[calc(100vw-32px)] gap-5 p-2">
      {(["sm", "md", "lg"] as const).map((size) => (
        <Section key={size} title={`${size} · 24 / 32 / 40px square`}>
          <IconButton size={size} variant="ghost" aria-label={`Add ${size}`}>
            <Plus size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconButton>
          <IconButton size={size} variant="outline" aria-label={`Close ${size}`}>
            <X size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconButton>
          <IconButton
            size={size}
            variant="solid"
            aria-label={`Create ${size}`}
          >
            <Plus size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconButton>
          <IconButton
            size={size}
            variant="ghost"
            tone="danger"
            aria-label={`Remove ${size}`}
          >
            <Trash size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconButton>
          <IconButton
            size={size}
            variant="outline"
            tone="danger"
            aria-label={`Delete ${size}`}
          >
            <Trash size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconButton>
          <IconButton
            size={size}
            variant="solid"
            tone="danger"
            aria-label={`Destroy ${size}`}
          >
            <Trash size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconButton>
        </Section>
      ))}
    </div>
  ),
};

export const IconLinkSizesVariantsAndTones: Story = {
  render: () => (
    <div className="grid w-[560px] max-w-[calc(100vw-32px)] gap-5 p-2">
      {(["sm", "md", "lg"] as const).map((size) => (
        <Section key={size} title={`${size} · native link`}>
          <IconLink size={size} variant="ghost" href={`#ghost-${size}`} aria-label={`Open ghost ${size}`}>
            <DownloadSimple size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconLink>
          <IconLink size={size} variant="outline" href={`#outline-${size}`} aria-label={`Open outline ${size}`}>
            <DownloadSimple size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconLink>
          <IconLink size={size} variant="solid" href={`#solid-${size}`} aria-label={`Open solid ${size}`}>
            <DownloadSimple size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconLink>
          <IconLink size={size} variant="outline" tone="danger" href={`#danger-${size}`} aria-label={`Remove link ${size}`}>
            <Trash size={size === "sm" ? 14 : size === "md" ? 16 : 18} />
          </IconLink>
        </Section>
      ))}
    </div>
  ),
};

export const InteractionStates: Story = {
  parameters: {
    pseudo: {
      hover: '[data-story-state="hover"]',
      focusVisible: '[data-story-state="focus-visible"]',
      active: '[data-story-state="active"]',
    },
  },
  render: () => (
    <div className="grid w-[640px] max-w-[calc(100vw-32px)] gap-5 p-2">
      <Section title="Default · hover · focus-visible · active">
        <Button variant="ghost">Default</Button>
        <Button variant="ghost" data-story-state="hover">
          Hover
        </Button>
        <Button variant="ghost" data-story-state="focus-visible">
          Focus visible
        </Button>
        <Button variant="ghost" data-story-state="active">
          Active
        </Button>
      </Section>
      <Section title="Pressed · disabled · loading">
        <Button pressed>Pressed</Button>
        <Button disabled>Disabled</Button>
        <Button loading>Loading</Button>
      </Section>
      <Section title="Icon default · hover · focus-visible · active">
        <IconButton variant="ghost" aria-label="Back, default">
          <ArrowLeft size={16} />
        </IconButton>
        <IconButton
          variant="ghost"
          aria-label="Back, hover"
          data-story-state="hover"
        >
          <ArrowLeft size={16} />
        </IconButton>
        <IconButton
          variant="ghost"
          aria-label="Back, focus visible"
          data-story-state="focus-visible"
        >
          <ArrowLeft size={16} />
        </IconButton>
        <IconButton
          variant="ghost"
          aria-label="Back, active"
          data-story-state="active"
        >
          <ArrowLeft size={16} />
        </IconButton>
      </Section>
      <Section title="Icon pressed · disabled · loading">
        <IconButton pressed variant="ghost" aria-label="Back, pressed">
          <ArrowLeft size={16} />
        </IconButton>
        <IconButton disabled variant="ghost" aria-label="Close, disabled">
          <X size={16} />
        </IconButton>
        <IconButton loading variant="ghost" aria-label="Close, loading">
          <X size={16} />
        </IconButton>
      </Section>
      <Section title="Danger pressed · disabled · loading">
        <Button tone="danger" variant="outline" pressed>
          Pressed
        </Button>
        <Button tone="danger" variant="outline" disabled>
          Disabled
        </Button>
        <Button tone="danger" variant="solid" loading>
          Removing
        </Button>
      </Section>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const loading = canvas.getByRole("button", { name: "Loading" });
    await expect(loading).toBeDisabled();
    await expect(loading).toHaveAttribute("aria-busy", "true");
    await expect(
      canvas.getByRole("button", { name: "Close, loading" }),
    ).toHaveAttribute("aria-busy", "true");
    await expect(
      canvas.getByRole("button", { name: "Removing" }),
    ).toHaveAttribute("aria-busy", "true");
  },
};

export const LongLabel: Story = {
  render: () => (
    <div className="grid max-w-[320px] gap-3">
      <Button className="max-w-full">
        A deliberately long action label that preserves one-line button geometry
      </Button>
      <Button className="w-full" variant="solid">
        A full-width action with a deliberately long label
      </Button>
    </div>
  ),
};

export const LinkSemanticsAndInverseTone: Story = {
  render: () => (
    <div className="rounded-xl bg-black/85 p-5">
      <div className="flex flex-wrap items-center gap-3">
        <Button tone="inverse" variant="ghost">
          Ghost
        </Button>
        <Button tone="inverse" variant="outline">
          Outline
        </Button>
        <Button tone="inverse" variant="solid">
          Solid
        </Button>
        <IconLink
          href="#download-fixture"
          download="fixture.txt"
          tone="inverse"
          variant="outline"
          aria-label="Download fixture"
        >
          <DownloadSimple size={16} />
        </IconLink>
        <IconButton
          tone="inverse"
          variant="outline"
          aria-label="Close preview"
        >
          <X size={16} />
        </IconButton>
        <Button tone="inverse" variant="outline" disabled>
          Disabled
        </Button>
        <Button tone="inverse" variant="solid" loading>
          Loading
        </Button>
        <IconButton
          tone="inverse"
          variant="outline"
          disabled
          aria-label="Close preview, disabled"
        >
          <X size={16} />
        </IconButton>
        <IconButton
          tone="inverse"
          variant="outline"
          loading
          aria-label="Close preview, loading"
        >
          <X size={16} />
        </IconButton>
      </div>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const download = canvas.getByRole("link", { name: "Download fixture" });
    await expect(download).toHaveAttribute("download", "fixture.txt");
    await expect(download).toHaveAttribute("title", "Download fixture");
    await expect(download).toHaveAttribute("data-tone", "inverse");
    await expect(
      canvas.getByRole("button", { name: "Close preview, loading" }),
    ).toHaveAttribute("aria-busy", "true");
  },
};
