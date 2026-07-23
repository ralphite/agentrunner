export type CoverageCell =
  | { status: "covered"; storyId: string }
  | { status: "missing" }
  | {
      status: "n-a";
      reason: string;
      evidence: string;
      owner: string;
    };

export interface ComponentTarget {
  componentId: string;
  source: string;
  exportName: string;
  cells: Record<string, CoverageCell>;
}

export type StoryManifest = readonly ComponentTarget[];

const BASE_CELLS = [
  "render:default",
  "theme:dark",
  "viewport:phone",
  "a11y:keyboard",
] as const;

function missingCells(extra: readonly string[] = []): Record<string, CoverageCell> {
  return Object.fromEntries(
    [...BASE_CELLS, ...extra].map((cell) => [cell, { status: "missing" }]),
  );
}

function target(
  componentId: string,
  source: string,
  exportName = componentId,
  extra: readonly string[] = [],
): ComponentTarget {
  return {
    componentId,
    source,
    exportName,
    cells: missingCells(extra),
  };
}

export const storyManifest = [
  {
    componentId: "ApprovalCard",
    source: "src/components/ApprovalCard.tsx",
    exportName: "ApprovalCard",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "components-attention-approvalcard--pending",
      },
      "theme:dark": {
        status: "covered",
        storyId: "components-attention-approvalcard--pending-dark",
      },
      "viewport:phone": {
        status: "covered",
        storyId: "components-attention-approvalcard--pending-phone",
      },
      "a11y:keyboard": {
        status: "covered",
        storyId: "components-attention-approvalcard--keyboard-approval",
      },
      "interaction:details": {
        status: "covered",
        storyId: "components-attention-approvalcard--details-open",
      },
      "domain:readonly-child": {
        status: "covered",
        storyId: "components-attention-approvalcard--readonly-child",
      },
    },
  },
  target("AskForm", "src/components/AskForm.tsx"),
  target("ChangesOutcome", "src/components/ChangesOutcome.tsx"),
  target("CommandPalette", "src/components/CommandPalette.tsx"),
  target("Composer", "src/components/Composer.tsx"),
  target("ContextMenu", "src/components/ContextMenu.tsx"),
  target("DaemonAlert", "src/components/DaemonAlert.tsx"),
  target("DiffView", "src/components/DiffView.tsx"),
  target("ErrorBoundary", "src/components/ErrorBoundary.tsx"),
  target("FindBar", "src/components/FindBar.tsx"),
  target("Home", "src/components/Home.tsx"),
  target("Lightbox", "src/components/Lightbox.tsx"),
  target("Markdown", "src/components/Markdown.tsx"),
  target("Menu", "src/components/Menu.tsx"),
  target("MenuItem", "src/components/Menu.tsx"),
  target("MenuLabel", "src/components/Menu.tsx"),
  target("MermaidBlock", "src/components/Mermaid.tsx"),
  target("Modal", "src/components/Modals.tsx"),
  target("Modals", "src/components/Modals.tsx"),
  target("SessionNotFound", "src/components/NotFound.tsx"),
  target("Popover", "src/components/Popover.tsx"),
  target("PopSection", "src/components/Popover.tsx"),
  target("PopItem", "src/components/Popover.tsx"),
  target("RunView", "src/components/RunView.tsx"),
  target("Scheduled", "src/components/Scheduled.tsx"),
  target("SessionView", "src/components/SessionView.tsx"),
  target("Settings", "src/components/Settings.tsx"),
  target("SettingsAppearance", "src/components/SettingsAppearance.tsx"),
  target("SettingsArchived", "src/components/SettingsArchived.tsx"),
  target("SettingsConfiguration", "src/components/SettingsConfiguration.tsx"),
  target("SettingsGeneral", "src/components/SettingsGeneral.tsx"),
  target("SettingsGit", "src/components/SettingsGit.tsx"),
  target("SettingsShortcuts", "src/components/SettingsShortcuts.tsx"),
  target("SettingsWorktrees", "src/components/SettingsWorktrees.tsx"),
  target("Shortcuts", "src/components/Shortcuts.tsx"),
  target("Sidebar", "src/components/Sidebar.tsx"),
  target("Subagents", "src/components/Subagents.tsx"),
  target("SupervisionPanel", "src/components/SupervisionPanel.tsx"),
  target("TimelineView", "src/components/Timeline.tsx"),
  target("Toasts", "src/components/Toasts.tsx"),
] satisfies StoryManifest;
