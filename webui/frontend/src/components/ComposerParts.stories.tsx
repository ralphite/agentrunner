import { useMemo, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { createStoryApiHandlers } from "../storybook/handlers";
import { SLASH } from "./slash";
import {
  AccessPicker,
  AddMenu,
  AssistActions,
  AttachmentChip,
  AttachmentList,
  BranchPicker,
  DeliveryModeControl,
  FileMentionMenu,
  GoalOptions,
  ModelPicker,
  ProjectPicker,
  RunLocationPicker,
  SlashCommandMenu,
  SubmitButton,
  type ComposerAttachment,
} from "./ComposerParts";

function StorySurface({ children }: { children?: React.ReactNode }) {
  return (
    <div className="cx cx-home mx-auto flex min-h-[360px] w-full max-w-3xl items-end p-8">
      <div className="cx-card relative w-full overflow-visible">
        {children}
      </div>
    </div>
  );
}

const meta = {
  title: "Components/Input/Composer Parts",
  component: StorySurface,
  parameters: {
    layout: "fullscreen",
    msw: {
      handlers: createStoryApiHandlers().groups.workspace,
    },
  },
} satisfies Meta<typeof StorySurface>;

export default meta;
type Story = StoryObj<typeof meta>;

const body = (canvasElement: HTMLElement) =>
  within(canvasElement.ownerDocument.body);

async function openPopover(
  canvasElement: HTMLElement,
  name: string | RegExp,
) {
  const canvas = within(canvasElement);
  await userEvent.click(canvas.getByRole("button", { name }));
}

const projects = [
  {
    workspace: "/workspace/agentrunner",
    label: "agentrunner",
    subtitle: "~/workspace",
    active: true,
  },
  {
    workspace: "/workspace/handa",
    label: "handa",
    subtitle: "~/workspace",
  },
  {
    workspace: "/workspace/docs",
    label: "docs",
    subtitle: "~/workspace",
  },
];

function ProjectPickerHarness({
  initialPage = "projects",
  initialQuery = "",
  selected = true,
  items = projects,
  triggerLabel,
}: {
  initialPage?: "projects" | "new";
  initialQuery?: string;
  selected?: boolean;
  items?: typeof projects;
  triggerLabel?: string;
}) {
  const [page, setPage] = useState(initialPage);
  const [query, setQuery] = useState(initialQuery);
  const matches = useMemo(
    () =>
      items.filter((item) =>
        item.label.toLowerCase().includes(query.toLowerCase()),
      ),
    [items, query],
  );
  return (
    <div className="cx-env-strip">
      <ProjectPicker
        label={selected ? triggerLabel || "agentrunner" : "Select project"}
        query={query}
        page={page}
        projects={matches}
        selected={selected}
        onOpen={() => {}}
        onQueryChange={setQuery}
        onSelect={fn()}
        onShowNew={() => setPage("new")}
        onBack={() => setPage("projects")}
        onStartScratch={fn()}
        onUseExisting={fn()}
        onClear={fn()}
      />
    </div>
  );
}

export const ProjectPickerRecent: Story = {
  render: () => (
    <StorySurface>
      <ProjectPickerHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "agentrunner");
    const page = body(canvasElement);
    const dialog = page.getByRole("dialog", { name: "Project picker" });
    await expect(dialog).toBeVisible();
    await expect(
      within(dialog).getByRole("button", { name: /agentrunner/ }),
    ).toBeVisible();
  },
};

export const ProjectPickerFiltered: Story = {
  render: () => (
    <StorySurface>
      <ProjectPickerHarness initialQuery="handa" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "agentrunner");
    const page = body(canvasElement);
    await expect(page.getByRole("button", { name: /handa/ })).toBeVisible();
    await expect(page.queryByRole("button", { name: /^docs/ })).not.toBeInTheDocument();
  },
};

export const ProjectPickerNoResults: Story = {
  render: () => (
    <StorySurface>
      <ProjectPickerHarness initialQuery="missing" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "agentrunner");
    await expect(body(canvasElement).getByText("No projects found")).toBeVisible();
  },
};

export const ProjectPickerNewProject: Story = {
  render: () => (
    <StorySurface>
      <ProjectPickerHarness initialPage="new" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "agentrunner");
    const page = body(canvasElement);
    await expect(page.getByText("New project")).toBeVisible();
    await expect(page.getByRole("button", { name: /Start from scratch/ })).toBeVisible();
  },
};

export const ProjectPickerNoSelection: Story = {
  render: () => (
    <StorySurface>
      <ProjectPickerHarness selected={false} />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Select project");
    await expect(
      body(canvasElement).getByRole("button", {
        name: /Don't work in a project/,
      }),
    ).toBeVisible();
  },
};

