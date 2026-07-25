import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  expect,
  fn,
  waitFor,
  within,
} from "storybook/test";
import { pacedUserEvent as userEvent } from "../storybook/humanPlayback";
import type { Session } from "../types";
import { buildSession } from "../storybook/fixtures";
import {
  SidebarConnectionStatus,
  SidebarPreviewCard,
  SidebarProjectActions,
  SidebarProjectItem,
  SidebarSessionActions,
  SidebarSessionItem,
} from "./SidebarItems";

const noop = () => {};

function StorySessionActions({
  title,
  pinned = false,
  unread = false,
  archived = false,
  onTogglePin,
  onToggleArchive,
}: {
  title: string;
  pinned?: boolean;
  unread?: boolean;
  archived?: boolean;
  onTogglePin: () => void;
  onToggleArchive: () => void;
}) {
  return (
    <SidebarSessionActions
      title={title}
      pinned={pinned}
      unread={unread}
      archived={archived}
      onTogglePin={onTogglePin}
      onRename={noop}
      onToggleRead={noop}
      onToggleArchive={onToggleArchive}
    />
  );
}

function StateMatrix({ children }: { children: ReactNode }) {
  return (
    <div
      data-testid="state-matrix"
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))",
        gap: 16,
        padding: 20,
        alignItems: "start",
      }}
    >
      {children}
    </div>
  );
}

function StateCell({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <section
      aria-label={label}
      style={{
        minWidth: 0,
        border: "1px solid var(--line)",
        borderRadius: 10,
        overflow: "hidden",
        background: "var(--side)",
      }}
    >
      <div
        style={{
          padding: "7px 10px",
          borderBottom: "1px solid var(--line)",
          color: "var(--ink)",
          fontSize: 11,
          fontWeight: 650,
          letterSpacing: ".04em",
          textTransform: "uppercase",
        }}
      >
        {label}
      </div>
      {children}
    </section>
  );
}

const baseSession = buildSession({
  id: "sidebar-item",
  title: "Prepare reusable Sidebar items",
  status: "waiting",
  workspace: "/workspace/agentrunner",
});

const meta = {
  title: "Components/Navigation/Sidebar Items",
  component: SidebarSessionItem,
  parameters: {
    layout: "fullscreen",
  },
  args: {
    session: baseSession,
    title: "Prepare reusable Sidebar items",
    when: "4m ago",
    onSelect: fn(),
    onOpenContext: fn(),
    onPreview: fn(),
    onPreviewEnd: fn(),
    onDismissPreview: fn(),
    onTogglePin: fn(),
    onToggleArchive: fn(),
  },
  render: (args) => (
    <div className="sidebar" style={{ width: 300, minHeight: 170 }}>
      <div className="project-list" style={{ paddingTop: 12 }}>
        <SidebarSessionItem
          {...args}
          actions={(
            <StorySessionActions
              title={args.title}
              pinned={args.pinned}
              unread={args.unread}
              archived={args.archived}
              onTogglePin={args.onTogglePin}
              onToggleArchive={args.onToggleArchive}
            />
          )}
        />
      </div>
    </div>
  ),
} satisfies Meta<typeof SidebarSessionItem>;

export default meta;
type Story = StoryObj<typeof meta>;

interface SessionState {
  label: string;
  session: Session;
  active?: boolean;
  unread?: boolean;
  pinned?: boolean;
  archived?: boolean;
  nested?: boolean;
}

