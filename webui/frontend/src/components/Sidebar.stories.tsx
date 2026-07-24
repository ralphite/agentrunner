import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fireEvent, fn, userEvent, waitFor, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import { useAppServices } from "../app/appServices";
import { SIDEBAR_MIN_WIDTH, useStore, type AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import {
  buildHealth,
  buildProjectMeta,
  buildRun,
  buildSession,
} from "../storybook/fixtures";
import { Sidebar } from "./Sidebar";

type StoryApi = AppServices["api"];

const projects = {
  "/workspace/agentrunner": buildProjectMeta({
    displayName: "AgentRunner",
    pinned: true,
    lastOpened: 1_768_478_400_000,
  }),
  "/workspace/demo": buildProjectMeta({
    displayName: "Demo lab",
    folded: false,
    lastOpened: 1_768_392_000_000,
  }),
};

const safeApi = new Proxy({
  gitBranches: async () => ({
    isRepo: true,
    current: "storybook/components",
    branches: ["main", "storybook/components"],
    dirty: 2,
    hasCommits: true,
  }),
  updateProject: async (
    workspace: string,
    patch: Partial<(typeof projects)[keyof typeof projects]>,
  ) => ({
    ...projects,
    [workspace]: {
      ...(projects[workspace as keyof typeof projects] ?? {}),
      ...patch,
    },
  }),
  openIn: async () => ({ status: "opened" }),
  makeWorktree: async (repo: string, branch: string) => ({
    path: "/workspace/storybook-worktree",
    repo,
    branch,
  }),
  daemonStart: async () => ({ status: "started" }),
} as unknown as StoryApi, {
  get: (target, property) => {
    const handler = Reflect.get(target, property);
    if (handler) return handler;
    return () => {
      throw new Error(`Unexpected Storybook API call: ${String(property)}`);
    };
  },
});

const sessions = [
  buildSession({
    id: "story-review",
    title: "Review component states",
    workspace: "/workspace/agentrunner",
    status: "waiting_approval",
    attention: { approvals: 2, answers: 0 },
    turns: 8,
  }),
  buildSession({
    id: "story-build",
    title: "Build the Demo runner",
    workspace: "/workspace/agentrunner",
    status: "running",
    turns: 5,
  }),
  buildSession({
    id: "story-polish",
    title: "Polish mobile navigation",
    workspace: "/workspace/demo",
    status: "completed",
    turns: 3,
  }),
  {
    ...buildSession({
      id: "story-notes",
      title: "Capture release notes",
      status: "completed",
      turns: 2,
    }),
    workspace: undefined,
  },
];

const initialState = {
  health: buildHealth(),
  sessions,
  sessionsReady: true,
  sessionsLoadingOlder: false,
  runs: [
    buildRun({
      id: "story-audit",
      label: "Continuous UI audit",
      kind: "drive",
      status: "running",
    }),
  ],
  currentSid: null,
  currentPage: "home",
  archived: [],
  showArchived: false,
  pinned: ["story-review"],
  unread: ["story-build"],
  renames: {},
  projects,
} satisfies Partial<AppState>;

function SidebarFixture({
  state,
}: {
  state: Partial<AppState>;
}) {
  return (
    <StoryAppFrame
      initialState={state}
      services={{ api: safeApi }}
    >
      <div className="relative min-h-screen" style={{ width: 300 }}>
        <Sidebar
          onHide={fn()}
          onNavigate={fn()}
          onOpenPalette={fn()}
          onOpenSettings={fn()}
        />
        <SidebarRouteProbe />
      </div>
    </StoryAppFrame>
  );
}

function SidebarRouteProbe() {
  const services = useAppServices();
  const currentPage = useStore((state) => state.currentPage);
  const currentSid = useStore((state) => state.currentSid);
  return (
    <output className="sr-only" aria-label="Sidebar route">
      {currentSid ?? currentPage}|{services.navigation.hash() || "home"}
    </output>
  );
}

const meta = {
  title: "Components/Navigation/Sidebar",
  component: Sidebar,
  args: {
    onHide: fn(),
    onNavigate: fn(),
    onOpenPalette: fn(),
    onOpenSettings: fn(),
  },
  render: (args) => (
    <StoryAppFrame
      initialState={initialState}
      services={{ api: safeApi }}
    >
      <div className="relative min-h-screen" style={{ width: 300 }}>
        <Sidebar {...args} />
        <SidebarRouteProbe />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("navigation", { name: "Primary" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "AgentRunner home" }))
      .toBeVisible();
    await expect(canvas.getByRole("button", { name: "Projects" }))
      .toHaveAttribute("aria-expanded", "true");
    await expect(canvas.getByText("Review component states")).toBeVisible();
    await expect(canvas.getByRole("status", { name: "Connected to daemon" }))
      .toBeVisible();
  },
} satisfies Meta<typeof Sidebar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: {
    onHide: fn(),
    onNavigate: fn(),
    onOpenPalette: fn(),
    onOpenSettings: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const scheduled = canvas.getByRole("button", { name: "Scheduled" });
    scheduled.focus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByRole("status", { name: "Sidebar route" }))
      .toHaveTextContent("scheduled|scheduled");
    await expect(args.onNavigate).toHaveBeenCalledOnce();

    const heading = canvasElement.querySelector<HTMLButtonElement>(
      ".project-heading",
    );
    await expect(heading).not.toBeNull();
    heading!.focus();
    await userEvent.keyboard("{Shift>}{F10}{/Shift}");

    const body = within(canvasElement.ownerDocument.body);
    const firstAction = body.getByRole("menuitem", { name: "Unpin project" });
    await waitFor(() => expect(firstAction).toHaveFocus());
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(heading!).toHaveFocus());
  },
};