export const ProjectPickerPageFlowKeyboard: Story = {
  render: () => (
    <StorySurface>
      <ProjectPickerHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = body(canvasElement);
    const trigger = canvas.getByRole("button", { name: "agentrunner" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(page.getByRole("textbox", { name: "Search projects" })).toHaveFocus(),
    );

    page.getByRole("button", { name: "New project" }).focus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(page.getByRole("button", { name: "Back to projects" })).toHaveFocus(),
    );

    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(page.getByRole("textbox", { name: "Search projects" })).toHaveFocus(),
    );
    await userEvent.keyboard("{Escape}");
    await expect(trigger).toHaveFocus();
  },
};

const longProjectLabel =
  "agentrunner-with-an-intentionally-long-project-name-that-must-truncate";
const manyProjects = Array.from({ length: 14 }, (_, index) => ({
  workspace: `/workspace/very/deep/team/repository-${index}`,
  label: index === 0 ? longProjectLabel : `repository-${index}`,
  subtitle: "~/workspace/very/deep/team",
  active: index === 0,
}));

export const ProjectPickerLongOverflow: Story = {
  render: () => (
    <StorySurface>
      <ProjectPickerHarness
        items={manyProjects}
        triggerLabel={longProjectLabel}
      />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, longProjectLabel);
    const page = body(canvasElement);
    const dialog = page.getByRole("dialog", { name: "Project picker" });
    const list = dialog.querySelector<HTMLElement>(".cx-project-list");
    const title = within(dialog).getByText(longProjectLabel);
    await expect(title).toBeVisible();
    await waitFor(() => expect(list!.scrollHeight).toBeGreaterThan(list!.clientHeight));
    await expect(title.scrollWidth).toBeGreaterThan(title.clientWidth);
  },
};

function RunLocationHarness({
  kind = "chat",
  initial = "worktree",
  unavailable,
}: {
  kind?: "chat" | "background";
  initial?: "worktree" | "local";
  unavailable?: string;
}) {
  const [location, setLocation] = useState(initial);
  return (
    <div className="cx-env-strip">
      <RunLocationPicker
        kind={kind}
        location={location}
        worktreeUnavailableReason={unavailable}
        onSelect={setLocation}
        onUnavailableWorktree={fn()}
      />
    </div>
  );
}

export const RunLocationWorktree: Story = {
  render: () => (
    <StorySurface>
      <RunLocationHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, /New worktree/);
    await expect(
      body(canvasElement).getByRole("menuitem", { name: /New worktree/ }),
    ).toBeVisible();
  },
};

export const RunLocationLocal: Story = {
  render: () => (
    <StorySurface>
      <RunLocationHarness initial="local" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Local");
    await expect(
      body(canvasElement).getByRole("menuitem", { name: /^Local/ }),
    ).toBeVisible();
  },
};

export const RunLocationBackground: Story = {
  render: () => (
    <StorySurface>
      <RunLocationHarness kind="background" initial="local" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", {
        name: "Background · Local",
      }),
    ).toBeVisible();
  },
};

export const RunLocationBackgroundWorktree: Story = {
  render: () => (
    <StorySurface>
      <RunLocationHarness kind="background" initial="worktree" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", {
        name: "Background · New worktree",
      }),
    ).toBeVisible();
  },
};

export const RunLocationUnavailable: Story = {
  render: () => (
    <StorySurface>
      <RunLocationHarness
        initial="local"
        unavailable="Repo has no commits yet — commit one first"
      />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Local");
    await expect(
      body(canvasElement).getByText(
        "Repo has no commits yet — commit one first",
      ),
    ).toBeVisible();
  },
};

export const RunLocationKeyboardSelection: Story = {
  render: () => (
    <StorySurface>
      <RunLocationHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = body(canvasElement);
    const trigger = canvas.getByRole("button", { name: "New worktree" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(page.getByRole("menuitem", { name: /New worktree/ })).toHaveFocus(),
    );
    await userEvent.keyboard("{ArrowDown}{Enter}");
    await waitFor(() =>
      expect(canvas.getByRole("button", { name: "Local" })).toHaveFocus(),
    );
  },
};

function BranchPickerHarness({
  isRepo = true,
  location = "worktree",
  initialQuery = "",
  branches = ["main", "storybook/components", "release/v2"],
  narrow = false,
}: {
  isRepo?: boolean;
  location?: "worktree" | "local";
  initialQuery?: string;
  branches?: string[];
  narrow?: boolean;
}) {
  const [query, setQuery] = useState(initialQuery);
  const [label, setLabel] = useState("storybook/components");
  const filtered = branches.filter((branch) =>
    branch.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <div className="cx-env-strip">
      <BranchPicker
        label={isRepo ? label : "No branch"}
        narrow={narrow}
        isRepo={isRepo}
        location={location}
        dirty={location === "local" ? 3 : 0}
        query={query}
        branches={filtered}
        totalBranches={branches.length}
        onOpen={() => {}}
        onQueryChange={setQuery}
        onSelect={(branch, close) => {
          setLabel(branch);
          close();
        }}
      />
    </div>
  );
}

export const BranchPickerWorktree: Story = {
  render: () => (
    <StorySurface>
      <BranchPickerHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "storybook/components");
    const page = body(canvasElement);
    await expect(page.getByText("Start worktree from")).toBeVisible();
    await userEvent.click(page.getByRole("button", { name: /^main/ }));
    await expect(
      within(canvasElement).getByRole("button", { name: "main" }),
    ).toBeVisible();
  },
};

