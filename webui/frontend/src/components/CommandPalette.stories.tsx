import { useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import { useStore, type AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { buildRun, buildSession } from "../storybook/fixtures";
import { humanPause } from "../storybook/humanPlayback";
import { CommandPalette } from "./CommandPalette";

type StoryApi = AppServices["api"];

const noNetworkApi = new Proxy({} as StoryApi, {
  get: (_target, property) => () => {
    throw new Error(`Unexpected Storybook API call: ${String(property)}`);
  },
});

const sessions = [
  buildSession({
    id: "story-review",
    title: "Review the Storybook migration",
    workspace: "/workspace/agentrunner",
    status: "waiting_approval",
    attention: { approvals: 2, answers: 0 },
    turns: 7,
  }),
  buildSession({
    id: "story-components",
    title: "Build reusable components",
    workspace: "/workspace/agentrunner",
    status: "running",
    turns: 4,
  }),
  buildSession({
    id: "story-demo",
    title: "Polish the interactive demo",
    workspace: "/workspace/demo",
    status: "completed",
    turns: 5,
  }),
  buildSession({
    id: "story-archived",
    title: "Archive the legacy UI",
    workspace: "/workspace/legacy",
    status: "completed",
    turns: 9,
  }),
];

const initialState = {
  sessions,
  sessionsReady: true,
  runs: [
    buildRun({
      id: "story-nightly",
      label: "Nightly component audit",
      kind: "drive",
      status: "running",
    }),
  ],
  archived: ["story-archived"],
  unread: ["story-components"],
  pinned: ["story-review"],
  renames: {},
} satisfies Partial<AppState>;

function PaletteRouteProbe() {
  const currentPage = useStore((state) => state.currentPage);
  return <output aria-label="Current route">{currentPage}</output>;
}

function PaletteFocusFixture({
  onClose,
  onOpenSettings,
}: {
  onClose: (restoreFocus?: boolean) => void;
  onOpenSettings?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const opener = useRef<HTMLButtonElement>(null);
  const close = (restoreFocus = true) => {
    onClose(restoreFocus);
    setOpen(false);
    if (restoreFocus) {
      requestAnimationFrame(() => opener.current?.focus());
    }
  };

  return (
    <StoryAppFrame initialState={initialState} services={{ api: noNetworkApi }}>
      <div className="p-6">
        <button ref={opener} onClick={() => setOpen(true)}>
          Open command palette
        </button>
        <PaletteRouteProbe />
        {open && (
          <CommandPalette onClose={close} onOpenSettings={onOpenSettings} />
        )}
      </div>
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Navigation/CommandPalette",
  component: CommandPalette,
  args: {
    onClose: fn(),
    onOpenSettings: fn(),
  },
  render: (args) => (
    <StoryAppFrame initialState={initialState} services={{ api: noNetworkApi }}>
      <CommandPalette {...args} />
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("dialog", { name: "Command palette" }),
    ).toBeVisible();
    await expect(
      canvas.getByPlaceholderText("Search sessions or run a command"),
    ).toHaveFocus();
    await expect(
      canvas.getByText("Review the Storybook migration"),
    ).toBeVisible();
    await expect(canvas.queryByText("Archive the legacy UI")).toBeNull();
  },
} satisfies Meta<typeof CommandPalette>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: {
    onClose: fn(),
    onOpenSettings: fn(),
  },
  render: (args) => <PaletteFocusFixture {...args} />,
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const opener = canvas.getByRole("button", {
      name: "Open command palette",
    });

    await userEvent.click(opener);
    const input = canvas.getByPlaceholderText(
      "Search sessions or run a command",
    );
    await expect(input).toHaveFocus();
    await userEvent.type(input, "go to scheduled");
    await humanPause();
    await userEvent.keyboard("{Enter}");
    await expect(
      canvas.getByRole("status", { name: "Current route" }),
    ).toHaveTextContent("scheduled");
    await expect(args.onClose).toHaveBeenLastCalledWith(false);
    await humanPause();

    await userEvent.click(opener);
    const reopenedInput = canvas.getByPlaceholderText(
      "Search sessions or run a command",
    );
    await expect(reopenedInput).toHaveFocus();
    await humanPause();
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(opener).toHaveFocus());
    await expect(args.onClose).toHaveBeenLastCalledWith(true);
  },
};

export const PointerSelection: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const first = canvas.getByRole("option", {
      name: /Review the Storybook migration/,
    });
    const target = canvas.getByRole("option", {
      name: /Build reusable components/,
    });
    await expect(first).toHaveAttribute("aria-selected", "true");

    await userEvent.hover(target);
    await expect(target).toHaveAttribute("aria-selected", "true");
    await expect(target).toHaveClass("sel");
    await expect(first).toHaveAttribute("aria-selected", "false");
  },
};

export const KeyboardSelection: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText(
      "Search sessions or run a command",
    );
    await expect(input).toHaveFocus();
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");

    const target = canvas.getByRole("option", {
      name: /Polish the interactive demo/,
    });
    await expect(target).toHaveAttribute("aria-selected", "true");
    await expect(target).toHaveClass("sel");
  },
};

export const SearchResultsWithArchived: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByPlaceholderText(
      "Search sessions or run a command",
    );
    await userEvent.type(input, "legacy");

    await expect(canvas.getByText("Archived")).toBeVisible();
    const archived = canvas.getByRole("option", {
      name: /Archive the legacy UI/,
    });
    await expect(archived).toBeVisible();
    await expect(archived).toHaveAttribute("aria-selected", "true");
    await expect(archived.querySelector(".status-dot")).toHaveStyle({
      visibility: "hidden",
    });
  },
};

export const NoMatches: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      canvas.getByPlaceholderText("Search sessions or run a command"),
      "no-result-can-match-this-query",
    );
    await expect(canvas.getByText("No matches")).toBeVisible();
    await expect(canvas.queryByRole("option")).not.toBeInTheDocument();
  },
};

const attentionSessions = Array.from({ length: 10 }, (_, index) =>
  buildSession({
    id: `attention-${String(index + 1).padStart(2, "0")}`,
    title: `Attention session ${index + 1}`,
    workspace: "/workspace/attention",
    status: "waiting_approval",
    attention: { approvals: 1, answers: 0 },
  }),
);

export const AttentionOverflow: Story = {
  render: (args) => (
    <StoryAppFrame
      initialState={{
        ...initialState,
        sessions: attentionSessions,
        archived: [],
        unread: [],
        pinned: [],
        runs: [],
      }}
      services={{ api: noNetworkApi }}
    >
      <CommandPalette {...args} />
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Needs attention")).toBeVisible();
    await expect(
      canvas.getByRole("option", { name: /Attention session 1/ }),
    ).toBeVisible();
    await expect(canvas.getAllByText("Commands")).toHaveLength(1);
  },
};