const sessionStates: SessionState[] = [
  {
    label: "Ready",
    session: buildSession({ id: "ready", title: "Ready for the next prompt", status: "waiting" }),
  },
  {
    label: "Running",
    session: buildSession({ id: "running", title: "Building the component system", status: "running" }),
  },
  {
    label: "Completed",
    session: buildSession({ id: "completed", title: "Component audit completed", status: "completed" }),
  },
  {
    label: "Paused",
    session: buildSession({ id: "paused", title: "Scheduled work paused", status: "paused" }),
  },
  {
    label: "Needs approval",
    session: buildSession({
      id: "approval",
      title: "Approve workspace access",
      status: "waiting_approval",
      attention: { approvals: 1, answers: 0 },
    }),
    unread: true,
  },
  {
    label: "Multiple actions",
    session: buildSession({
      id: "actions",
      title: "Resolve pending decisions",
      status: "waiting_approval",
      attention: { approvals: 2, answers: 1 },
    }),
    unread: true,
  },
  {
    label: "Needs answer",
    session: buildSession({
      id: "answer",
      title: "Choose a release target",
      status: "waiting_answer",
      attention: { approvals: 0, answers: 1 },
    }),
  },
  {
    label: "Failed",
    session: buildSession({ id: "failed", title: "Browser check failed", status: "crashed" }),
  },
  {
    label: "Needs recovery",
    session: buildSession({ id: "stranded", title: "Resume from checkpoint", status: "stranded" }),
  },
  {
    label: "Unread",
    session: buildSession({ id: "unread", title: "New assistant activity", status: "waiting" }),
    unread: true,
  },
  {
    label: "Current nested",
    session: buildSession({ id: "current", title: "Current project session", status: "waiting" }),
    active: true,
    nested: true,
  },
  {
    label: "Pinned",
    session: buildSession({ id: "pinned", title: "Pinned architecture review", status: "waiting" }),
    pinned: true,
  },
  {
    label: "Archived",
    session: buildSession({ id: "archived", title: "Archived release notes", status: "completed" }),
    archived: true,
  },
  {
    label: "Managed worktree",
    session: buildSession({
      id: "worktree",
      title: "Implement in isolated worktree",
      status: "waiting",
      workspace: "/Users/demo/.local/share/agentrunner/worktrees/sidebar-items",
    }),
  },
  {
    label: "Long title",
    session: buildSession({
      id: "long-title",
      title: "Investigate the exceptionally long session title that must truncate without moving status controls",
      status: "running",
    }),
  },
];

export const SessionStateMatrix: Story = {
  render: () => (
    <StateMatrix>
      {sessionStates.map((state) => (
        <StateCell key={state.label} label={state.label}>
          <div className="sidebar" style={{ width: "100%", minHeight: 72 }}>
            <div className="project-list" style={{ paddingTop: 12 }}>
              <SidebarSessionItem
                session={state.session}
                title={state.session.title || state.session.id}
                when="4m ago"
                active={state.active}
                unread={state.unread}
                pinned={state.pinned}
                archived={state.archived}
                nested={state.nested}
                onSelect={noop}
                onOpenContext={noop}
                onPreview={noop}
                onPreviewEnd={noop}
                onDismissPreview={noop}
                onTogglePin={noop}
                onToggleArchive={noop}
              />
            </div>
          </div>
        </StateCell>
      ))}
    </StateMatrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: /New assistant activity · New activity/ })).toBeVisible();
    await expect(canvas.getAllByRole("status", { name: "Session running" })).toHaveLength(2);
    await expect(canvas.getByLabelText("Worktree session")).toBeVisible();
    await expect(canvasElement.querySelector(".project-session-wrap.current.nested")).not.toBeNull();
    await expect(canvasElement.querySelector(".project-session-wrap.archived")).not.toBeNull();
    await expect(canvas.getByTitle("3 actions needed")).toHaveTextContent("3");
    const approvalRow = canvas.getByRole("button", { name: /Approve workspace access · Needs approval/ });
    await expect(approvalRow.closest(".project-session-wrap")?.querySelector(".status-dot.appr")).not.toBeNull();
    await expect(approvalRow.closest(".project-session-wrap")?.querySelector(".status-dot.unread")).toBeNull();
  },
};