export const BranchPickerLocalDirty: Story = {
  render: () => (
    <StorySurface>
      <BranchPickerHarness location="local" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "storybook/components");
    await expect(body(canvasElement).getByText("Local branch · 3 uncommitted")).toBeVisible();
  },
};

export const BranchPickerNoMatches: Story = {
  render: () => (
    <StorySurface>
      <BranchPickerHarness initialQuery="missing" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "storybook/components");
    await expect(body(canvasElement).getByText("No branches found")).toBeVisible();
  },
};

export const BranchPickerEmptyRepo: Story = {
  render: () => (
    <StorySurface>
      <BranchPickerHarness branches={[]} />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "storybook/components");
    await expect(body(canvasElement).getByText("No branches yet")).toBeVisible();
  },
};

export const BranchPickerDisabled: Story = {
  render: () => (
    <StorySurface>
      <BranchPickerHarness isRepo={false} />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "No branch" }),
    ).toBeDisabled();
  },
};

export const BranchPickerLongNarrowOverflow: Story = {
  render: () => (
    <StorySurface>
      <BranchPickerHarness
        narrow
        branches={[
          "storybook/components",
          ...Array.from(
            { length: 18 },
            (_, index) =>
              `feature/team/component-state-${index}-with-a-very-long-suffix-that-keeps-going-across-a-narrow-composer-control`,
          ),
        ]}
      />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "storybook/components");
    const page = body(canvasElement);
    const panel = page.getByRole("dialog", { name: "Branch picker" });
    const longBranch = page.getByText(
      "feature/team/component-state-0-with-a-very-long-suffix-that-keeps-going-across-a-narrow-composer-control",
    );
    await expect(longBranch).toBeVisible();
    await waitFor(() =>
      expect(panel.scrollHeight).toBeGreaterThan(panel.clientHeight),
    );
  },
};

export const BranchPickerDialogKeyboard: Story = {
  render: () => (
    <StorySurface>
      <BranchPickerHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = body(canvasElement);
    const trigger = canvas.getByRole("button", { name: "storybook/components" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(page.getByLabelText("Search branches")).toHaveFocus(),
    );
    await userEvent.tab();
    await expect(page.getByRole("button", { name: /^main/ })).toHaveFocus();
    await userEvent.keyboard("{Escape}");
    await expect(trigger).toHaveFocus();
  },
};

const attachments: ComposerAttachment[] = [
  {
    path: "/runtime/storybook/screenshot.png",
    name: "screenshot.png",
    isImage: true,
  },
  {
    path: "/runtime/storybook/requirements.md",
    name: "requirements.md",
    isImage: false,
  },
];

export const AttachmentImageAndFile: Story = {
  render: () => (
    <StorySurface>
      <AttachmentList attachments={attachments} onRemove={fn()} />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("button", {
        name: "Remove attachment screenshot.png",
      }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", {
        name: "Remove attachment requirements.md",
      }),
    ).toBeVisible();
  },
};

export const AttachmentSingleImage: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-atts flex flex-wrap gap-[6px] pt-[12px] px-[14px]">
        <AttachmentChip attachment={attachments[0]} onRemove={fn()} />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", {
        name: "Remove attachment screenshot.png",
      }),
    );
  },
};

export const AttachmentEmpty: Story = {
  render: () => (
    <StorySurface>
      <AttachmentList attachments={[]} onRemove={fn()} />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).queryByLabelText("Attachments"),
    ).not.toBeInTheDocument();
  },
};