export const ResizeHandleKeyboard: Story = {
  play: async ({ canvasElement }) => {
    const separator = within(canvasElement).getByRole("separator", { name: "Resize sidebar" });
    separator.focus();
    await userEvent.keyboard("{Home}");
    await expect(separator).toHaveAttribute("aria-valuenow", String(SIDEBAR_MIN_WIDTH));
    await userEvent.keyboard("{ArrowRight}");
    await expect(separator).toHaveAttribute("aria-valuenow", String(SIDEBAR_MIN_WIDTH + 16));
    await userEvent.keyboard("{End}");
    await expect(Number(separator.getAttribute("aria-valuenow"))).toBeGreaterThan(SIDEBAR_MIN_WIDTH + 16);
  },
};

export const ResizeHandleHover: Story = {
  parameters: {
    pseudo: {
      hover: ".sidebar-resize-handle",
    },
  },
  render: () => (
    <div className="pseudo-hover">
      <SidebarFixture state={initialState} />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const separator = within(canvasElement).getByRole("separator", {
      name: "Resize sidebar",
    });
    await userEvent.hover(separator);
    await waitFor(() =>
      expect(
        ["transparent", "rgba(0, 0, 0, 0)"],
      ).not.toContain(
        getComputedStyle(separator, "::after").backgroundColor,
      ),
    );
  },
};

export const ResizeHandleFocusVisible: Story = {
  parameters: {
    pseudo: {
      focusVisible: ".sidebar-resize-handle",
    },
  },
  play: async ({ canvasElement }) => {
    const separator = within(canvasElement).getByRole("separator", {
      name: "Resize sidebar",
    });
    separator.focus();
    await expect(separator).toHaveFocus();
    await waitFor(() =>
      expect(
        ["transparent", "rgba(0, 0, 0, 0)"],
      ).not.toContain(
        getComputedStyle(separator, "::after").backgroundColor,
      ),
    );
  },
};

export const ResizeHandleDragging: Story = {
  play: async ({ canvasElement }) => {
    const separator = within(canvasElement).getByRole("separator", {
      name: "Resize sidebar",
    });
    const body = canvasElement.ownerDocument.body;
    fireEvent.pointerDown(separator, { button: 0, clientX: 320 });
    await expect(body).toHaveClass("sidebar-resizing");
    await waitFor(() =>
      expect(
        ["transparent", "rgba(0, 0, 0, 0)"],
      ).not.toContain(
        getComputedStyle(separator, "::after").backgroundColor,
      ),
    );
    fireEvent.pointerUp(canvasElement.ownerDocument.defaultView!, {
      button: 0,
      clientX: 336,
    });
    await expect(body).not.toHaveClass("sidebar-resizing");
  },
};