export const SessionInteraction: Story = {
  name: "Mobile session actions",
  globals: {
    viewport: { value: "phone", isRotated: false },
  },
  render: (args) => {
    const rows = [
      {
        session: buildSession({
          id: "touch-running-worktree",
          title: "Keep the exceptionally long running worktree title readable beside persistent state and actions",
          status: "running",
          workspace: "/Users/demo/.local/share/agentrunner/worktrees/touch-running-worktree",
        }),
        unread: true,
      },
      {
        session: buildSession({
          id: "touch-attention",
          title: "Resolve multiple approval and answer requests without losing the attention count",
          status: "waiting_approval",
          attention: { approvals: 2, answers: 1 },
        }),
        unread: true,
      },
    ];
    return (
      <div className="sidebar" style={{ width: 320, minHeight: "100dvh" }}>
        <div className="project-list" style={{ paddingTop: 12 }}>
          {rows.map(({ session, unread }) => (
            <SidebarSessionItem
              key={session.id}
              session={session}
              title={session.title || session.id}
              unread={unread}
              onSelect={args.onSelect}
              onOpenContext={args.onOpenContext}
              onPreview={args.onPreview}
              onPreviewEnd={args.onPreviewEnd}
              onDismissPreview={args.onDismissPreview}
              onTogglePin={args.onTogglePin}
              onToggleArchive={args.onToggleArchive}
              actions={(
                <StorySessionActions
                  title={session.title || session.id}
                  unread={unread}
                  onTogglePin={args.onTogglePin}
                  onToggleArchive={args.onToggleArchive}
                />
              )}
            />
          ))}
        </div>
      </div>
    );
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const triggers = canvas.getAllByRole("button", { name: /More actions for/ });
    await expect(triggers).toHaveLength(2);
    for (const trigger of triggers) {
      const rect = trigger.getBoundingClientRect();
      await expect(rect.width).toBeGreaterThanOrEqual(44);
      await expect(rect.height).toBeGreaterThanOrEqual(44);
    }

    const runningRow = canvas.getByRole("button", {
      name: /^Keep the exceptionally long running worktree title readable/,
    }).closest<HTMLElement>(".project-session-wrap")!;
    await expect(within(runningRow).getByRole("status", { name: "Session running" })).toBeVisible();
    await expect(within(runningRow).getByLabelText("Worktree session")).toBeVisible();
    const longTitle = runningRow.querySelector<HTMLElement>(".project-session-title")!;
    await expect(longTitle.clientWidth).toBeGreaterThan(0);
    await expect(longTitle.scrollWidth).toBeGreaterThan(longTitle.clientWidth);
    await expect(canvas.getByTitle("3 actions needed")).toBeVisible();

    await userEvent.click(runningRow.querySelector(".project-session")!);
    await expect(args.onSelect).toHaveBeenCalledOnce();
    await userEvent.click(triggers[0]);
    await expect(args.onSelect).toHaveBeenCalledOnce();
    const items = canvas.getAllByRole("menuitem");
    await expect(items.map((item) => item.textContent?.trim())).toEqual([
      "Pin",
      "Rename…",
      "Mark as read",
      "Archive",
    ]);
    await waitFor(() => expect(document.activeElement).toBe(items[0]));
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(document.activeElement).toBe(triggers[0]));
    await expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
      document.documentElement.clientWidth,
    );
  },
};