export const AttachmentLongAndWrapping: Story = {
  render: () => (
    <StorySurface>
      <AttachmentList
        attachments={Array.from({ length: 8 }, (_, index) => ({
          path: `/runtime/storybook/report-${index}.md`,
          name:
            index === 0
              ? "an-extremely-long-review-artifact-filename-that-must-truncate.md"
              : `report-${index}.md`,
          isImage: false,
        }))}
        onRemove={fn()}
      />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const group = canvas.getByRole("group", { name: "Attachments" });
    const longName = canvas.getByText(
      "an-extremely-long-review-artifact-filename-that-must-truncate.md",
    );
    await expect(canvas.getAllByTitle("Remove attachment")).toHaveLength(8);
    await expect(longName.scrollWidth).toBeGreaterThan(longName.clientWidth);
    await expect(group.getBoundingClientRect().height).toBeGreaterThan(40);
  },
};

export const FileMentionResults: Story = {
  render: () => (
    <StorySurface>
      <FileMentionMenu
        query="Comp"
        known
        files={[
          "src/components/Composer.tsx",
          "src/components/ComposerParts.tsx",
        ]}
        activeIndex={0}
        onActiveIndexChange={fn()}
        onSelect={fn()}
      />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("option", {
        name: "src/components/Composer.tsx",
      }),
    ).toHaveAttribute("aria-selected", "true");
  },
};

export const FileMentionNoMatches: Story = {
  render: () => (
    <StorySurface>
      <FileMentionMenu
        query="missing"
        known
        files={[]}
        activeIndex={0}
        onActiveIndexChange={fn()}
        onSelect={fn()}
      />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByText("No matching files")).toBeVisible();
  },
};

export const FileMentionUnknownWorkspace: Story = {
  render: () => (
    <StorySurface>
      <FileMentionMenu
        query=""
        known={false}
        files={[]}
        activeIndex={0}
        onActiveIndexChange={fn()}
        onSelect={fn()}
      />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByText("Workspace unknown")).toBeVisible();
  },
};

function FileMentionHarness({
  files,
}: {
  files: string[];
}) {
  const [activeIndex, setActiveIndex] = useState(0);
  return (
    <FileMentionMenu
      query="src"
      known
      files={files}
      activeIndex={activeIndex}
      onActiveIndexChange={setActiveIndex}
      onSelect={fn()}
    />
  );
}

const longMentionFiles = Array.from(
  { length: 14 },
  (_, index) =>
    `src/features/component-state-${index}/an-intentionally-long-file-name-for-overflow.tsx`,
);

export const FileMentionPointerAndOverflow: Story = {
  render: () => (
    <StorySurface>
      <FileMentionHarness files={longMentionFiles} />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const listbox = canvas.getByRole("listbox", { name: "Workspace files" });
    const second = canvas.getByRole("option", { name: longMentionFiles[1] });
    await userEvent.hover(second);
    await expect(second).toHaveAttribute("aria-selected", "true");
    await waitFor(() =>
      expect(listbox.scrollHeight).toBeGreaterThan(listbox.clientHeight),
    );
    await expect(second).toHaveTextContent(longMentionFiles[1]);
  },
};

export const SlashCommandResults: Story = {
  render: () => (
    <StorySurface>
      <SlashCommandMenu
        commands={SLASH.filter((command) =>
          ["goal", "loop", "bestof"].includes(command.name),
        )}
        activeIndex={1}
        onActiveIndexChange={fn()}
        onSelect={fn()}
      />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("option", { name: /\/loop/ }),
    ).toHaveAttribute("aria-selected", "true");
  },
};

function SlashCommandHarness() {
  const [activeIndex, setActiveIndex] = useState(0);
  return (
    <SlashCommandMenu
      commands={SLASH}
      activeIndex={activeIndex}
      onActiveIndexChange={setActiveIndex}
      onSelect={fn()}
    />
  );
}

export const SlashCommandPointerAndOverflow: Story = {
  render: () => (
    <StorySurface>
      <SlashCommandHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const listbox = canvas.getByRole("listbox", { name: "Slash commands" });
    const model = canvas.getByRole("option", { name: /\/model/ });
    await userEvent.hover(model);
    await expect(model).toHaveAttribute("aria-selected", "true");
    await waitFor(() =>
      expect(listbox.scrollHeight).toBeGreaterThan(listbox.clientHeight),
    );
  },
};

function AddMenuHarness({
  initialPage = "root",
  isSession = false,
  goalMode = false,
  planMode = false,
  kind = "chat",
  persona = "dev",
}: {
  initialPage?: "root" | "advanced" | "agent";
  isSession?: boolean;
  goalMode?: boolean;
  planMode?: boolean;
  kind?: "chat" | "background";
  persona?: string;
}) {
  const [page, setPage] = useState(initialPage);
  return (
    <div className="cx-bar">
      <AddMenu
        page={page}
        isSession={isSession}
        goalMode={goalMode}
        planMode={planMode}
        kind={kind}
        persona={persona}
        onOpen={() => {}}
        onPageChange={setPage}
        onPickFiles={fn()}
        onToggleGoal={fn()}
        onTogglePlan={fn()}
        onStartLoop={fn()}
        onStartBest={fn()}
        onToggleBackground={fn()}
        onSelectPersona={fn()}
        onEditSpec={fn()}
      />
    </div>
  );
}

