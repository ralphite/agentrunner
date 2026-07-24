import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { Session } from "../types";
import type { ProjectGroup as ProjectGroupModel } from "../viewModels";
import {
  ArchivedProjectGroup,
  ArchivedSessionItem,
} from "./SettingsArchivedParts";

const session: Session = {
  id: "archived-review",
  status: "completed",
  turns: 6,
  title: "Review the component system",
  workspace: "/Users/demo/agentrunner",
};

const meta = {
  title: "Components/Settings/Archived Parts",
  component: ArchivedSessionItem,
  args: {
    session,
    title: session.title!,
    onOpen: fn(),
    onUnarchive: fn(),
  },
  decorators: [
    (Story) => (
      <div className="rs-panel rs-archived mx-auto max-w-[760px] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ArchivedSessionItem>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SessionItem: Story = {};

export const SessionLifecycleMatrix: Story = {
  render: (args) => (
    <>
      {[
        { id: "ready", status: "ready", title: "Ready to continue" },
        { id: "running", status: "running", title: "Still running" },
        {
          id: "approval",
          status: "waiting_approval",
          title: "Waiting for approval",
        },
        { id: "completed", status: "completed", title: "Completed session" },
        { id: "paused", status: "paused", title: "Paused goal session" },
        { id: "failed", status: "failed", title: "Failed session" },
        {
          id: "long",
          status: "completed",
          title:
            "A very long archived session title that confirms truncation and action alignment",
        },
      ].map((state) => (
        <ArchivedSessionItem
          {...args}
          key={state.id}
          session={{
            ...session,
            id: state.id,
            status: state.status,
            title: state.title,
          }}
          title={state.title}
        />
      ))}
    </>
  ),
};

export const SessionKeyboardActions: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const open = canvas.getByRole("button", {
      name: "Open Review the component system",
    });
    open.focus();
    await expect(open).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onOpen).toHaveBeenCalledWith("archived-review");
    const restore = canvas.getByRole("button", { name: "Unarchive" });
    restore.focus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onUnarchive).toHaveBeenCalledWith("archived-review");
  },
};

const project: ProjectGroupModel = {
  key: "/Users/demo/agentrunner",
  label: "agentrunner",
  workspace: "/Users/demo/agentrunner",
  hint: "/Users/demo",
  sessions: [
    session,
    {
      ...session,
      id: "archived-a11y",
      title: "Audit keyboard navigation",
      status: "waiting_approval",
    },
  ],
};

export const ProjectGroup: Story = {
  render: (args) => (
    <ArchivedProjectGroup
      project={project}
      titleOf={(item) => item.title || item.id}
      onOpen={args.onOpen}
      onUnarchive={args.onUnarchive}
    />
  ),
};

export const WorkspaceLessGroup: Story = {
  render: (args) => (
    <ArchivedProjectGroup
      project={{
        key: "__no_workspace__",
        label: "No workspace",
        sessions: [{ ...session, id: "scratch", workspace: undefined }],
      }}
      titleOf={(item) => item.title || item.id}
      onOpen={args.onOpen}
      onUnarchive={args.onUnarchive}
    />
  ),
};