export const SessionQuickActionsReveal: Story = {
  parameters: {
    pseudo: {
      hover: ".project-session-wrap",
    },
  },
  args: {
    session: buildSession({
      id: "running-worktree",
      title: "Keep running while quick actions are visible",
      status: "running",
      workspace: "/Users/demo/.local/share/agentrunner/worktrees/running-worktree",
    }),
    title: "Keep running while quick actions are visible",
    pinned: true,
    archived: true,
  },
  render: (args) => (
    <div className="pseudo-hover sidebar" style={{ width: 300, minHeight: 170 }}>
      <div className="project-list" style={{ paddingTop: 12 }}>
        <SidebarSessionItem {...args} />
      </div>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const list = canvasElement.querySelector<HTMLElement>(".project-list");
    const row = canvasElement.querySelector<HTMLElement>(".project-session-wrap");
    await expect(list).not.toBeNull();
    await expect(row).not.toBeNull();

    const rowRect = row!.getBoundingClientRect();
    const listStyle = window.getComputedStyle(list!);
    const innerWidth =
      list!.clientWidth -
      Number.parseFloat(listStyle.paddingLeft || "0") -
      Number.parseFloat(listStyle.paddingRight || "0");
    expect(Math.abs(rowRect.width - innerWidth)).toBeLessThanOrEqual(1);

    await userEvent.hover(row!);
    await waitFor(() => expect(row!.querySelector(".session-quick-actions")).toBeVisible());
    await expect(canvas.getByRole("button", { name: "Unpin Keep running while quick actions are visible" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Unarchive Keep running while quick actions are visible" })).toBeVisible();
    await expect(canvas.getByRole("status", { name: "Session running" })).toBeVisible();
    await expect(canvas.getByLabelText("Worktree session")).not.toBeVisible();
  },
};

export const SessionQuickActionsFocus: Story = {
  args: {
    ...SessionQuickActionsReveal.args,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    canvas.getByRole("button", { name: /Keep running while quick actions are visible · Running/ }).focus();
    await expect(canvas.getByRole("button", { name: "Unpin Keep running while quick actions are visible" })).toBeVisible();
    await expect(canvas.getByRole("status", { name: "Session running" })).toBeVisible();
    await expect(canvas.getByLabelText("Worktree session")).not.toBeVisible();
  },
};

const actionProps = {
  onTogglePin: noop,
  onRename: noop,
  onToggleRead: noop,
  onToggleArchive: noop,
};

export const SessionActionsStateMatrix: Story = {
  render: () => (
    <StateMatrix>
      <StateCell label="Default actions">
        <div className="ctx-menu" role="menu" style={{ position: "static", margin: 12 }}>
          <SidebarSessionActions title="Ready session" {...actionProps} />
        </div>
      </StateCell>
      <StateCell label="Pinned, unread, archived">
        <div className="ctx-menu" role="menu" style={{ position: "static", margin: 12 }}>
          <SidebarSessionActions
            title="Preserved session"
            pinned
            unread
            archived
            {...actionProps}
          />
        </div>
      </StateCell>
    </StateMatrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("menuitem", { name: "Pin" })).toBeVisible();
    await expect(canvas.getByRole("menuitem", { name: "Unpin" })).toBeVisible();
    await expect(canvas.getByRole("menuitem", { name: "Mark as read" })).toBeVisible();
    await expect(canvas.getByRole("menuitem", { name: "Unarchive" })).toBeVisible();
  },
};

const projectActionProps = {
  onTogglePin: noop,
  onReveal: noop,
  onCreateWorktree: noop,
  onRename: noop,
  onArchiveChats: noop,
  onToggleRemoved: noop,
};

function ProjectSession({
  id,
  title,
  active = false,
}: {
  id: string;
  title: string;
  active?: boolean;
}) {
  const session = buildSession({ id, title, status: "waiting" });
  return (
    <SidebarSessionItem
      session={session}
      title={title}
      nested
      active={active}
      onSelect={noop}
      onOpenContext={noop}
      onPreview={noop}
      onPreviewEnd={noop}
      onDismissPreview={noop}
      onTogglePin={noop}
      onToggleArchive={noop}
    />
  );
}

function ProjectExample({
  label,
  workspace = "/workspace/agentrunner",
  folded = false,
  removed = false,
  overflow = null,
  current = false,
}: {
  label: string;
  workspace?: string | null;
  folded?: boolean;
  removed?: boolean;
  overflow?: "more" | "less" | null;
  current?: boolean;
}) {
  const actualWorkspace = workspace ?? undefined;
  return (
    <div className="sidebar" style={{ width: "100%", minHeight: folded ? 64 : 126 }}>
      <div className="project-list" style={{ paddingTop: 8 }}>
        <SidebarProjectItem
          name={label}
          workspace={actualWorkspace}
          folded={folded}
          removed={removed}
          overflow={overflow}
          actions={(
            <SidebarProjectActions
              workspace={actualWorkspace}
              removed={removed}
              {...projectActionProps}
            />
          )}
          onToggle={noop}
          onOpenContext={noop}
          onPreview={noop}
          onPreviewEnd={noop}
          onDismissPreview={noop}
          onNewChat={noop}
          onToggleOverflow={noop}
        >
          {!folded && (
            <ProjectSession
              id={`${label}-session`}
              title={current ? "Current session beyond the row cap" : "Project session"}
              active={current}
            />
          )}
        </SidebarProjectItem>
      </div>
    </div>
  );
}

export const ProjectStateMatrix: Story = {
  render: () => (
    <StateMatrix>
      <StateCell label="Expanded">
        <ProjectExample label="AgentRunner" />
      </StateCell>
      <StateCell label="Collapsed">
        <ProjectExample label="Collapsed project" folded />
      </StateCell>
      <StateCell label="Removed">
        <ProjectExample label="Removed project" removed />
      </StateCell>
      <StateCell label="No workspace">
        <ProjectExample label="Imported sessions" workspace={null} />
      </StateCell>
      <StateCell label="Overflow hidden">
        <ProjectExample label="Large project" overflow="more" />
      </StateCell>
      <StateCell label="Overflow expanded">
        <ProjectExample label="Expanded project" overflow="less" />
      </StateCell>
      <StateCell label="Current anchor after cap">
        <ProjectExample label="Current project" current />
      </StateCell>
      <StateCell label="Long project name">
        <ProjectExample label="An exceptionally long project name that must truncate before its trailing actions" />
      </StateCell>
    </StateMatrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Collapsed project" })).toHaveAttribute("aria-expanded", "false");
    await expect(canvas.getByRole("button", { name: "AgentRunner" })).toHaveAttribute("aria-expanded", "true");
    await expect(canvas.getByRole("button", { name: "Show more sessions" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Show fewer sessions" })).toBeVisible();
    await expect(canvasElement.querySelector('[data-project-state="removed"]')).not.toBeNull();
    await expect(canvasElement.querySelector(".project-session-wrap.current.nested")).not.toBeNull();
  },
};

export const ProjectActionsStateMatrix: Story = {
  render: () => (
    <StateMatrix>
      <StateCell label="Workspace project">
        <div className="ctx-menu" role="menu" style={{ position: "static", margin: 12 }}>
          <SidebarProjectActions workspace="/workspace/agentrunner" {...projectActionProps} />
        </div>
      </StateCell>
      <StateCell label="Pinned project">
        <div className="ctx-menu" role="menu" style={{ position: "static", margin: 12 }}>
          <SidebarProjectActions pinned workspace="/workspace/agentrunner" {...projectActionProps} />
        </div>
      </StateCell>
      <StateCell label="Removed project">
        <div className="ctx-menu" role="menu" style={{ position: "static", margin: 12 }}>
          <SidebarProjectActions removed workspace="/workspace/agentrunner" {...projectActionProps} />
        </div>
      </StateCell>
      <StateCell label="No workspace">
        <div className="ctx-menu" role="menu" style={{ position: "static", margin: 12 }}>
          <SidebarProjectActions {...projectActionProps} />
        </div>
      </StateCell>
    </StateMatrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getAllByRole("menuitem", { name: "Reveal in Finder" })).toHaveLength(3);
    await expect(canvas.getByRole("menuitem", { name: "Unpin project" })).toBeVisible();
    await expect(canvas.getByRole("menuitem", { name: "Restore project" })).toBeVisible();
    await expect(canvas.getAllByRole("menuitem", { name: "Remove" })).toHaveLength(3);
  },
};

export const ProjectActionsHover: Story = {
  parameters: {
    pseudo: {
      hover: [".project-heading-row", ".project-heading"],
    },
  },
  render: () => (
    <div className="pseudo-hover" style={{ width: 320, padding: 16 }}>
      <ProjectExample label="Interactive project" />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const list = canvasElement.querySelector<HTMLElement>(".project-list");
    const heading = canvas.getByRole("button", { name: "Interactive project" });
    const row = heading.closest<HTMLElement>(".project-heading-row");
    await expect(list).not.toBeNull();
    await expect(row).not.toBeNull();

    const rowRect = row!.getBoundingClientRect();
    const listStyle = window.getComputedStyle(list!);
    const innerWidth =
      list!.clientWidth -
      Number.parseFloat(listStyle.paddingLeft || "0") -
      Number.parseFloat(listStyle.paddingRight || "0");
    expect(Math.abs(rowRect.width - innerWidth)).toBeLessThanOrEqual(1);

    await userEvent.hover(row!);
    await waitFor(() =>
      expect(row!.querySelector(".project-heading-actions")).toBeVisible(),
    );
    const more = canvas.getByRole("button", { name: "More actions for Interactive project" });
    await expect(more).toBeVisible();
    await expect(canvas.getByRole("button", { name: "New chat in Interactive project" })).toBeVisible();
    await expect(row!.querySelector(".proj-caret")).toBeVisible();
    await expect(row!.querySelector(".proj-folder")).not.toBeVisible();
  },
};

export const ProjectActionsFocusAndMenuOpen: Story = {
  render: () => (
    <div style={{ width: 320, padding: 16 }}>
      <ProjectExample label="Interactive project" />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const heading = canvas.getByRole("button", { name: "Interactive project" });
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();
    await userEvent.tab();
    await expect(heading).toHaveFocus();
    const more = canvas.getByRole("button", { name: "More actions for Interactive project" });
    await expect(more).toBeVisible();
    await userEvent.click(more);
    await expect(canvas.getByRole("menu")).toBeVisible();
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: "Pin project" })).toHaveFocus());
    await expect(more).toBeVisible();
  },
};

