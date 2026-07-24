import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { APPEARANCE_DEFAULTS, applyAppearance } from "../theme";
import {
  FontRow as FontRowView,
  SettingsAppearance,
  ThemePreview as ThemePreviewView,
  ToggleRow as ToggleRowView,
} from "./SettingsAppearance";

const appearance = JSON.stringify({
  theme: "system",
  uiFontSize: 14,
  codeFontSize: 12,
  contrast: 50,
  diffMarkers: "color",
  reduceMotion: false,
  syntax: true,
});

const meta = {
  title: "Components/Settings/Appearance",
  component: SettingsAppearance,
  args: {
    query: "",
  },
  render: (args) => (
    <StoryAppFrame services={{ local: { "arwebui.appearance": appearance } }}>
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsAppearance {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "Appearance" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "System" })).toHaveAttribute("aria-pressed", "true");
    await expect(canvas.getByRole("switch", { name: "Reduce motion" })).toHaveAttribute("aria-checked", "false");
  },
} satisfies Meta<typeof SettingsAppearance>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "System" })).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Light" })).toHaveFocus();
    await userEvent.tab();
    const dark = canvas.getByRole("button", { name: "Dark" });
    await expect(dark).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(dark).toHaveAttribute("aria-pressed", "true");
  },
};

export const DiffMarkerSelection: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const color = canvas.getByRole("button", { name: "Color" });
    const signs = canvas.getByRole("button", { name: "+ / −" });
    await expect(color).toHaveAttribute("aria-pressed", "true");
    await expect(color).toHaveClass("on");

    await userEvent.click(signs);
    await expect(signs).toHaveAttribute("aria-pressed", "true");
    await expect(signs).toHaveClass("on");
    await expect(color).toHaveAttribute("aria-pressed", "false");
    await expect(color).not.toHaveClass("on");
  },
};

export const NoMatches: Story = {
  args: {
    query: "audio output",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No appearance settings match “audio output”.")).toBeVisible();
    await expect(canvas.queryAllByRole("button")).toHaveLength(0);
  },
};

function LeafFrame({ children }: { children: React.ReactNode }) {
  return (
    <StoryAppFrame>
      <div className="mx-auto max-w-[640px] p-6">{children}</div>
    </StoryAppFrame>
  );
}

function FontRowFixture() {
  const [value, setValue] = useState(14);
  return (
    <LeafFrame>
      <FontRowView
        label="UI font size"
        desc="Base size for interface text."
        value={value}
        min={12}
        max={18}
        onChange={setValue}
      />
    </LeafFrame>
  );
}

export const FontRow: Story = {
  render: () => <FontRowFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const increase = canvas.getByRole("button", { name: "Increase UI font size" });
    increase.focus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByText("15px")).toBeVisible();
    await userEvent.tab({ shift: true });
    const decrease = canvas.getByRole("button", { name: "Decrease UI font size" });
    await expect(decrease).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByText("14px")).toBeVisible();
  },
};

function GlobalTypeScaleFixture() {
  useEffect(() => {
    applyAppearance(APPEARANCE_DEFAULTS);
    return () => applyAppearance(APPEARANCE_DEFAULTS);
  }, []);
  return (
    <StoryAppFrame services={{ local: { "arwebui.appearance": appearance } }}>
      <div className="mx-auto grid max-w-[760px] gap-6 p-6">
        <div className="grid gap-2 rounded-xl border border-line bg-panel p-4">
          <span data-testid="ui-font-probe">Interface text follows UI font size.</span>
          <code className="mono" data-testid="code-font-probe">
            code text follows code font size
          </code>
        </div>
        <SettingsAppearance query="font size" />
      </div>
    </StoryAppFrame>
  );
}

export const GlobalTypeScaleApplication: Story = {
  render: () => <GlobalTypeScaleFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const uiProbe = canvas.getByTestId("ui-font-probe");
    const codeProbe = canvas.getByTestId("code-font-probe");
    await expect(getComputedStyle(uiProbe).fontSize).toBe("14px");
    await expect(getComputedStyle(codeProbe).fontSize).toBe("12px");

    await userEvent.click(
      canvas.getByRole("button", { name: "Increase UI font size" }),
    );
    await expect(getComputedStyle(uiProbe).fontSize).toBe("15px");
    await expect(getComputedStyle(codeProbe).fontSize).toBe("12px");

    await userEvent.click(
      canvas.getByRole("button", { name: "Increase Code font size" }),
    );
    await expect(getComputedStyle(codeProbe).fontSize).toBe("13px");
  },
};

export const ThemePreview: Story = {
  render: () => (
    <LeafFrame>
      <div className="grid grid-cols-3 gap-3" aria-label="Theme previews">
        {(["system", "light", "dark"] as const).map((theme) => (
          <figure key={theme}>
            <ThemePreviewView id={theme} />
            <figcaption className="mt-2 text-center text-sm">{theme}</figcaption>
          </figure>
        ))}
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByLabelText("Theme previews")).toBeVisible();
    await expect(canvas.getByText("system")).toBeVisible();
    await expect(canvas.getByText("light")).toBeVisible();
    await expect(canvas.getByText("dark")).toBeVisible();
  },
};

function ToggleRowFixture() {
  const [checked, setChecked] = useState(false);
  return (
    <LeafFrame>
      <ToggleRowView
        label="Reduce motion"
        desc="Minimize transitions and animations."
        checked={checked}
        onChange={setChecked}
      />
    </LeafFrame>
  );
}

export const ToggleRow: Story = {
  render: () => <ToggleRowFixture />,
  play: async ({ canvasElement }) => {
    const toggle = within(canvasElement).getByRole("switch", {
      name: "Reduce motion",
    });
    toggle.focus();
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await userEvent.keyboard(" ");
    await expect(toggle).toHaveAttribute("aria-checked", "true");
  },
};