export const AddMenuRoot: Story = {
  render: () => (
    <StorySurface>
      <AddMenuHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByRole("button", {
      name: "Add and advanced options",
    });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    const page = body(canvasElement);
    await waitFor(() =>
      expect(
        page.getByRole("menuitem", { name: /Files and folders/ }),
      ).toHaveFocus(),
    );
    await expect(page.getByRole("menuitem", { name: /Plan mode/ })).toBeEnabled();
    await userEvent.keyboard("{Escape}");
    await expect(trigger).toHaveFocus();
    await userEvent.click(trigger);
  },
};

export const AddMenuPlanActive: Story = {
  render: () => (
    <StorySurface>
      <AddMenuHarness planMode />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Add and advanced options");
    await expect(
      body(canvasElement).getByRole("menuitem", { name: /Plan mode/ }),
    ).toBeVisible();
  },
};

export const AddMenuGoalActive: Story = {
  render: () => (
    <StorySurface>
      <AddMenuHarness goalMode />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Add and advanced options");
    await expect(
      body(canvasElement).getByRole("menuitem", {
        name: /Goal Turn goal mode off/,
      }),
    ).toBeVisible();
  },
};

export const AddMenuSession: Story = {
  render: () => (
    <StorySurface>
      <AddMenuHarness isSession />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Add and advanced options");
    await expect(
      body(canvasElement).getByRole("menuitem", { name: /Plan mode/ }),
    ).toBeDisabled();
  },
};

export const AddMenuAutomation: Story = {
  render: () => (
    <StorySurface>
      <AddMenuHarness initialPage="advanced" kind="background" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Add and advanced options");
    const page = body(canvasElement);
    await expect(page.getByText("Automation")).toBeVisible();
    await expect(
      page.getByRole("menuitem", { name: /Background run/ }),
    ).toBeVisible();
  },
};

export const AddMenuAgents: Story = {
  render: () => (
    <StorySurface>
      <AddMenuHarness initialPage="agent" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Add and advanced options");
    const page = body(canvasElement);
    await expect(page.getByText("Agent")).toBeVisible();
    await expect(page.getByRole("menuitem", { name: /Edit agent spec/ })).toBeVisible();
  },
};

export const AddMenuSelectedPersona: Story = {
  render: () => (
    <StorySurface>
      <AddMenuHarness initialPage="agent" persona="reviewer" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Add and advanced options");
    const reviewer = body(canvasElement).getByRole("menuitem", {
      name: /Reviewer/,
    });
    await expect(reviewer.querySelector(".pop-check")).toBeVisible();
  },
};

export const AddMenuPageFlowKeyboard: Story = {
  render: () => (
    <StorySurface>
      <AddMenuHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = body(canvasElement);
    const trigger = canvas.getByRole("button", {
      name: "Add and advanced options",
    });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(page.getByRole("menuitem", { name: /Files and folders/ })).toHaveFocus(),
    );

    page.getByRole("menuitem", { name: /Automation/ }).focus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(page.getByRole("menuitem", { name: "Back to add menu" })).toHaveFocus(),
    );

    page.getByRole("menuitem", { name: /Agent/ }).focus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(
        page.getByRole("menuitem", { name: "Back to automation menu" }),
      ).toHaveFocus(),
    );

    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(page.getByRole("menuitem", { name: "Back to add menu" })).toHaveFocus(),
    );
    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(page.getByRole("menuitem", { name: /Files and folders/ })).toHaveFocus(),
    );
  },
};

function AccessHarness({
  variant = "home",
  initial = "ask",
}: {
  variant?: "home" | "session";
  initial?: "full" | "acceptEdits" | "ask" | "plan";
}) {
  const [active, setActive] = useState(initial);
  const labels = {
    full: "Full access",
    acceptEdits: "Auto-accept edits",
    ask: "Ask to approve",
    plan: "Plan",
  };
  const risks = {
    full: "high",
    acceptEdits: "med",
    ask: "low",
    plan: "low",
  };
  return (
    <div className="cx-bar">
      <AccessPicker
        variant={variant}
        active={active}
        label={labels[active]}
        risk={risks[active]}
        onHomeSelect={(next, close) => {
          setActive(next);
          close();
        }}
        onSessionSelect={(target, close) => {
          setActive(target === "default" ? "ask" : "acceptEdits");
          close();
        }}
      />
    </div>
  );
}

