import { useCallback, useRef, useState, type ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import type { DiffScope } from "../types";
import {
  ChangedFilesMenu,
  CommitPushMenu,
  DiffMoreActionsMenu,
  DiffScopePicker,
  DiffStateView,
  DiffToolbar,
  type DiffChangedFile,
} from "./DiffParts";

const files: DiffChangedFile[] = [
  {
    path: "src/components/DiffParts.tsx",
    status: "modified",
    add: 184,
    del: 12,
    binary: false,
    conflict: false,
  },
  {
    path: "src/features/changes/a/very/deep/folder/with-a-deliberately-long-component-name-that-must-ellipsis.tsx",
    status: "added",
    add: null,
    del: 0,
    binary: false,
    conflict: false,
  },
  {
    path: "src/runtime/mergeDriver.ts",
    status: "modified",
    add: 7,
    del: 5,
    binary: false,
    conflict: true,
  },
  {
    path: "artifacts/demo-recording.mp4",
    status: "added",
    add: null,
    del: 0,
    binary: true,
    conflict: false,
  },
];

function Frame({
  children,
  width = 720,
}: {
  children: ReactNode;
  width?: number;
}) {
  return (
    <StoryAppFrame>
      <main className="min-h-[520px] bg-bg p-5">
        <section
          className="changes-panel mx-auto min-h-[440px] overflow-hidden border border-line bg-panel"
          style={{ width: `min(${width}px, 100%)` }}
        >
          <div className="diffwrap">{children}</div>
        </section>
      </main>
    </StoryAppFrame>
  );
}

function ScopeHarness() {
  const [scope, setScope] = useState<DiffScope>("last-turn");
  return (
    <Frame>
      <div className="diffbar diffbar-state">
        <DiffScopePicker scope={scope} onSelect={setScope} />
      </div>
      <output className="block p-4">Selected: {scope}</output>
    </Frame>
  );
}

function ChangedFilesHarness() {
  const [query, setQuery] = useState("");
  const [focused, setFocused] = useState("none");
  const shown = query.trim()
    ? files.filter((file) =>
        file.path.toLowerCase().includes(query.trim().toLowerCase()),
      )
    : files;
  return (
    <Frame width={390}>
      <div className="diffbar">
        <span className="spacer" />
        <ChangedFilesMenu
          files={shown}
          fileCount={files.length}
          query={query}
          hiddenUntracked={12_345}
          onQueryChange={setQuery}
          onFocusFile={setFocused}
        />
      </div>
      <output className="block break-all p-4">Focused: {focused}</output>
    </Frame>
  );
}

function MoreActionsHarness({ busy = false }: { busy?: boolean }) {
  const [action, setAction] = useState("none");
  return (
    <Frame width={390}>
      <div className="diffbar">
        <span className="spacer" />
        <DiffMoreActionsMenu
          fileCount={files.length}
          allShownOpen
          barTight
          empty={false}
          wrap
          narrow={false}
          view="inline"
          scope="working-tree"
          worktree
          mainRepo="/projects/agentrunner"
          busy={busy}
          onToggleAll={() => setAction("collapse")}
          onToggleWrap={() => setAction("wrap")}
          onCopy={() => setAction("copy")}
          onToggleView={() => setAction("split")}
          onRefresh={() => setAction("refresh")}
          onApplyProject={() => setAction("apply")}
          onRemoveWorktree={() => setAction("remove")}
        />
      </div>
      <output className="block p-4">Action: {action}</output>
    </Frame>
  );
}

function CommitHarness({
  conflictCount = 0,
  empty = false,
  isRepo = true,
  busy = false,
}: {
  conflictCount?: number;
  empty?: boolean;
  isRepo?: boolean;
  busy?: boolean;
}) {
  const [action, setAction] = useState("none");
  return (
    <Frame width={390}>
      <div className="diffbar">
        <span className="spacer" />
        <CommitPushMenu
          isRepo={isRepo}
          busy={busy}
          empty={empty}
          conflictCount={conflictCount}
          compact={false}
          onCommit={() => setAction("commit")}
          onCommitAndPush={() => setAction("commit-and-push")}
          onPush={() => setAction("push")}
        />
      </div>
      <output className="block p-4">Action: {action}</output>
    </Frame>
  );
}

function ToolbarHarness({ tight = false }: { tight?: boolean }) {
  const [scope, setScope] = useState<DiffScope>("working-tree");
  const [query, setQuery] = useState("");
  const [wrap, setWrap] = useState(true);
  const [view, setView] = useState<"inline" | "split">("inline");
  const [action, setAction] = useState("none");
  const [measuredTight, setMeasuredTight] = useState(tight);
  const resizeObserver = useRef<ResizeObserver | null>(null);
  const barRef = useCallback(
    (element: HTMLDivElement | null) => {
      resizeObserver.current?.disconnect();
      resizeObserver.current = null;
      if (!element) return;
      const measure = () =>
        setMeasuredTight(tight || element.clientWidth < 640);
      measure();
      resizeObserver.current = new ResizeObserver(measure);
      resizeObserver.current.observe(element);
    },
    [tight],
  );
  const compact = tight || measuredTight;
  const shown = query.trim()
    ? files.filter((file) =>
        file.path.toLowerCase().includes(query.trim().toLowerCase()),
      )
    : files;
  return (
    <Frame width={tight ? 390 : 720}>
      <DiffToolbar
        variant="ready"
        barRef={barRef}
        scope={scope}
        onScopeChange={setScope}
        onRefresh={() => setAction("refresh")}
        onClose={() => setAction("close")}
        barTight={compact}
        empty={false}
        totalAdd={191}
        totalDel={17}
        worktree
        mainRepo="/projects/agentrunner"
        branch="storybook/component-boundaries-with-a-long-branch-name"
        chipCompact={compact}
        files={shown}
        fileCount={files.length}
        query={query}
        hiddenUntracked={12_345}
        allShownOpen
        narrow={compact}
        view={view}
        wrap={wrap}
        busy={false}
        isRepo
        conflictCount={1}
        onQueryChange={setQuery}
        onFocusFile={(path) => setAction(`focus:${path}`)}
        onToggleAll={() => setAction("collapse")}
        onToggleWrap={() => setWrap((current) => !current)}
        onCopy={() => setAction("copy")}
        onToggleView={() =>
          setView((current) => (current === "inline" ? "split" : "inline"))
        }
        onApplyProject={() => setAction("apply")}
        onRemoveWorktree={() => setAction("remove")}
        onCommit={() => setAction("commit")}
        onCommitAndPush={() => setAction("commit-and-push")}
        onPush={() => setAction("push")}
      />
      <output className="block break-all p-4">
        Scope: {scope}; Action: {action}; View: {view}; Wrap:{" "}
        {wrap ? "on" : "off"}
      </output>
    </Frame>
  );
}

const meta = {
  title: "Components/Changes/DiffParts",
  component: DiffToolbar,
  args: {
    variant: "state",
    scope: "last-turn",
    onScopeChange: () => undefined,
    onRefresh: () => undefined,
  },
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof DiffToolbar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = {
  render: () => (
    <Frame>
      <DiffStateView state={{ kind: "loading" }} />
    </Frame>
  ),
};

export const ErrorRetry: Story = {
  render: () => {
    const Retry = () => {
      const [retried, setRetried] = useState(false);
      return (
        <Frame>
          <DiffStateView
            state={{
              kind: "error",
              message: "The in-memory diff fixture is unavailable.",
              onRetry: () => setRetried(true),
            }}
          />
          <output className="block text-center">Retried: {String(retried)}</output>
        </Frame>
      );
    };
    return <Retry />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Try again" }));
    await expect(canvas.getByText("Retried: true")).toBeVisible();
  },
};

export const UnavailableStates: Story = {
  render: () => (
    <StoryAppFrame>
      <main className="grid min-h-screen gap-4 bg-bg p-5 md:grid-cols-2">
        <section className="diffwrap border border-line bg-panel">
          <DiffStateView
            state={{
              kind: "last-turn-unavailable",
              reason: "The baseline was recorded before durable turn snapshots.",
            }}
          />
        </section>
        <section className="diffwrap border border-line bg-panel">
          <DiffStateView
            state={{ kind: "workspace-unavailable", onRetry: () => undefined }}
          />
        </section>
        <section className="diffwrap border border-line bg-panel">
          <DiffStateView
            state={{ kind: "nested", busy: false, onTrack: () => undefined }}
          />
        </section>
        <section className="diffwrap border border-line bg-panel">
          <DiffStateView
            state={{ kind: "non-repo", busy: true, onTrack: () => undefined }}
          />
        </section>
      </main>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Last turn unavailable")).toBeVisible();
    await expect(canvas.getByText("Workspace unavailable")).toBeVisible();
    await expect(
      canvas.getByText("Changes can't be tracked here yet"),
    ).toBeVisible();
    await expect(canvas.getByText("No Git changes to review")).toBeVisible();
    await expect(
      canvas.getAllByRole("button", { name: "Track changes (git init)" })[1],
    ).toBeDisabled();
  },
};

export const EmptyAndNoMatches: Story = {
  render: () => (
    <StoryAppFrame>
      <main className="grid min-h-screen gap-4 bg-bg p-5 md:grid-cols-3">
        <section className="diffwrap border border-line bg-panel">
          <DiffStateView state={{ kind: "empty", scope: "last-turn" }} />
        </section>
        <section className="diffwrap border border-line bg-panel">
          <DiffStateView state={{ kind: "empty", scope: "working-tree" }} />
        </section>
        <section className="diffwrap border border-line bg-panel">
          <DiffStateView
            state={{
              kind: "no-matches",
              query: "nonexistent",
              fileCount: 42,
            }}
          />
        </section>
      </main>
    </StoryAppFrame>
  ),
};

export const ScopePickerKeyboard: Story = {
  render: () => <ScopeHarness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Change diff scope" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(
        canvas.getByRole("menuitem", { name: /Working Tree/ }),
      ).toHaveFocus(),
    );
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByText("Selected: working-tree")).toBeVisible();
    await expect(trigger).toHaveFocus();
  },
};