export const SessionNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", {
        name: /Build the Demo runner · New activity/,
      }),
    );
    await expect(canvas.getByRole("status", { name: "Sidebar route" }))
      .toHaveTextContent("story-build|story-build");
  },
};

export const Loading: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        sessions: [],
        sessionsReady: false,
        pinned: [],
        projects: {},
        runs: [],
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("status", { name: "Loading sessions" })).toBeVisible();
    await expect(canvas.queryByText("No sessions yet")).not.toBeInTheDocument();
  },
};

export const Empty: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        sessions: [],
        sessionsReady: true,
        pinned: [],
        projects: {},
        runs: [],
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No sessions yet")).toBeVisible();
    await expect(canvas.getByText("Start a session to see it here.")).toBeVisible();
  },
};

export const ConnectionChecking: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        health: null,
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("status", { name: "Connecting to daemon" }),
    ).toBeVisible();
  },
};

export const ConnectionOfflineRestart: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        health: buildHealth({ daemonUp: false }),
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const restart = within(canvasElement).getByRole("button", {
      name: "Daemon offline — click to restart",
    });
    await expect(restart).toBeVisible();
    await userEvent.click(restart);
  },
};

export const ScheduledUnreadNotice: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        sessions: [
          ...sessions,
          buildSession({
            id: "scheduled-unread",
            title: "Scheduled audit has new activity",
            kind: "driver",
            status: "completed",
          }),
        ],
        unread: ["scheduled-unread"],
        runs: [],
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const notice = within(canvasElement).getByTitle("1 with new activity");
    await expect(notice).toBeVisible();
    await expect(notice).toHaveClass("unread");
  },
};

export const FoldedSections: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const pinned = canvas.getByRole("button", { name: "Pinned" });
    const projectsHeading = canvas.getByRole("button", { name: "Projects" });

    await userEvent.click(pinned);
    await userEvent.click(projectsHeading);
    await expect(pinned).toHaveAttribute("aria-expanded", "false");
    await expect(projectsHeading).toHaveAttribute("aria-expanded", "false");
    await expect(canvas.queryByText("Review component states")).not.toBeInTheDocument();
    await expect(canvas.queryByRole("button", { name: "AgentRunner" })).not.toBeInTheDocument();
    await expect(canvas.getByText("Capture release notes")).toBeVisible();
  },
};

export const FooterMenuOpen: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "More options" }));
    await expect(canvas.getByRole("menu")).toBeVisible();
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: /^Settings/ })).toHaveFocus());
    await expect(canvas.getByRole("menuitem", { name: /Keyboard shortcuts & help/ })).toBeVisible();
    await expect(canvas.getByRole("menuitem", { name: "Theme: system" })).toBeVisible();
  },
};

export const SessionContextMenuOpen: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const session = canvas.getByRole("button", { name: /Build the Demo runner · New activity/ });
    const row = session.closest<HTMLElement>(".project-session-wrap");
    await expect(row).not.toBeNull();

    await userEvent.hover(row!);
    await expect(canvasElement.querySelector(".session-preview")).toBeVisible();
    fireEvent.contextMenu(row!, { clientX: 180, clientY: 210 });
    await expect(canvasElement.querySelector(".session-preview")).not.toBeInTheDocument();
    await expect(canvas.getByRole("menu")).toBeVisible();
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: "Pin" })).toHaveFocus());
    await expect(canvas.getByRole("menuitem", { name: "Mark as read" })).toBeVisible();
  },
};

export const CollapsedProject: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        projects: {
          ...projects,
          "/workspace/agentrunner": buildProjectMeta({
            displayName: "AgentRunner",
            pinned: true,
            folded: true,
          }),
        },
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "AgentRunner" }))
      .toHaveAttribute("aria-expanded", "false");
    await expect(canvas.queryByText("Build the Demo runner")).not.toBeInTheDocument();
  },
};