export const AccessHomeAsk: Story = {
  render: () => (
    <StorySurface>
      <AccessHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Ask to approve");
    await expect(
      body(canvasElement).getByRole("menuitem", {
        name: /Ask to approve/,
      }),
    ).toBeVisible();
  },
};

export const AccessHomeFull: Story = {
  render: () => (
    <StorySurface>
      <AccessHarness initial="full" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Full access");
    await expect(
      body(canvasElement).getByRole("menuitem", { name: /Full access/ }),
    ).toBeVisible();
  },
};

export const AccessSessionSwitchable: Story = {
  render: () => (
    <StorySurface>
      <AccessHarness variant="session" initial="acceptEdits" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Auto-accept edits");
    const page = body(canvasElement);
    await expect(page.getByRole("button", { name: /Full access/ })).toBeDisabled();
    await expect(page.getByRole("button", { name: /^Plan/ })).toBeDisabled();
  },
};

export const AccessSessionUnknown: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar">
        <AccessPicker variant="session" />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", {
        name: "Access: set by agent spec",
      }),
    ).toBeVisible();
  },
};

export const AccessClosedStateMatrix: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar flex-wrap">
        {(
          [
            ["ask", "Ask to approve", "low"],
            ["acceptEdits", "Auto-accept edits", "med"],
            ["full", "Full access", "high"],
            ["plan", "Plan", "low"],
          ] as const
        ).map(([active, label, risk]) => (
          <AccessPicker
            key={active}
            variant="home"
            active={active}
            label={label}
            risk={risk}
            onHomeSelect={fn()}
          />
        ))}
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Ask to approve" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Auto-accept edits" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Full access" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Plan" })).toBeVisible();
  },
};

export const AccessHomeKeyboardSelection: Story = {
  render: () => (
    <StorySurface>
      <AccessHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = body(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Ask to approve" });
    await userEvent.click(trigger);
    await waitFor(() =>
      expect(page.getByRole("menu")).toBeVisible(),
    );
    const ask = page.getByRole("menuitem", { name: /Ask to approve/ });
    ask.focus();
    await expect(ask).toHaveFocus();
    await userEvent.keyboard("{ArrowDown}");
    await expect(
      page.getByRole("menuitem", { name: /Auto-accept edits/ }),
    ).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "Auto-accept edits" }),
      ).toHaveFocus(),
    );
    await expect(page.queryByRole("menu")).not.toBeInTheDocument();
  },
};

function ModelPickerHarness({
  initialPage = "root",
  budgetOverride = null,
  initialModelLabel = "Gemini Flash",
}: {
  initialPage?: "root" | "model" | "effort" | "advanced";
  budgetOverride?: number | null;
  initialModelLabel?: string;
}) {
  const [page, setPage] = useState(initialPage);
  const [model, setModel] = useState({
    provider: "gemini",
    id: "gemini-flash-latest",
    label: initialModelLabel,
  });
  const [effort, setEffort] = useState<"light" | "medium" | "high" | "xhigh">(
    "medium",
  );
  return (
    <div className="cx-bar justify-end">
      <ModelPicker
        provider={model.provider}
        model={model.id}
        modelLabel={model.label}
        effort={effort}
        effortLabel="Medium"
        budgetOverride={budgetOverride}
        page={page}
        onOpen={() => {}}
        onPageChange={setPage}
        onSelectModel={(provider, id) =>
          setModel({ provider, id, label: id })
        }
        onSelectEffort={setEffort}
        onCustomModel={fn()}
        onCustomBudget={fn()}
      />
    </div>
  );
}

export const ModelPickerSummary: Story = {
  render: () => (
    <StorySurface>
      <ModelPickerHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, /Gemini Flash/);
    const page = body(canvasElement);
    await expect(page.getByRole("menuitem", { name: /Model/ })).toBeVisible();
    await expect(page.getByRole("menuitem", { name: /Effort/ })).toBeVisible();
  },
};

export const ModelPickerModels: Story = {
  render: () => (
    <StorySurface>
      <ModelPickerHarness initialPage="model" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, /Gemini Flash/);
    await expect(body(canvasElement).getByText("Model")).toBeVisible();
  },
};

export const ModelPickerEffort: Story = {
  render: () => (
    <StorySurface>
      <ModelPickerHarness initialPage="effort" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, /Gemini Flash/);
    await expect(
      body(canvasElement).getByRole("menuitem", { name: /Medium/ }),
    ).toBeVisible();
  },
};