export const ChangedFilesLongPaths: Story = {
  render: () => <ChangedFilesHarness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Changed files" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    const filter = await canvas.findByRole("textbox", {
      name: "Filter files by path",
    });
    await userEvent.click(filter);
    await userEvent.type(filter, "merge");
    await expect(canvas.getByText("1 of 4 files match")).toBeVisible();
    await userEvent.click(
      canvas.getByRole("button", { name: /mergeDriver\.ts/ }),
    );
    await expect(
      canvas.getByText("Focused: src/runtime/mergeDriver.ts"),
    ).toBeVisible();
  },
};

export const ChangedFilesOverflow: Story = {
  render: () => <ChangedFilesHarness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Changed files" }),
    );
    await expect(
      canvas.getByRole("button", {
        name: /with-a-deliberately-long-component-name-that-must-ellipsis\.tsx/,
      }),
    ).toBeVisible();
    await expect(
      canvas.getByText("12,345 generated files hidden"),
    ).toBeVisible();
  },
};

export const MoreActionsTightWorktree: Story = {
  render: () => <MoreActionsHarness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", {
      name: "More changes actions",
    });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(
        canvas.getByRole("menuitem", { name: /Collapse all files/ }),
      ).toHaveFocus(),
    );
    await userEvent.click(
      canvas.getByRole("menuitem", { name: /Apply to project/ }),
    );
    await expect(canvas.getByText("Action: apply")).toBeVisible();
  },
};