export const RemovedProjectRecovery: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        projects: {
          ...projects,
          "/workspace/demo": buildProjectMeta({
            displayName: "Demo lab",
            removed: true,
          }),
        },
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByRole("button", { name: "Demo lab" })).not.toBeInTheDocument();
    await userEvent.click(canvas.getByRole("button", { name: "Show removed projects · 1" }));
    const restoredHeading = canvas.getByRole("button", { name: "Demo lab" });
    await expect(restoredHeading).toBeVisible();
    restoredHeading.focus();
    await userEvent.keyboard("{Shift>}{F10}{/Shift}");
    const body = within(canvasElement.ownerDocument.body);
    await expect(body.getByRole("menuitem", { name: "Restore project" })).toBeVisible();
  },
};

export const ArchivedVisibility: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        archived: ["story-polish"],
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByText("Polish mobile navigation")).not.toBeInTheDocument();
    await userEvent.click(canvas.getByRole("button", { name: "Show archived · 1" }));
    await expect(canvas.getByText("Polish mobile navigation")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Hide archived · 1" })).toBeVisible();
  },
};

export const HistoryLoading: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        sessionsLoadingOlder: true,
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("Loading older sessions…"),
    ).toBeVisible();
  },
};

const overflowSessions = Array.from({ length: 8 }, (_, index) => buildSession({
  id: `overflow-${index + 1}`,
  title: index === 7 ? "Current session beyond cap" : `Recent session ${index + 1}`,
  workspace: "/workspace/overflow",
  status: index === 7 ? "running" : "waiting",
  updatedAt: `2026-01-15T${String(12 - index).padStart(2, "0")}:00:00Z`,
}));

export const OverflowKeepsCurrentAnchor: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        sessions: overflowSessions,
        currentSid: "overflow-8",
        pinned: [],
        unread: [],
        projects: {
          "/workspace/overflow": buildProjectMeta({
            displayName: "Overflow project",
            folded: false,
          }),
        },
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Current session beyond cap")).toBeVisible();
    await expect(canvasElement.querySelector(".project-session-wrap.current")).not.toBeNull();
    await expect(canvas.getByRole("button", { name: "Show more" })).toBeVisible();
  },
};

const projectOverflowSessions = Array.from({ length: 9 }, (_, index) => buildSession({
  id: `project-overflow-${index + 1}`,
  title: `Project overflow session ${index + 1}`,
  workspace: `/workspace/project-overflow-${index + 1}`,
  status: "waiting",
  updatedAt: `2026-01-${String(15 - index).padStart(2, "0")}T12:00:00Z`,
}));

export const ProjectGroupOverflow: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        sessions: projectOverflowSessions,
        pinned: [],
        unread: [],
        projects: {},
        runs: [],
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const showAll = canvas.getByRole("button", { name: "Show all 9 projects" });
    await expect(showAll).toHaveTextContent("Show more · 1");
    await userEvent.click(showAll);
    await expect(canvas.getByRole("button", { name: "Show only the 8 most recent projects" })).toBeVisible();
    await expect(canvas.getAllByRole("button", { name: /project-overflow-/i })).toHaveLength(9);
  },
};

const workspaceLessOverflowSessions = Array.from({ length: 7 }, (_, index) => ({
  ...buildSession({
    id: `workspace-less-${index + 1}`,
    title: `Workspace-less session ${index + 1}`,
    status: "waiting",
    updatedAt: `2026-01-${String(15 - index).padStart(2, "0")}T12:00:00Z`,
  }),
  workspace: undefined,
}));

export const WorkspaceLessSessionOverflow: Story = {
  render: () => (
    <SidebarFixture
      state={{
        ...initialState,
        sessions: workspaceLessOverflowSessions,
        pinned: [],
        unread: [],
        projects: {},
        runs: [],
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const more = canvas.getByRole("button", { name: "Show more · 1" });
    await userEvent.click(more);
    await expect(canvas.getByText("Workspace-less session 7")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Show less" })).toBeVisible();
  },
};