export const ModelPickerAdvanced: Story = {
  render: () => (
    <StorySurface>
      <ModelPickerHarness initialPage="advanced" budgetOverride={8192} />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, /Gemini Flash/);
    const page = body(canvasElement);
    await expect(page.getByRole("menuitem", { name: /Custom model id/ })).toBeVisible();
    await expect(page.getByText("8192 tokens")).toBeVisible();
  },
};

export const ModelPickerCustomLongSummary: Story = {
  render: () => (
    <StorySurface>
      <ModelPickerHarness
        initialModelLabel="provider/custom-model-with-an-intentionally-long-version-suffix"
        budgetOverride={8192}
      />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const trigger = within(canvasElement).getByTitle("Model & effort");
    await expect(trigger).toHaveTextContent("Custom");
    await expect(trigger).toHaveTextContent(
      "provider/custom-model-with-an-intentionally-long-version-suffix",
    );
  },
};

export const ModelPickerPageFlowKeyboard: Story = {
  render: () => (
    <StorySurface>
      <ModelPickerHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = body(canvasElement);
    const trigger = canvas.getByTitle("Model & effort");
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(page.getByRole("menuitem", { name: /Model Gemini Flash/ })).toHaveFocus(),
    );

    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(page.getByRole("menuitem", { name: "Back to model menu" })).toHaveFocus(),
    );
    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(page.getByRole("menuitem", { name: /Model Gemini Flash/ })).toHaveFocus(),
    );

    page.getByRole("menuitem", { name: /Effort Medium/ }).focus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(page.getByRole("menuitem", { name: "Back to model menu" })).toHaveFocus(),
    );
  },
};

function GoalOptionsHarness({
  configured = false,
}: {
  configured?: boolean;
}) {
  const [verifier, setVerifier] = useState(configured ? "npm test" : "");
  const [rounds, setRounds] = useState(configured ? 20 : 10);
  return (
    <div className="cx-bar">
      <GoalOptions
        verifier={verifier}
        rounds={rounds}
        onVerifierChange={setVerifier}
        onRoundsChange={setRounds}
        onExit={fn()}
      />
    </div>
  );
}

export const GoalOptionsSelfCertified: Story = {
  render: () => (
    <StorySurface>
      <GoalOptionsHarness />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Goal");
    const page = body(canvasElement);
    await expect(page.getByPlaceholderText(/agent self-certifies/)).toHaveValue("");
    await waitFor(() =>
      expect(page.getByRole("spinbutton")).toHaveValue(10),
    );
  },
};

export const GoalOptionsVerifier: Story = {
  render: () => (
    <StorySurface>
      <GoalOptionsHarness configured />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Goal");
    const page = body(canvasElement);
    await expect(page.getByPlaceholderText(/agent self-certifies/)).toHaveValue(
      "npm test",
    );
    await waitFor(() =>
      expect(page.getByRole("spinbutton")).toHaveValue(20),
    );
  },
};

export const GoalOptionsLongBoundary: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar">
        <GoalOptions
          verifier="npm run test:storybook -- --project chromium && npm run build-storybook"
          rounds={1}
          onVerifierChange={fn()}
          onRoundsChange={fn()}
          onExit={fn()}
        />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await openPopover(canvasElement, "Goal");
    const page = body(canvasElement);
    await expect(page.getByDisplayValue("1")).toHaveValue(1);
    const command = page.getByPlaceholderText(/agent self-certifies/);
    await expect(command).toHaveValue(
      "npm run test:storybook -- --project chromium && npm run build-storybook",
    );
    await expect(command.scrollWidth).toBeGreaterThan(command.clientWidth);
  },
};

export const GoalOptionsExitKeyboard: Story = {
  render: () => (
    <StorySurface>
      <GoalOptionsHarness configured />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = body(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Goal" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(page.getByPlaceholderText(/agent self-certifies/)).toBeVisible(),
    );
    page.getByPlaceholderText(/agent self-certifies/).focus();
    await expect(page.getByPlaceholderText(/agent self-certifies/)).toHaveFocus();
    page.getByRole("button", { name: "Exit Goal mode" }).focus();
    await userEvent.keyboard("{Enter}");
    await expect(trigger).toHaveFocus();
  },
};

export const AssistOptimize: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <AssistActions
          hasText
          canUndo={false}
          optimizing={false}
          micVisible
          micActive={false}
          dictationBusy={false}
          onOptimize={fn()}
          onUndo={fn()}
          onToggleMic={fn()}
        />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByTitle("Optimize prompt — rewrite this draft to be clearer")).toBeEnabled();
    await expect(canvas.getByTitle("Dictate")).toBeEnabled();
  },
};