export const MoreActionsBusy: Story = {
  render: () => <MoreActionsHarness busy />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "More changes actions" }),
    );
    await expect(
      canvas.getByRole("menuitem", { name: /Apply to project/ }),
    ).toBeDisabled();
    await expect(
      canvas.getByRole("menuitem", { name: /Remove worktree/ }),
    ).toBeDisabled();
  },
};

export const CommitReady: Story = {
  render: () => <CommitHarness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Commit or push" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(
        canvas.getByRole("menuitem", { name: /^Commit git/ }),
      ).toHaveFocus(),
    );
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByText("Action: commit")).toBeVisible();
  },
};

export const CommitConflict: Story = {
  render: () => <CommitHarness conflictCount={2} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Commit or push" }),
    );
    await expect(
      canvas.getByRole("menuitem", { name: /^Commit Resolve/ }),
    ).toBeDisabled();
    await expect(
      canvas.getByRole("menuitem", { name: /^Commit & push Resolve/ }),
    ).toBeDisabled();
    await userEvent.click(canvas.getByRole("menuitem", { name: /^Push/ }));
    await expect(canvas.getByText("Action: push")).toBeVisible();
  },
};

export const CommitEmpty: Story = {
  render: () => <CommitHarness empty />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Commit or push" }),
    );
    await expect(
      canvas.getByRole("menuitem", { name: /^Commit git/ }),
    ).toBeDisabled();
    await expect(canvas.getByRole("menuitem", { name: /^Push/ })).toBeEnabled();
  },
};

export const CommitUnavailable: Story = {
  render: () => <CommitHarness isRepo={false} />,
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Commit or push" }),
    ).toBeDisabled();
  },
};

export const ToolbarReady: Story = {
  render: () => <ToolbarHarness />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Review")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Close changes" }),
    ).toBeVisible();
    await userEvent.click(
      canvas.getByRole("button", { name: "Wrap long lines" }),
    );
    await expect(canvas.getByText(/Wrap: off/)).toBeVisible();
  },
};

export const ToolbarTight: Story = {
  render: () => <ToolbarHarness tight />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByText("Review")).not.toBeInTheDocument();
    await expect(
      canvas.getByRole("button", { name: "Close changes" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "More changes actions" }),
    ).toBeVisible();
  },
};

export const ToolbarState: Story = {
  render: () => (
    <Frame width={390}>
      <DiffToolbar
        variant="state"
        scope="last-turn"
        onScopeChange={() => undefined}
        onRefresh={() => undefined}
        onClose={() => undefined}
      />
      <DiffStateView state={{ kind: "loading" }} />
    </Frame>
  ),
};
