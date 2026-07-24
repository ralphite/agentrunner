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
  // Most Stories are colocated by replacing `.tsx` with `.stories.tsx`.
  // Composite source files may deliberately give each exported component its
  // own Story file; those targets name that exact file here.
  storySource?: string;
  // Omitted for a file-private visible React leaf. The declaration name then
  // equals componentId until the extraction commit makes it a named export.
  exportName?: string;
  cells: Record<string, CoverageCell>;
}

export type StoryManifest = readonly ComponentTarget[];

export interface WorkbenchStory {
  storyId: string;
  source: string;
  kind: "cuj" | "demo";
  evidence: string;
  owner: string;
}

export interface PrivateVisibleExclusion {
  source: string;
  declarationName: string;
  reason: string;
  evidence: string;
  owner: string;
}

const BASE_CELLS = [
  "render:default",
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

function coveredBaseTarget(
  componentId: string,
  source: string,
  storyPrefix: string,
  exportName = componentId,
  keyboardStory = "keyboard-navigation",
): ComponentTarget {
  return {
    componentId,
    source,
    exportName,
    cells: {
      "render:default": {
        status: "covered",
        storyId: `${storyPrefix}--default`,
      },
      "a11y:keyboard": {
        status: "covered",
        storyId: `${storyPrefix}--${keyboardStory}`,
      },
    },
  };
}

function coveredDirectLeafTarget(
  componentId: string,
  source: string,
  storyId: string,
  compositionEvidence: string,
): ComponentTarget {
  return {
    componentId,
    source,
    exportName: componentId,
    cells: {
      "render:default": {
        status: "covered",
        storyId,
      },
      "a11y:keyboard": {
        status: "n-a",
        reason: "Keyboard semantics are owned by the interactive composition or the leaf is inert.",
        evidence: `${storyId} runs the direct leaf through the Story browser a11y gate; ${compositionEvidence}`,
        owner: "webui",
      },
    },
  };
}

function coveredInteractiveLeafTarget(
  componentId: string,
  source: string,
  storyPrefix: string,
): ComponentTarget {
  return {
    componentId,
    source,
    exportName: componentId,
    cells: {
      "render:default": {
        status: "covered",
        storyId: `${storyPrefix}--${componentId
          .replace(/([a-z0-9])([A-Z])/g, "$1-$2")
          .toLowerCase()}-default`,
      },
      "a11y:keyboard": {
        status: "covered",
        storyId: `${storyPrefix}--${componentId
          .replace(/([a-z0-9])([A-Z])/g, "$1-$2")
          .toLowerCase()}-keyboard-navigation`,
      },
    },
  };
}

function withCells(
  base: ComponentTarget,
  extra: Record<string, CoverageCell>,
): ComponentTarget {
  return {
    ...base,
    cells: {
      ...base.cells,
      ...extra,
    },
  };
}

function coveredStateTarget({
  componentId,
  source,
  storySource,
  renderStory,
  keyboardStory,
  stateStories = {},
}: {
  componentId: string;
  source: string;
  storySource?: string;
  renderStory: string;
  keyboardStory?: string | null;
  stateStories?: Record<string, string>;
}): ComponentTarget {
  return {
    componentId,
    source,
    storySource,
    exportName: componentId,
    cells: {
      "render:default": {
        status: "covered",
        storyId: renderStory,
      },
      "a11y:keyboard": keyboardStory
        ? {
            status: "covered",
            storyId: keyboardStory,
          }
        : {
            status: "n-a",
            reason: "This leaf is informational and has no independently focusable control.",
            evidence: `${renderStory} renders the leaf directly through the Story browser a11y gate; keyboard behavior belongs to its interactive parent composition.`,
            owner: "webui",
          },
      ...Object.fromEntries(
        Object.entries(stateStories).map(([cellId, storyId]) => [
          cellId,
          { status: "covered" as const, storyId },
        ]),
      ),
    },
  };
}

function coveredPrefixedStateTarget(
  componentId: string,
  source: string,
  storyPrefix: string,
  renderStory: string,
  keyboardStory: string | null,
  extraStories: readonly string[] = [],
  storySource?: string,
): ComponentTarget {
  return coveredStateTarget({
    componentId,
    source,
    storySource,
    renderStory: `${storyPrefix}--${renderStory}`,
    keyboardStory: keyboardStory
      ? `${storyPrefix}--${keyboardStory}`
      : null,
    stateStories: Object.fromEntries(
      extraStories.map((storyName) => [
        `state:${storyName}`,
        `${storyPrefix}--${storyName}`,
      ]),
    ),
  });
}

// The AST baseline intentionally over-collects uppercase functions so a new
// visible leaf cannot silently escape the denominator. Every candidate that is
// not a Story target must be classified here with reviewable evidence.
export const privateVisibleExclusions = [
  {
    source: "src/app/AppRuntime.tsx",
    declarationName: "RuntimeController",
    reason: "Non-visual runtime effect owner; it renders children unchanged.",
    evidence: "AppRuntime Story covers the rendered shell while RuntimeController characterization tests cover effects.",
    owner: "webui",
  },
  {
    source: "src/components/ChangesOutcome.tsx",
    declarationName: "PlusMinusSquare",
    reason: "Decorative icon adapter with no independent state or interaction.",
    evidence: "Rendered only inside ChangesOutcome controls and hidden from the accessibility tree.",
    owner: "webui",
  },
  {
    source: "src/components/DiffParts.tsx",
    declarationName: "DiffCloseButton",
    reason: "Internal DiffToolbar affordance with no standalone product contract.",
    evidence: "DiffToolbar Ready, Tight, and State Stories render the close action in both production toolbar variants.",
    owner: "webui",
  },
  ...[
    "BestIcon",
    "GoalIcon",
    "LoopIcon",
  ].map((declarationName) => ({
    source: "src/components/Composer.tsx",
    declarationName,
    reason: "Decorative icon adapter; not an independently operable UI surface.",
    evidence: "Its semantics and state belong to the Composer control that supplies the accessible name.",
    owner: "webui",
  })),
  ...["AccessIcon", "RiskGlyph"].map((declarationName) => ({
    source: "src/components/ComposerParts.tsx",
    declarationName,
    reason: "Decorative status icon with no independent state or interaction.",
    evidence: "AccessPicker Stories cover every labelled access and risk state that selects the icon.",
    owner: "webui",
  })),
  {
    source: "src/components/ComposerParts.tsx",
    declarationName: "PickerBack",
    reason: "Internal ModelPicker subpage header; it has no standalone product contract.",
    evidence: "ModelPicker Stories exercise the Model, Effort, and Thinking budget subpages and their back interaction.",
    owner: "webui",
  },
  ...["CloudMark", "Telescope"].map((declarationName) => ({
    source: "src/components/Home.tsx",
    declarationName,
    reason: "Decorative illustration primitive with no independent product state.",
    evidence: "Home owns the visible empty-state composition and accessibility semantics.",
    owner: "webui",
  })),
  ...["CategoryIcon", "StepIcon"].map((declarationName) => ({
    source: "src/components/Timeline.tsx",
    declarationName,
    reason: "Decorative status icon selected by its owning timeline row.",
    evidence: "Tool/Activity Stories cover each status through the complete labelled row.",
    owner: "webui",
  })),
] satisfies readonly PrivateVisibleExclusion[];

// CUJs and Demos exercise multiple production targets at once. They belong to
// the same authoritative inventory, but are not component coverage cells and
// therefore carry their own exact Story source/evidence record.
export const workbenchStories = [
  {
    storyId: "demos-core-session-playback--demo",
    source: "src/storybook/demos/CoreSessionPlayback.stories.tsx",
    kind: "demo",
    evidence:
      "Production AppRuntime/AppShell journey from Home project and Build intent through configuration, send, deterministic streaming, Environment, completion, Changes, and Review; in-canvas transport covers Play/Pause/Next/Replay/Reset/speed/autoplay.",
    owner: "webui",
  },
] satisfies readonly WorkbenchStory[];

export const storyManifest = [
  {
    componentId: "AppShell",
    source: "src/App.tsx",
    exportName: "AppShell",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "pages-appshell--default",
      },
      "a11y:keyboard": {
        status: "covered",
        storyId: "pages-appshell--keyboard-navigation",
      },
    },
  },
  {
    componentId: "AppRuntime",
    source: "src/app/AppRuntime.tsx",
    exportName: "AppRuntime",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "pages-appruntime--default",
      },
      "a11y:keyboard": {
        status: "covered",
        storyId: "pages-appruntime--keyboard-navigation",
      },
    },
  },
  {
    componentId: "ApprovalCard",
    source: "src/components/ApprovalCard.tsx",
    exportName: "ApprovalCard",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "components-attention-approvalcard--pending",
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
  withCells(
    coveredBaseTarget(
      "AskForm",
      "src/components/AskForm.tsx",
      "components-attention-askform",
      "AskForm",
      "keyboard-answer",
    ),
    {
      "domain:multiple-answers": {
        status: "covered",
        storyId: "components-attention-askform--multiple-answers",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "ChangesOutcome",
      "src/components/ChangesOutcome.tsx",
      "components-changes-changesoutcome",
    ),
    {
      "data:workspace-fallback": {
        status: "covered",
        storyId: "components-changes-changesoutcome--workspace-fallback",
      },
      "failure:request": {
        status: "covered",
        storyId: "components-changes-changesoutcome--request-failure",
      },
    },
  ),
  ...[
    ["ArtifactChips", "artifact-chips"],
    ["ArtifactRow", "artifact-row"],
    ["ChangesShell", "changes-shell"],
    ["ImageArtifacts", "image-artifacts"],
    ["ImageCard", "image-card"],
  ].map(([componentId, storyName]) =>
    coveredDirectLeafTarget(
      componentId,
      "src/components/ChangesOutcome.tsx",
      `components-changes-changesoutcome--${storyName}`,
      "ChangesOutcome direct and keyboard Stories exercise the leaf in its production composition; theme and viewport are verified with Storybook controls.",
    ),
  ),
  coveredBaseTarget(
    "CommandPalette",
    "src/components/CommandPalette.tsx",
    "components-navigation-commandpalette",
  ),
  withCells(
    coveredBaseTarget(
      "Composer",
      "src/components/Composer.tsx",
      "components-input-composer",
    ),
    {
      "state:draft": {
        status: "covered",
        storyId: "components-input-composer--draft",
      },
      "delivery:queue": {
        status: "covered",
        storyId: "components-input-composer--running-queued",
      },
      "delivery:steer": {
        status: "covered",
        storyId: "components-input-composer--running-steer",
      },
      "interaction:stop": {
        status: "covered",
        storyId: "components-input-composer--stop-active-turn",
      },
      "state:fork-attachments": {
        status: "covered",
        storyId: "components-input-composer--fork-draft-with-attachments",
      },
      "interaction:project": {
        status: "covered",
        storyId: "components-input-composer--project-picker",
      },
      "interaction:model-effort": {
        status: "covered",
        storyId: "components-input-composer--model-and-effort",
      },
      "interaction:access": {
        status: "covered",
        storyId: "components-input-composer--access-and-approval",
      },
      "interaction:goal": {
        status: "covered",
        storyId: "components-input-composer--goal-launcher",
      },
      "interaction:slash": {
        status: "covered",
        storyId: "components-input-composer--slash-commands",
      },
    },
  ),
  coveredDirectLeafTarget(
    "GoalLoopLauncher",
    "src/components/Composer.tsx",
    "components-input-composer--goal-loop-launcher",
    "Composer direct and keyboard Stories exercise the launcher in its production composition; theme and viewport are verified with Storybook controls.",
  ),
  coveredBaseTarget(
    "ContextMenu",
    "src/components/ContextMenu.tsx",
    "components-overlays-contextmenu",
  ),
  withCells(
    coveredBaseTarget(
      "DaemonAlert",
      "src/components/DaemonAlert.tsx",
      "components-attention-daemonalert",
      "DaemonAlert",
      "keyboard-retry",
    ),
    {
      "lifecycle:healthy-hidden": {
        status: "covered",
        storyId: "components-attention-daemonalert--healthy-hidden",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "DiffView",
      "src/components/DiffView.tsx",
      "components-changes-diffview",
    ),
    {
      "failure:request": {
        status: "covered",
        storyId: "components-changes-diffview--request-failure",
      },
      "data:workspace-unavailable": {
        status: "covered",
        storyId: "components-changes-diffview--workspace-unavailable",
      },
    },
  ),
  ...[
    ["FileBody", "file-body"],
    ["FileHead", "file-head"],
    ["UntrackedFile", "untracked-file"],
  ].map(([componentId, storyName]) =>
    coveredDirectLeafTarget(
      componentId,
      "src/components/DiffView.tsx",
      `components-changes-diffview--${storyName}`,
      "DiffView direct and keyboard Stories exercise the leaf in its production composition; theme and viewport are verified with Storybook controls.",
    ),
  ),
  coveredPrefixedStateTarget(
    "DiffScopePicker",
    "src/components/DiffParts.tsx",
    "components-changes-diffparts",
    "scope-picker-keyboard",
    "scope-picker-keyboard",
  ),
  coveredPrefixedStateTarget(
    "DiffSkeleton",
    "src/components/DiffParts.tsx",
    "components-changes-diffparts",
    "loading",
    null,
  ),
  coveredPrefixedStateTarget(
    "DiffStateView",
    "src/components/DiffParts.tsx",
    "components-changes-diffparts",
    "loading",
    "error-retry",
    ["unavailable-states", "empty-and-no-matches"],
  ),
  coveredPrefixedStateTarget(
    "ChangedFilesMenu",
    "src/components/DiffParts.tsx",
    "components-changes-diffparts",
    "changed-files-long-paths",
    "changed-files-long-paths",
    ["changed-files-overflow"],
  ),
  coveredPrefixedStateTarget(
    "DiffMoreActionsMenu",
    "src/components/DiffParts.tsx",
    "components-changes-diffparts",
    "more-actions-tight-worktree",
    "more-actions-tight-worktree",
    ["more-actions-busy"],
  ),
  coveredPrefixedStateTarget(
    "CommitPushMenu",
    "src/components/DiffParts.tsx",
    "components-changes-diffparts",
    "commit-ready",
    "commit-ready",
    ["commit-conflict", "commit-empty", "commit-unavailable"],
  ),
  coveredPrefixedStateTarget(
    "DiffToolbar",
    "src/components/DiffParts.tsx",
    "components-changes-diffparts",
    "toolbar-ready",
    "toolbar-ready",
    ["toolbar-tight", "toolbar-state"],
  ),
  withCells(
    coveredBaseTarget(
      "ErrorBoundary",
      "src/components/ErrorBoundary.tsx",
      "components-feedback-errorboundary",
      "ErrorBoundary",
      "keyboard-recovery",
    ),
    {
      "failure:render": {
        status: "covered",
        storyId: "components-feedback-errorboundary--render-error",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "FindBar",
      "src/components/FindBar.tsx",
      "components-navigation-findbar",
    ),
    {
      "data:no-matches": {
        status: "covered",
        storyId: "components-navigation-findbar--no-matches",
      },
    },
  ),
  withCells(
    coveredBaseTarget("Home", "src/components/Home.tsx", "pages-home"),
    {
      "interaction:starter-intent": {
        status: "covered",
        storyId: "pages-home--starter-intent-flow",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "Lightbox",
      "src/components/Lightbox.tsx",
      "components-media-lightbox",
    ),
    {
      "interaction:zoom-limits": {
        status: "covered",
        storyId: "components-media-lightbox--zoom-limits",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "Markdown",
      "src/components/Markdown.tsx",
      "components-content-markdown",
    ),
    {
      "security:untrusted-html": {
        status: "covered",
        storyId: "components-content-markdown--untrusted-html",
      },
    },
  ),
  ...[
    ["CodeBlock", "code-block"],
    ["MdImage", "md-image"],
  ].map(([componentId, storyName]) =>
    coveredDirectLeafTarget(
      componentId,
      "src/components/Markdown.tsx",
      `components-content-markdown--${storyName}`,
      "Markdown direct and keyboard Stories exercise the leaf in its production composition; theme and viewport are verified with Storybook controls.",
    ),
  ),
  coveredBaseTarget("Menu", "src/components/Menu.tsx", "components-overlays-menu"),
  coveredBaseTarget("MenuItem", "src/components/Menu.tsx", "components-overlays-menu"),
  coveredBaseTarget("MenuLabel", "src/components/Menu.tsx", "components-overlays-menu"),
  withCells(
    coveredBaseTarget(
      "MermaidBlock",
      "src/components/Mermaid.tsx",
      "components-content-mermaidblock",
    ),
    {
      "failure:invalid-source": {
        status: "covered",
        storyId: "components-content-mermaidblock--invalid-source-fallback",
      },
    },
  ),
  withCells(
    target("Modal", "src/components/Modals.tsx"),
    {
      "render:default": {
        status: "covered",
        storyId: "components-overlays-modals--standalone-default",
      },
      "a11y:keyboard": {
        status: "covered",
        storyId: "components-overlays-modals--standalone-keyboard-navigation",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "Modals",
      "src/components/Modals.tsx",
      "components-overlays-modals",
    ),
    {
      "layering:prompt-over-modal": {
        status: "covered",
        storyId: "components-overlays-modals--prompt-over-main-modal",
      },
      "interaction:confirm": {
        status: "covered",
        storyId: "components-overlays-modals--confirm-action",
      },
    },
  ),
  ...[
    "AgentModal",
    "ConfirmModal",
    "ForkModal",
    "MainModal",
    "NewSessionModal",
    "PromptModal",
    "RenameModal",
    "RunDetailsModal",
    "RunModal",
    "TrustModal",
    "ViewerModal",
  ].map((componentId) =>
    coveredInteractiveLeafTarget(
      componentId,
      "src/components/Modals.tsx",
      "components-overlays-modals",
    ),
  ),
  withCells(
    coveredBaseTarget(
      "SessionNotFound",
      "src/components/NotFound.tsx",
      "components-feedback-sessionnotfound",
      "SessionNotFound",
      "keyboard-back",
    ),
    {
      "density:long-session-id": {
        status: "covered",
        storyId: "components-feedback-sessionnotfound--long-session-id",
      },
    },
  ),
  coveredBaseTarget("Popover", "src/components/Popover.tsx", "components-overlays-popover"),
  coveredBaseTarget("PopSection", "src/components/Popover.tsx", "components-overlays-popover"),
  coveredBaseTarget("PopItem", "src/components/Popover.tsx", "components-overlays-popover"),
  withCells(
    coveredBaseTarget(
      "RunView",
      "src/components/RunView.tsx",
      "components-runs-runview",
    ),
    {
      "state:waiting-output": {
        status: "covered",
        storyId: "components-runs-runview--waiting-for-output",
      },
      "failure:verdict": {
        status: "covered",
        storyId: "components-runs-runview--failed-verdict",
      },
      "state:completed-one-time": {
        status: "covered",
        storyId: "components-runs-runview--completed-one-time-run",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "Scheduled",
      "src/components/Scheduled.tsx",
      "pages-scheduled",
    ),
    {
      "data:empty": {
        status: "covered",
        storyId: "pages-scheduled--empty",
      },
      "state:detail": {
        status: "covered",
        storyId: "pages-scheduled--schedule-detail",
      },
      "state:paused": {
        status: "covered",
        storyId: "pages-scheduled--paused-schedule-detail",
      },
      "interaction:edit": {
        status: "covered",
        storyId: "pages-scheduled--edit-schedule",
      },
      "state:loading": {
        status: "covered",
        storyId: "pages-scheduled--detail-loading",
      },
      "failure:detail": {
        status: "covered",
        storyId: "pages-scheduled--detail-error",
      },
      "data:no-results": {
        status: "covered",
        storyId: "pages-scheduled--filter-and-no-results",
      },
    },
  ),
  ...[
    ["ScheduleDetailPanel", "schedule-detail-panel"],
    ["ScheduleEditDialog", "schedule-edit-dialog"],
  ].map(([componentId, storyName]) =>
    coveredDirectLeafTarget(
      componentId,
      "src/components/Scheduled.tsx",
      `pages-scheduled--${storyName}`,
      "Scheduled direct and keyboard Stories exercise the leaf in its production composition; theme and viewport are verified with Storybook controls.",
    ),
  ),
  withCells(
    coveredBaseTarget(
      "SessionView",
      "src/components/SessionView.tsx",
      "components-sessions-sessionview",
    ),
    {
      "state:loading": {
        status: "covered",
        storyId: "components-sessions-sessionview--loading",
      },
      "data:empty": {
        status: "covered",
        storyId: "components-sessions-sessionview--empty",
      },
      "attention:approval": {
        status: "covered",
        storyId: "components-sessions-sessionview--approval-required",
      },
      "attention:structured-answer": {
        status: "covered",
        storyId: "components-sessions-sessionview--structured-answer-required",
      },
      "state:goal-progress": {
        status: "covered",
        storyId: "components-sessions-sessionview--goal-and-progress",
      },
      "failure:provider": {
        status: "covered",
        storyId: "components-sessions-sessionview--provider-failure",
      },
      "failure:not-found": {
        status: "covered",
        storyId: "components-sessions-sessionview--not-found",
      },
      "failure:transient-poll": {
        status: "covered",
        storyId: "components-sessions-sessionview--transient-poll-error",
      },
      "terminal:limit": {
        status: "covered",
        storyId: "components-sessions-sessionview--terminal-limit",
      },
      "delivery:queued-messages": {
        status: "covered",
        storyId: "components-sessions-sessionview--queued-messages",
      },
    },
  ),
  ...[
    ["GoalBanner", "goal-banner"],
    ["ProgressSummary", "progress-summary"],
  ].map(([componentId, storyName]) =>
    coveredDirectLeafTarget(
      componentId,
      "src/components/SessionView.tsx",
      `components-sessions-sessionview--${storyName}`,
      "SessionView direct and keyboard Stories exercise the leaf in its production composition; theme and viewport are verified with Storybook controls.",
    ),
  ),
  withCells(
    coveredBaseTarget(
      "Settings",
      "src/components/Settings.tsx",
      "pages-settings",
    ),
    {
      "data:no-matches": {
        status: "covered",
        storyId: "pages-settings--search-no-matches",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "SettingsAppearance",
      "src/components/SettingsAppearance.tsx",
      "components-settings-appearance",
    ),
    {
      "data:no-matches": {
        status: "covered",
        storyId: "components-settings-appearance--no-matches",
      },
    },
  ),
  ...[
    ["FontRow", "font-row"],
    ["ThemePreview", "theme-preview"],
    ["ToggleRow", "toggle-row"],
  ].map(([componentId, storyName]) =>
    coveredDirectLeafTarget(
      componentId,
      "src/components/SettingsAppearance.tsx",
      `components-settings-appearance--${storyName}`,
      "SettingsAppearance direct and keyboard Stories exercise the leaf in its production composition; theme and viewport are verified with Storybook controls.",
    ),
  ),
  withCells(
    coveredBaseTarget(
      "SettingsArchived",
      "src/components/SettingsArchived.tsx",
      "components-settings-archived",
    ),
    {
      "data:empty": {
        status: "covered",
        storyId: "components-settings-archived--empty",
      },
      "data:no-matches": {
        status: "covered",
        storyId: "components-settings-archived--no-matches",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "SettingsConfiguration",
      "src/components/SettingsConfiguration.tsx",
      "components-settings-configuration",
    ),
    {
      "data:daemon-unavailable": {
        status: "covered",
        storyId: "components-settings-configuration--daemon-unavailable",
      },
      "data:loading-unknown": {
        status: "covered",
        storyId: "components-settings-configuration--loading-unknown",
      },
      "data:no-matches": {
        status: "covered",
        storyId: "components-settings-configuration--no-matches",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "SettingsGeneral",
      "src/components/SettingsGeneral.tsx",
      "components-settings-general",
    ),
    {
      "data:daemon-unavailable": {
        status: "covered",
        storyId: "components-settings-general--daemon-unavailable",
      },
      "data:no-matches": {
        status: "covered",
        storyId: "components-settings-general--no-matches",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "SettingsGit",
      "src/components/SettingsGit.tsx",
      "components-settings-git",
    ),
    {
      "data:custom-template": {
        status: "covered",
        storyId: "components-settings-git--custom-template",
      },
      "data:no-matches": {
        status: "covered",
        storyId: "components-settings-git--no-matches",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "SettingsShortcuts",
      "src/components/SettingsShortcuts.tsx",
      "components-settings-shortcuts",
    ),
    {
      "data:no-matches": {
        status: "covered",
        storyId: "components-settings-shortcuts--no-matches",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "SettingsWorktrees",
      "src/components/SettingsWorktrees.tsx",
      "components-settings-worktrees",
    ),
    {
      "data:empty": {
        status: "covered",
        storyId: "components-settings-worktrees--empty",
      },
      "data:no-matches": {
        status: "covered",
        storyId: "components-settings-worktrees--no-matches",
      },
      "data:pagination": {
        status: "covered",
        storyId: "components-settings-worktrees--pagination",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "Shortcuts",
      "src/components/Shortcuts.tsx",
      "components-navigation-shortcuts",
    ),
    {
      "data:no-matches": {
        status: "covered",
        storyId: "components-navigation-shortcuts--no-matches",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "Sidebar",
      "src/components/Sidebar.tsx",
      "components-navigation-sidebar",
    ),
    {
      "interaction:session-navigation": {
        status: "covered",
        storyId: "components-navigation-sidebar--session-navigation",
      },
      "state:loading": {
        status: "covered",
        storyId: "components-navigation-sidebar--loading",
      },
      "state:empty": {
        status: "covered",
        storyId: "components-navigation-sidebar--empty",
      },
      "connection:checking": {
        status: "covered",
        storyId: "components-navigation-sidebar--connection-checking",
      },
      "connection:offline": {
        status: "covered",
        storyId: "components-navigation-sidebar--connection-offline-restart",
      },
      "project:collapsed": {
        status: "covered",
        storyId: "components-navigation-sidebar--collapsed-project",
      },
      "project:removed-recovery": {
        status: "covered",
        storyId: "components-navigation-sidebar--removed-project-recovery",
      },
      "overflow:current-anchor": {
        status: "covered",
        storyId: "components-navigation-sidebar--overflow-keeps-current-anchor",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "Subagents",
      "src/components/Subagents.tsx",
      "components-supervision-subagents",
    ),
    {
      "data:empty": {
        status: "covered",
        storyId: "components-supervision-subagents--empty",
      },
    },
  ),
  withCells(
    coveredBaseTarget(
      "SupervisionPanel",
      "src/components/SupervisionPanel.tsx",
      "components-supervision-supervisionpanel",
    ),
    {
      "failure:unknown-overflow": {
        status: "covered",
        storyId: "components-supervision-supervisionpanel--failure-unknown-and-overflow",
      },
      "interaction:goal-editing": {
        status: "covered",
        storyId: "components-supervision-supervisionpanel--goal-editing",
      },
      "lifecycle:loading": {
        status: "covered",
        storyId: "components-supervision-supervisionpanel--loading",
      },
      "lifecycle:resting": {
        status: "covered",
        storyId: "components-supervision-supervisionpanel--resting",
      },
    },
  ),
  withCells(
    coveredDirectLeafTarget(
      "EnvironmentSection",
      "src/components/SupervisionPanel.tsx",
      "components-supervision-supervisionpanel--environment-section",
      "SupervisionPanel composition and keyboard Stories exercise the section; theme and viewport are verified with Storybook controls.",
    ),
    {
      "worktree:clean": {
        status: "covered",
        storyId:
          "components-supervision-supervisionpanel--environment-clean-worktree",
      },
      "workspace:in-place": {
        status: "covered",
        storyId:
          "components-supervision-supervisionpanel--environment-in-place-workspace",
      },
      "context:subagent": {
        status: "covered",
        storyId:
          "components-supervision-supervisionpanel--environment-subagent",
      },
      "interaction:commit-menu": {
        status: "covered",
        storyId:
          "components-supervision-supervisionpanel--environment-commit-menu",
      },
    },
  ),
  coveredPrefixedStateTarget(
    "GoalSection",
    "src/components/SupervisionParts.tsx",
    "components-supervision-goal-section",
    "active",
    "editing",
    [
      "paused-self-certified",
      "settled-outcomes",
      "settled-echoed-compact",
    ],
    "src/components/SupervisionGoal.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "ProgressItemRow",
    "src/components/SupervisionParts.tsx",
    "components-supervision-progress",
    "item-states",
    null,
    ["single-completed"],
    "src/components/SupervisionProgress.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "ProgressSection",
    "src/components/SupervisionParts.tsx",
    "components-supervision-progress",
    "checklist-lifecycle",
    null,
    ["item-states", "single-completed"],
    "src/components/SupervisionProgress.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "ArtifactItem",
    "src/components/SupervisionParts.tsx",
    "components-supervision-artifacts",
    "single-artifact-item",
    "file-types-and-overflow",
    [],
    "src/components/SupervisionArtifacts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "ArtifactsSection",
    "src/components/SupervisionParts.tsx",
    "components-supervision-artifacts",
    "file-types-and-overflow",
    "file-types-and-overflow",
    ["single-artifact-item"],
    "src/components/SupervisionArtifacts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "AttentionItem",
    "src/components/SupervisionParts.tsx",
    "components-supervision-attention",
    "interactive-child-item",
    "all-notice-types",
    [],
    "src/components/SupervisionAttention.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "AttentionSection",
    "src/components/SupervisionParts.tsx",
    "components-supervision-attention",
    "all-notice-types",
    "all-notice-types",
    ["interactive-child-item"],
    "src/components/SupervisionAttention.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "BackgroundProcessRow",
    "src/components/SupervisionParts.tsx",
    "components-supervision-panel-status",
    "background-process-item-states",
    null,
    [],
    "src/components/SupervisionStatus.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "BackgroundProcessesSection",
    "src/components/SupervisionParts.tsx",
    "components-supervision-panel-status",
    "background-process-item-states",
    null,
    [],
    "src/components/SupervisionStatus.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "SupervisionAgentsSection",
    "src/components/SupervisionParts.tsx",
    "components-supervision-panel-status",
    "loading-resting-and-agents",
    null,
    [],
    "src/components/SupervisionStatus.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "SupervisionCloseButton",
    "src/components/SupervisionParts.tsx",
    "components-supervision-panel-status",
    "close-and-run-details-actions",
    "close-and-run-details-actions",
    [],
    "src/components/SupervisionStatus.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "SupervisionLoadingState",
    "src/components/SupervisionParts.tsx",
    "components-supervision-panel-status",
    "loading-resting-and-agents",
    null,
    [],
    "src/components/SupervisionStatus.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "SupervisionRestingState",
    "src/components/SupervisionParts.tsx",
    "components-supervision-panel-status",
    "loading-resting-and-agents",
    null,
    [],
    "src/components/SupervisionStatus.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "SupervisionRunDetailsButton",
    "src/components/SupervisionParts.tsx",
    "components-supervision-panel-status",
    "close-and-run-details-actions",
    "close-and-run-details-actions",
    [],
    "src/components/SupervisionStatus.stories.tsx",
  ),
  withCells(
    coveredBaseTarget(
      "TimelineView",
      "src/components/Timeline.tsx",
      "components-timeline-timelineview",
    ),
    {
      "lifecycle:active-streaming": {
        status: "covered",
        storyId: "components-timeline-timelineview--active-streaming",
      },
      "failure:overflow": {
        status: "covered",
        storyId: "components-timeline-timelineview--failure-and-overflow",
      },
    },
  ),
  ...[
    ["ActivityGroup", "activity-group"],
    ["AskDetailView", "ask-detail-view"],
    ["CollapsibleUserText", "collapsible-user-text"],
    ["EditDetailView", "edit-detail-view"],
    ["GlobDetailView", "glob-detail-view"],
    ["GrepDetailView", "grep-detail-view"],
    ["Item", "item"],
    ["JSONDetail", "json-detail"],
    ["MiniDiff", "mini-diff"],
    ["MsgActions", "msg-actions"],
    ["ReadDetailView", "read-detail-view"],
    ["RetriedFold", "retried-fold"],
    ["SemanticDetailView", "semantic-detail-view"],
    ["ShellDetail", "shell-detail"],
    ["SpawnDetailView", "spawn-detail-view"],
    ["Thumbs", "thumbs"],
    ["ToolCard", "tool-card"],
    ["ToolDetail", "tool-detail"],
    ["WebDetailView", "web-detail-view"],
    ["WorkedFold", "worked-fold"],
  ].map(([componentId, storyName]) =>
    coveredDirectLeafTarget(
      componentId,
      "src/components/Timeline.tsx",
      `components-timeline-timelineview--${storyName}`,
      "TimelineView direct, composition, and keyboard Stories exercise the leaf; theme and viewport are verified with Storybook controls.",
    ),
  ),
  withCells(
    coveredBaseTarget(
      "Toasts",
      "src/components/Toasts.tsx",
      "components-feedback-toasts",
      "Toasts",
      "keyboard-dismiss",
    ),
    {
      "interaction:details-expanded": {
        status: "covered",
        storyId: "components-feedback-toasts--details-expanded",
      },
    },
  ),
  coveredPrefixedStateTarget(
    "CommandPaletteItem",
    "src/components/CommandPaletteItem.tsx",
    "components-navigation-command-palette-item",
    "command",
    "keyboard-and-pointer-selection",
    ["selected-command", "session-state-matrix", "scheduled-run"],
  ),
  coveredPrefixedStateTarget(
    "ProjectPicker",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "project-picker-recent",
    "project-picker-filtered",
    [
      "project-picker-no-results",
      "project-picker-new-project",
      "project-picker-no-selection",
    ],
  ),
  coveredPrefixedStateTarget(
    "RunLocationPicker",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "run-location-worktree",
    "run-location-local",
    ["run-location-background", "run-location-unavailable"],
  ),
  coveredPrefixedStateTarget(
    "BranchPicker",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "branch-picker-worktree",
    "branch-picker-local-dirty",
    [
      "branch-picker-no-matches",
      "branch-picker-empty-repo",
      "branch-picker-disabled",
    ],
  ),
  coveredPrefixedStateTarget(
    "AttachmentChip",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "attachment-single-image",
    "attachment-image-and-file",
  ),
  coveredPrefixedStateTarget(
    "AttachmentList",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "attachment-image-and-file",
    "attachment-single-image",
    ["attachment-empty"],
  ),
  coveredPrefixedStateTarget(
    "FileMentionMenu",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "file-mention-results",
    "file-mention-no-matches",
    ["file-mention-unknown-workspace"],
  ),
  coveredPrefixedStateTarget(
    "SlashCommandMenu",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "slash-command-results",
    "slash-command-results",
  ),
  coveredPrefixedStateTarget(
    "AddMenu",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "add-menu-root",
    "add-menu-agents",
    ["add-menu-plan-active", "add-menu-session", "add-menu-automation"],
  ),
  coveredPrefixedStateTarget(
    "AccessPicker",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "access-home-ask",
    "access-session-switchable",
    ["access-home-full", "access-session-unknown"],
  ),
  coveredPrefixedStateTarget(
    "ModelPicker",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "model-picker-summary",
    "model-picker-models",
    ["model-picker-effort", "model-picker-advanced"],
  ),
  coveredPrefixedStateTarget(
    "GoalOptions",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "goal-options-self-certified",
    "goal-options-verifier",
  ),
  coveredPrefixedStateTarget(
    "AssistActions",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "assist-optimize",
    "assist-undo",
    ["assist-optimizing-and-transcribing"],
  ),
  coveredPrefixedStateTarget(
    "DeliveryModeControl",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "delivery-queue",
    "delivery-steer",
  ),
  coveredPrefixedStateTarget(
    "SubmitButton",
    "src/components/ComposerParts.tsx",
    "components-input-composer-parts",
    "submit-ready",
    "submit-stop",
    ["submit-disabled", "submit-running-queue"],
  ),
  coveredPrefixedStateTarget(
    "HomeStarterCard",
    "src/components/HomeParts.tsx",
    "components-home-home-starter-card",
    "default",
    "keyboard-selection",
    ["disabled", "tone-and-copy-matrix"],
    "src/components/HomeStarterCard.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "IntentSuggestionList",
    "src/components/HomeParts.tsx",
    "components-home-intent-suggestion-list",
    "explore",
    "keyboard-selection",
    ["build", "review", "fix", "long-copy-and-single-item"],
    "src/components/IntentSuggestionList.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "RunHeader",
    "src/components/RunParts.tsx",
    "components-runs-run-header",
    "running",
    "keyboard-stop",
    ["lifecycle-matrix", "missing-metadata"],
    "src/components/RunHeader.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "RunLogItem",
    "src/components/RunParts.tsx",
    "components-runs-run-log-item",
    "message",
    null,
    ["event-matrix", "successful-verdict", "failed-verdict"],
    "src/components/RunLogItem.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "RunLogEmptyState",
    "src/components/RunParts.tsx",
    "components-runs-run-log-item",
    "waiting-for-output",
    null,
    [],
    "src/components/RunLogItem.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "ScheduledRunItem",
    "src/components/ScheduledParts.tsx",
    "components-scheduled-parts",
    "run-item-lifecycle-matrix",
    "run-item-keyboard-actions",
    ["run-item-organization-states"],
  ),
  coveredPrefixedStateTarget(
    "ScheduledRunActions",
    "src/components/ScheduledParts.tsx",
    "components-scheduled-parts",
    "run-actions-active",
    "run-actions-paused",
    [
      "run-actions-recoverable",
      "run-actions-settled",
      "run-actions-transient-run",
    ],
  ),
  coveredPrefixedStateTarget(
    "ScheduledToolbar",
    "src/components/ScheduledParts.tsx",
    "components-scheduled-parts",
    "toolbar-state-matrix",
    "toolbar-interaction",
  ),
  coveredPrefixedStateTarget(
    "ScheduledSuggestionCard",
    "src/components/ScheduledParts.tsx",
    "components-scheduled-parts",
    "suggestions-all-cadences",
    "suggestion-selection",
  ),
  coveredPrefixedStateTarget(
    "ScheduledSuggestions",
    "src/components/ScheduledParts.tsx",
    "components-scheduled-parts",
    "suggestions-all-cadences",
    "suggestion-selection",
  ),
  coveredPrefixedStateTarget(
    "ScheduledEmptyState",
    "src/components/ScheduledParts.tsx",
    "components-scheduled-parts",
    "empty-state-matrix",
    null,
  ),
  coveredPrefixedStateTarget(
    "SessionTopbar",
    "src/components/SessionChrome.tsx",
    "components-sessions-sessionchrome",
    "topbar-default",
    "topbar-keyboard-menu",
    [
      "topbar-sub-agent",
      "topbar-retry",
      "topbar-recovery",
      "topbar-changes-view",
      "topbar-overflow-actions",
    ],
  ),
  coveredPrefixedStateTarget(
    "TurnFailureCard",
    "src/components/SessionChrome.tsx",
    "components-sessions-sessionchrome",
    "failure-default",
    "failure-keyboard",
    ["failure-details", "failure-retrying", "failure-without-hint"],
  ),
  coveredPrefixedStateTarget(
    "TerminalAlert",
    "src/components/SessionChrome.tsx",
    "components-sessions-sessionchrome",
    "terminal-danger",
    "terminal-recovery",
    ["terminal-run-limit", "terminal-continue-with-goal"],
  ),
  coveredPrefixedStateTarget(
    "QueuedMessageItem",
    "src/components/SessionChrome.tsx",
    "components-sessions-sessionchrome",
    "queued-long-message",
    "queued-keyboard",
  ),
  coveredPrefixedStateTarget(
    "QueuedMessageList",
    "src/components/SessionChrome.tsx",
    "components-sessions-sessionchrome",
    "queued-messages",
    "queued-keyboard",
    ["queued-empty"],
  ),
  coveredPrefixedStateTarget(
    "SessionNotice",
    "src/components/SessionChrome.tsx",
    "components-sessions-sessionchrome",
    "notice-informational",
    "notice-action",
  ),
  coveredPrefixedStateTarget(
    "ArchivedSessionItem",
    "src/components/SettingsArchivedParts.tsx",
    "components-settings-archived-parts",
    "session-item",
    "session-keyboard-actions",
    ["session-lifecycle-matrix"],
  ),
  coveredPrefixedStateTarget(
    "ArchivedProjectGroup",
    "src/components/SettingsArchivedParts.tsx",
    "components-settings-archived-parts",
    "project-group",
    null,
    ["workspace-less-group"],
  ),
  coveredPrefixedStateTarget(
    "SidebarSessionItem",
    "src/components/SidebarItems.tsx",
    "components-navigation-sidebar-items",
    "session-state-matrix",
    "session-interaction",
  ),
  coveredPrefixedStateTarget(
    "SidebarSessionActions",
    "src/components/SidebarItems.tsx",
    "components-navigation-sidebar-items",
    "session-actions-state-matrix",
    "session-actions-state-matrix",
  ),
  coveredPrefixedStateTarget(
    "SidebarProjectItem",
    "src/components/SidebarItems.tsx",
    "components-navigation-sidebar-items",
    "project-state-matrix",
    "project-state-matrix",
  ),
  coveredPrefixedStateTarget(
    "SidebarProjectActions",
    "src/components/SidebarItems.tsx",
    "components-navigation-sidebar-items",
    "project-actions-state-matrix",
    "project-actions-state-matrix",
  ),
  coveredPrefixedStateTarget(
    "SidebarPreviewCard",
    "src/components/SidebarItems.tsx",
    "components-navigation-sidebar-items",
    "preview-state-matrix",
    null,
  ),
  coveredPrefixedStateTarget(
    "SidebarConnectionStatus",
    "src/components/SidebarItems.tsx",
    "components-navigation-sidebar-items",
    "connection-state-matrix",
    null,
  ),
  coveredPrefixedStateTarget(
    "SubagentItem",
    "src/components/Subagents.tsx",
    "components-supervision-subagent-item",
    "running",
    "keyboard-open",
    [
      "lifecycle-matrix",
      "without-session-or-metrics",
      "long-identity-and-large-usage",
      "nested-children",
    ],
    "src/components/SubagentItem.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "TimelinePendingMessage",
    "src/components/Timeline.tsx",
    "components-timeline-timeline-chrome",
    "pending-queued",
    null,
    ["pending-steering", "pending-attachments-and-long-copy"],
    "src/components/TimelineParts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "TimelineTailActions",
    "src/components/Timeline.tsx",
    "components-timeline-timeline-chrome",
    "tail-actions",
    "tail-actions-with-goal-verdict",
    ["tail-goal-verdict-only"],
    "src/components/TimelineParts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "TimelineJumpToLatest",
    "src/components/Timeline.tsx",
    "components-timeline-timeline-chrome",
    "jump-state-matrix",
    "jump-keyboard-interaction",
    [],
    "src/components/TimelineParts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "TimelineLoadingState",
    "src/components/Timeline.tsx",
    "components-timeline-timeline-chrome",
    "loading",
    null,
    [],
    "src/components/TimelineParts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "TimelineEmptyState",
    "src/components/Timeline.tsx",
    "components-timeline-timeline-chrome",
    "empty",
    null,
    [],
    "src/components/TimelineParts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "ToastItem",
    "src/components/ToastItem.tsx",
    "components-feedback-toast-item",
    "info",
    "keyboard-dismiss",
    ["error", "error-with-details", "long-content"],
  ),
  coveredPrefixedStateTarget(
    "WorktreeCard",
    "src/components/WorktreeCard.tsx",
    "components-settings-worktree-card",
    "multiple-sessions",
    "keyboard-open",
    ["single-session", "long-content"],
  ),
] satisfies StoryManifest;