export const AssistOptimizingAndTranscribing: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <AssistActions
          hasText
          canUndo={false}
          optimizing
          micVisible
          micActive
          dictationBusy
          onOptimize={fn()}
          onUndo={fn()}
          onToggleMic={fn()}
        />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByTitle("Optimize prompt — rewrite this draft to be clearer")).toBeDisabled();
    await expect(canvas.getByTitle("Transcribing…")).toBeDisabled();
  },
};

export const AssistUndo: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <AssistActions
          hasText
          canUndo
          optimizing={false}
          micVisible={false}
          micActive={false}
          dictationBusy={false}
          onOptimize={fn()}
          onUndo={fn()}
          onToggleMic={fn()}
        />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByTitle(
        "Undo optimize — restore your original draft",
      ),
    ).toBeVisible();
  },
};

export const AssistListening: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <AssistActions
          hasText={false}
          canUndo={false}
          optimizing={false}
          micVisible
          micActive
          dictationBusy={false}
          onOptimize={fn()}
          onUndo={fn()}
          onToggleMic={fn()}
        />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const mic = within(canvasElement).getByTitle("Stop dictation");
    await expect(mic).toHaveClass("listening");
    await expect(mic).toBeEnabled();
  },
};

export const AssistHidden: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <AssistActions
          hasText={false}
          canUndo={false}
          optimizing={false}
          micVisible={false}
          micActive={false}
          dictationBusy={false}
          onOptimize={fn()}
          onUndo={fn()}
          onToggleMic={fn()}
        />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).queryByRole("button"),
    ).not.toBeInTheDocument();
  },
};

function DeliveryHarness({
  initial,
}: {
  initial: "queue" | "steer";
}) {
  const [mode, setMode] = useState(initial);
  return (
    <div className="cx-bar justify-end">
      <DeliveryModeControl mode={mode} onChange={setMode} />
    </div>
  );
}

export const DeliveryQueue: Story = {
  render: () => (
    <StorySurface>
      <DeliveryHarness initial="queue" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Queue" }),
    ).toHaveAttribute("aria-pressed", "true");
  },
};

export const DeliverySteer: Story = {
  render: () => (
    <StorySurface>
      <DeliveryHarness initial="steer" />
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Steer" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await userEvent.click(canvas.getByRole("button", { name: "Queue" }));
    await expect(canvas.getByRole("button", { name: "Queue" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  },
};

export const SubmitDisabled: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <SubmitButton mode="send" disabled onSubmit={fn()} />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Send message" }),
    ).toBeDisabled();
  },
};

export const SubmitReady: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <SubmitButton mode="send" onSubmit={fn()} />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByTitle("Send (Enter)"),
    ).toBeEnabled();
  },
};

export const SubmitRunningQueue: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <SubmitButton
          mode="send"
          running
          deliveryMode="queue"
          onSubmit={fn()}
        />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByTitle("Send · queue (⌘⏎ to steer)"),
    ).toBeVisible();
  },
};

export const SubmitRunningSteer: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <SubmitButton
          mode="send"
          running
          deliveryMode="steer"
          onSubmit={fn()}
        />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByTitle("Send · steer (⌘⏎ to queue)"),
    ).toBeVisible();
  },
};

export const SubmitStop: Story = {
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <SubmitButton mode="stop" onSubmit={fn()} />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", {
        name: "Stop active turn",
      }),
    ).toBeVisible();
  },
};

export const SemanticControlPseudoStates: Story = {
  parameters: {
    pseudo: {
      hover: ".story-hover .cx-send",
      focusVisible: ".story-focus .cx-deliv",
      active: ".story-active .cx-send",
    },
  },
  render: () => (
    <StorySurface>
      <div className="cx-bar justify-end">
        <span className="story-hover">
          <SubmitButton mode="send" onSubmit={fn()} />
        </span>
        <span className="story-focus">
          <DeliveryModeControl mode="queue" onChange={fn()} />
        </span>
        <span className="story-active">
          <SubmitButton mode="stop" onSubmit={fn()} />
        </span>
        <SubmitButton mode="send" disabled onSubmit={fn()} />
      </div>
    </StorySurface>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const ready = canvas.getAllByRole("button", { name: "Send message" })[0];
    await waitFor(() => expect(ready).toBeVisible());
    await expect(canvas.getAllByRole("button", { name: "Send message" })).toHaveLength(2);
    await expect(canvas.getByRole("button", { name: "Queue" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await expect(canvas.getByRole("button", { name: "Stop active turn" })).toBeVisible();
    await expect(canvas.getAllByRole("button", { name: "Send message" })[1]).toBeDisabled();
  },
};