export const ProjectWithoutWorkspaceActions: Story = {
  render: () => (
    <div style={{ width: 320, padding: 16 }}>
      <ProjectExample label="Imported sessions" workspace={null} />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    canvas.getByRole("button", { name: "Imported sessions" }).focus();
    await expect(canvas.getByRole("button", { name: "New chat in Imported sessions" })).toBeVisible();

    await userEvent.click(canvas.getByRole("button", { name: "More actions for Imported sessions" }));
    await expect(canvas.getByRole("menu")).toBeVisible();
    await expect(canvas.queryByRole("menuitem", { name: "Reveal in Finder" })).not.toBeInTheDocument();
    await expect(canvas.queryByRole("menuitem", { name: "Create permanent worktree" })).not.toBeInTheDocument();
  },
};

export const PreviewStateMatrix: Story = {
  render: () => (
    <StateMatrix>
      <StateCell label="Project preview">
        <div style={{ padding: 12 }}>
          <SidebarPreviewCard
            kind="project"
            top={0}
            inline
            name="AgentRunner"
            pinned
            chats={8}
            workspace="/workspace/agentrunner"
          />
        </div>
      </StateCell>
      <StateCell label="Project without workspace">
        <div style={{ padding: 12 }}>
          <SidebarPreviewCard
            kind="project"
            top={0}
            inline
            name="Imported sessions"
            chats={1}
          />
        </div>
      </StateCell>
      <StateCell label="Running session">
        <div style={{ padding: 12 }}>
          <SidebarPreviewCard
            kind="session"
            top={0}
            inline
            title="Build the component demo"
            when="4m ago"
            project="AgentRunner"
            branch="storybook/components"
            status={{ text: "Running", cls: "run" }}
          />
        </div>
      </StateCell>
      <StateCell label="Workspace-less session">
        <div style={{ padding: 12 }}>
          <SidebarPreviewCard
            kind="session"
            top={0}
            inline
            title="Capture release notes"
            status={{ text: "Ready", cls: "idle" }}
          />
        </div>
      </StateCell>
      <StateCell label="Long content">
        <div style={{ padding: 12 }}>
          <SidebarPreviewCard
            kind="session"
            top={0}
            inline
            title="Investigate an exceptionally long title that must remain inside the preview card without moving metadata"
            when="23d ago"
            project="A project label that is much wider than the preview card"
            branch="feature/an-exceptionally-long-branch-name-for-preview-layout"
            status={{ text: "Needs approval", cls: "appr" }}
          />
        </div>
      </StateCell>
    </StateMatrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("/workspace/agentrunner")).toBeVisible();
    await expect(canvas.getByText("No workspace")).toBeVisible();
    await expect(canvas.getByText("storybook/components")).toBeVisible();
    await expect(canvas.getByText("No project")).toBeVisible();
  },
};

export const ConnectionStateMatrix: Story = {
  render: () => (
    <StateMatrix>
      <StateCell label="Offline restart">
        <div className="side-foot" style={{ position: "static" }}>
          <SidebarConnectionStatus state="offline" onRestart={noop} />
        </div>
      </StateCell>
    </StateMatrix>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Daemon offline — click to restart" })).toBeVisible();
  },
};
