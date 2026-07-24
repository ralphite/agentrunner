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

export interface GlobalStatePair {
  pairId: string;
  storyId: string;
  states: readonly string[];
  theme: "light" | "dark";
  viewport: { width: number; height: number };
  evidenceSelector: string;
  reload?: boolean;
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

export interface SemanticStateRequirement {
  componentId: string;
  state: string;
  source: string;
  evidenceSelector: string;
  storyId: string;
  evidence: string;
  owner: string;
}

// Semantic interaction states reveal, replace, or restructure visible UI.
// Unlike generic color-only pseudo states (covered by Storybook's global
// pseudo-state toolbar), each entry here must retain a deterministic Story.
export const semanticStateRequirements: readonly SemanticStateRequirement[] = [
  {
    componentId: "SidebarSessionItem",
    state: "hover-actions-revealed",
    source: "src/tw.css",
    evidenceSelector: ".project-session-wrap:hover .session-quick-actions",
    storyId:
      "components-navigation-sidebar-items--session-quick-actions-reveal",
    evidence: "Hover exposes Pin/Archive without hiding the running indicator.",
    owner: "webui",
  },
  {
    componentId: "SidebarSessionItem",
    state: "focus-within-actions-revealed",
    source: "src/tw.css",
    evidenceSelector:
      ".project-session-wrap:focus-within .session-quick-actions",
    storyId: "components-navigation-sidebar-items--session-quick-actions-focus",
    evidence: "Keyboard focus exposes the same actions as pointer hover.",
    owner: "webui",
  },
  {
    componentId: "SidebarSessionItem",
    state: "hover-worktree-icon-replaced",
    source: "src/tw.css",
    evidenceSelector: ".project-session-wrap:hover .session-worktree-icon",
    storyId:
      "components-navigation-sidebar-items--session-quick-actions-reveal",
    evidence:
      "The secondary worktree icon yields its slot to quick actions while the running spinner remains.",
    owner: "webui",
  },
  {
    componentId: "SidebarSessionItem",
    state: "focus-worktree-icon-replaced",
    source: "src/tw.css",
    evidenceSelector:
      ".project-session-wrap:focus-within .session-worktree-icon",
    storyId: "components-navigation-sidebar-items--session-quick-actions-focus",
    evidence:
      "Keyboard focus performs the same worktree-to-actions slot replacement.",
    owner: "webui",
  },
  {
    componentId: "SidebarProjectItem",
    state: "hover-actions-revealed",
    source: "src/tw.css",
    evidenceSelector: ".project-heading-row:hover .project-heading-actions",
    storyId: "components-navigation-sidebar-items--project-actions-hover",
    evidence: "Hover reveals the project menu and New chat actions.",
    owner: "webui",
  },
  {
    componentId: "SidebarProjectItem",
    state: "focus-within-actions-revealed",
    source: "src/tw.css",
    evidenceSelector:
      ".project-heading-row:focus-within .project-heading-actions",
    storyId:
      "components-navigation-sidebar-items--project-actions-focus-and-menu-open",
    evidence:
      "Keyboard focus reveals actions and the open menu keeps the trigger visible.",
    owner: "webui",
  },
  {
    componentId: "SidebarProjectItem",
    state: "hover-folder-to-caret",
    source: "src/tw.css",
    evidenceSelector: ".project-heading:hover .proj-caret",
    storyId: "components-navigation-sidebar-items--project-actions-hover",
    evidence:
      "Hover replaces the folder glyph with the expansion caret in the same slot.",
    owner: "webui",
  },
  {
    componentId: "SidebarProjectItem",
    state: "focus-folder-to-caret",
    source: "src/tw.css",
    evidenceSelector: ".project-heading:focus-visible .proj-caret",
    storyId:
      "components-navigation-sidebar-items--project-actions-focus-and-menu-open",
    evidence:
      "Focus-visible replaces the folder glyph with the expansion caret.",
    owner: "webui",
  },
  {
    componentId: "MsgActions",
    state: "hover-actions-revealed",
    source: "src/tw.css",
    evidenceSelector:
      ".timeline .tl-inner .msg:not(.msg-last):hover .msg-actions",
    storyId:
      "components-timeline-timelineview--message-actions-hover-and-focus",
    evidence:
      "Earlier messages reveal copy/time actions on hover while the final message remains visible at rest.",
    owner: "webui",
  },
  {
    componentId: "MsgActions",
    state: "focus-within-actions-revealed",
    source: "src/tw.css",
    evidenceSelector:
      ".timeline .tl-inner .msg:not(.msg-last):focus-within .msg-actions",
    storyId: "components-timeline-timelineview--message-actions-focus-within",
    evidence: "Keyboard focus exposes the same earlier-message actions.",
    owner: "webui",
  },
  {
    componentId: "ScheduledRunItem",
    state: "hover-actions-revealed",
    source: "src/tw.css",
    evidenceSelector: ".scheduled-row-wrap:hover .sched-more",
    storyId: "components-scheduled-parts--run-item-action-visibility-states",
    evidence: "Hover exposes the scheduled-run action trigger.",
    owner: "webui",
  },
  {
    componentId: "ScheduledRunItem",
    state: "focus-within-actions-revealed",
    source: "src/tw.css",
    evidenceSelector: ".scheduled-row-wrap:focus-within .sched-more",
    storyId: "components-scheduled-parts--run-item-focus-and-menu-open",
    evidence: "Keyboard focus exposes the scheduled-run action trigger.",
    owner: "webui",
  },
  {
    componentId: "ScheduledRunItem",
    state: "menu-open-actions-persist",
    source: "src/tw.css",
    evidenceSelector: ".scheduled-row-wrap.menu-open .sched-more",
    storyId: "components-scheduled-parts--run-item-focus-and-menu-open",
    evidence: "The trigger remains visible while its context menu is open.",
    owner: "webui",
  },
] satisfies readonly SemanticStateRequirement[];

const BASE_CELLS = ["render:default", "a11y:keyboard"] as const;

const COMPATIBILITY_STORY_SOURCES: Readonly<Record<string, string>> = {
  "src/features/composer/ComposerController.tsx":
    "src/components/Composer.stories.tsx",
  "src/features/composer/ComposerParts.tsx":
    "src/components/ComposerParts.stories.tsx",
  "src/features/session/SessionFeature.tsx":
    "src/components/SessionView.stories.tsx",
  "src/features/session/SessionView.tsx":
    "src/components/SessionView.stories.tsx",
  "src/features/timeline/TimelineFeature.tsx":
    "src/components/Timeline.stories.tsx",
};

function compatibilityStorySource(source: string): string | undefined {
  return COMPATIBILITY_STORY_SOURCES[source];
}

function missingCells(
  extra: readonly string[] = [],
): Record<string, CoverageCell> {
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
    storySource: compatibilityStorySource(source),
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
    storySource: compatibilityStorySource(source),
    exportName: componentId,
    cells: {
      "render:default": {
        status: "covered",
        storyId,
      },
      "a11y:keyboard": {
        status: "n-a",
        reason:
          "Keyboard semantics are owned by the interactive composition or the leaf is inert.",
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
    storySource: storySource ?? compatibilityStorySource(source),
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
            reason:
              "This leaf is informational and has no independently focusable control.",
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
    keyboardStory: keyboardStory ? `${storyPrefix}--${keyboardStory}` : null,
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
    evidence:
      "AppRuntime Story covers the rendered shell while RuntimeController characterization tests cover effects.",
    owner: "webui",
  },
  {
    source: "src/components/ChangesOutcome.tsx",
    declarationName: "PlusMinusSquare",
    reason: "Decorative icon adapter with no independent state or interaction.",
    evidence:
      "Rendered only inside ChangesOutcome controls and hidden from the accessibility tree.",
    owner: "webui",
  },
  {
    source: "src/components/DiffParts.tsx",
    declarationName: "DiffCloseButton",
    reason:
      "Internal DiffToolbar affordance with no standalone product contract.",
    evidence:
      "DiffToolbar Ready, Tight, and State Stories render the close action in both production toolbar variants.",
    owner: "webui",
  },
  ...["AccessIcon", "RiskGlyph"].map((declarationName) => ({
    source: "src/features/composer/ComposerParts.tsx",
    declarationName,
    reason: "Decorative status icon with no independent state or interaction.",
    evidence:
      "AccessPicker Stories cover every labelled access and risk state that selects the icon.",
    owner: "webui",
  })),
  {
    source: "src/features/composer/ComposerParts.tsx",
    declarationName: "PickerBack",
    reason:
      "Internal ModelPicker subpage header; it has no standalone product contract.",
    evidence:
      "ModelPicker Stories exercise the Model, Effort, and Thinking budget subpages and their back interaction.",
    owner: "webui",
  },
  ...["CloudMark", "Telescope"].map((declarationName) => ({
    source: "src/components/Home.tsx",
    declarationName,
    reason:
      "Decorative illustration primitive with no independent product state.",
    evidence:
      "Home owns the visible empty-state composition and accessibility semantics.",
    owner: "webui",
  })),
  ...["CategoryIcon", "StepIcon"].map((declarationName) => ({
    source: "src/features/timeline/TimelineFeature.tsx",
    declarationName,
    reason: "Decorative status icon selected by its owning timeline row.",
    evidence:
      "Tool/Activity Stories cover each status through the complete labelled row.",
    owner: "webui",
  })),
  {
    source: "src/features/timeline/TimelineFeature.tsx",
    declarationName: "TimelineContentView",
    reason:
      "Private render half of TimelineView; it has no state or product contract outside its controller composition.",
    evidence:
      "TimelineView Stories exercise loading, empty, activity, pending, typing, outcome, hover/focus actions, scroll restore and jump states through this exact view.",
    owner: "webui",
  },
  {
    source: "src/components/Scheduled.tsx",
    declarationName: "ScheduledView",
    reason:
      "Private render half of Scheduled; filtering, loading and commands are supplied by useScheduledController.",
    evidence:
      "Scheduled Stories exercise default, loading, empty, filtering, pagination, suggestion, detail, edit, conflict, busy and error states through this exact view.",
    owner: "webui",
  },
] satisfies readonly PrivateVisibleExclusion[];

// CUJs and Demos exercise multiple production targets at once. They belong to
// the same authoritative inventory, but are not component coverage cells and
// therefore carry their own exact Story source/evidence record.
export const workbenchStories = [
  {
    storyId: "cujs-core-session-journeys--configure-new-session",
    source: "src/storybook/cujs/CoreSessionJourneys.stories.tsx",
    kind: "cuj",
    evidence:
      "Fast deterministic journey configures project, intent, request, access, and model through production Home controls.",
    owner: "webui",
  },
  {
    storyId: "cujs-core-session-journeys--start-new-session",
    source: "src/storybook/cujs/CoreSessionJourneys.stories.tsx",
    kind: "cuj",
    evidence:
      "Fast deterministic journey sends the configured request and reaches the production Session shell.",
    owner: "webui",
  },
  {
    storyId: "cujs-core-session-journeys--stream-and-persist-response",
    source: "src/storybook/cujs/CoreSessionJourneys.stories.tsx",
    kind: "cuj",
    evidence:
      "Fast deterministic journey drives scripted stream chunks and the durable poll projection.",
    owner: "webui",
  },
  {
    storyId:
      "cujs-core-session-journeys--inspect-environment-and-completion",
    source: "src/storybook/cujs/CoreSessionJourneys.stories.tsx",
    kind: "cuj",
    evidence:
      "Fast deterministic journey opens the production Environment surface and observes the completion message.",
    owner: "webui",
  },
  {
    storyId: "cujs-core-session-journeys--review-changes-and-return",
    source: "src/storybook/cujs/CoreSessionJourneys.stories.tsx",
    kind: "cuj",
    evidence:
      "Fast deterministic journey completes the session, opens Changes, then closes it and returns to the stable composer.",
    owner: "webui",
  },
  {
    storyId: "demos-scenario-controls--default",
    source: "src/storybook/scenarios/ScenarioControls.stories.tsx",
    kind: "demo",
    evidence:
      "Interactive Play/Next/Reset and speed controls exercise the same ScenarioRunner used by the full demo.",
    owner: "webui",
  },
  {
    storyId: "demos-scenario-controls--all-playback-states",
    source: "src/storybook/scenarios/ScenarioControls.stories.tsx",
    kind: "demo",
    evidence:
      "Deterministic matrix covers idle, running, paused, completed, failed, resetting, and disposed playback controls.",
    owner: "webui",
  },
  {
    storyId: "demos-core-session-playback--demo",
    source: "src/storybook/demos/CoreSessionPlayback.stories.tsx",
    kind: "demo",
    evidence:
      "Production AppRuntime/AppShell journey from Home project and Build intent through configuration, send, deterministic streaming, Environment, completion, Changes, Review, and return to the session; in-canvas transport covers Play/Pause/Next/Replay/Reset/speed/autoplay.",
    owner: "webui",
  },
] satisfies readonly WorkbenchStory[];

// High-risk global dimensions reuse canonical Stories through globals and
// viewport parameters. They are deliberately not Phone/Dark Story copies.
export const globalStatePairs = [
  {
    pairId: "error-dark",
    storyId: "components-sessions-sessionview--provider-failure",
    states: ["error", "dark"],
    theme: "dark",
    viewport: { width: 1280, height: 720 },
    evidenceSelector: ".turn-error",
    evidence: "Provider failure remains readable in dark theme.",
    owner: "webui",
  },
  {
    pairId: "long-content-mobile",
    storyId: "components-input-composer--long-draft-and-attachments",
    states: ["long-content", "mobile"],
    theme: "light",
    viewport: { width: 390, height: 844 },
    evidenceSelector: ".cx-card",
    evidence: "Long composer content remains contained at the phone viewport.",
    owner: "webui",
  },
  {
    pairId: "overlay-short",
    storyId: "components-overlays-modals--prompt-over-main-modal",
    states: ["overlay", "short-viewport"],
    theme: "dark",
    viewport: { width: 390, height: 500 },
    evidenceSelector: '[role="dialog"][aria-label="Rename artifact"]',
    evidence: "Nested modal remains visible and keyboard reachable in a short viewport.",
    owner: "webui",
  },
  {
    pairId: "loading-reload",
    storyId: "components-sessions-sessionview--loading",
    states: ["loading", "reload"],
    theme: "light",
    viewport: { width: 1280, height: 720 },
    evidenceSelector: '[aria-label="Loading conversation"]',
    reload: true,
    evidence: "Session loading projection survives a document reload without an empty shell.",
    owner: "webui",
  },
  {
    pairId: "attention-mobile",
    storyId: "components-sessions-sessionview--approval-required",
    states: ["attention", "mobile"],
    theme: "dark",
    viewport: { width: 390, height: 844 },
    evidenceSelector: ".approval-card",
    evidence: "Approval attention state remains actionable without horizontal overflow on mobile.",
    owner: "webui",
  },
] satisfies readonly GlobalStatePair[];

const baseStoryManifest = [
  {
    componentId: "AppShell",
    source: "src/app/AppShell.tsx",
    storySource: "src/App.stories.tsx",
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
    componentId: "PageHost",
    source: "src/pages/PageHost.tsx",
    exportName: "PageHost",
    cells: {
      "route:home": {
        status: "covered",
        storyId: "pages-pagehost--home-route",
      },
      "route:session": {
        status: "covered",
        storyId: "pages-pagehost--session-route",
      },
      "route:scheduled": {
        status: "covered",
        storyId: "pages-pagehost--scheduled-route",
      },
      "route:run": {
        status: "covered",
        storyId: "pages-pagehost--run-route",
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
    componentId: "Button",
    source: "src/ui/Button.tsx",
    storySource: "src/ui/ActionPrimitives.stories.tsx",
    exportName: "Button",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "foundations-actions-button-and-iconbutton--default",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason:
          "Button delegates keyboard activation to the native button element.",
        evidence:
          "foundations-actions-button-and-iconbutton--default is checked by the Storybook a11y gate and asserts native button semantics.",
        owner: "webui",
      },
      "state:sizes-variants-tones": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--button-sizes-variants-and-tones",
      },
      "state:interaction": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--interaction-states",
      },
      "boundary:long-label": {
        status: "covered",
        storyId: "foundations-actions-button-and-iconbutton--long-label",
      },
      "state:inverse-tone": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--link-semantics-and-inverse-tone",
      },
    },
  },
  {
    componentId: "IconButton",
    source: "src/ui/IconButton.tsx",
    storySource: "src/ui/ActionPrimitives.stories.tsx",
    exportName: "IconButton",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "foundations-actions-button-and-iconbutton--default",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason:
          "IconButton delegates keyboard activation to the native button element and requires an accessible name.",
        evidence:
          "foundations-actions-button-and-iconbutton--default is checked by the Storybook a11y gate and asserts the icon action name.",
        owner: "webui",
      },
      "state:sizes-variants-tones": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--icon-button-sizes-variants-and-tones",
      },
      "state:interaction": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--interaction-states",
      },
      "state:inverse-tone": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--link-semantics-and-inverse-tone",
      },
    },
  },
  {
    componentId: "IconLink",
    source: "src/ui/IconLink.tsx",
    storySource: "src/ui/ActionPrimitives.stories.tsx",
    exportName: "IconLink",
    cells: {
      "render:default": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--link-semantics-and-inverse-tone",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason:
          "IconLink delegates keyboard activation and navigation to the native anchor element.",
        evidence:
          "foundations-actions-button-and-iconbutton--link-semantics-and-inverse-tone asserts native link, accessible-name, and download semantics under the Storybook a11y gate.",
        owner: "webui",
      },
      "state:inverse-tone": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--link-semantics-and-inverse-tone",
      },
      "state:sizes-variants-tones": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--icon-link-sizes-variants-and-tones",
      },
      "state:interaction": {
        status: "covered",
        storyId:
          "foundations-actions-button-and-iconbutton--interaction-states",
      },
    },
  },
  {
    componentId: "Field",
    source: "src/ui/Field.tsx",
    storySource: "src/ui/FieldPrimitives.stories.tsx",
    exportName: "Field",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--input-states",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason:
          "Field supplies labels and descriptions while its native child owns keyboard behavior.",
        evidence:
          "The Input states Story asserts generated required, invalid, disabled, read-only, label, help, and error relationships.",
        owner: "webui",
      },
      "a11y:label-help-error": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--input-states",
      },
      "state:required-disabled-invalid": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--input-states",
      },
      "boundary:long-label": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--input-states",
      },
    },
  },
  {
    componentId: "Input",
    source: "src/ui/Field.tsx",
    storySource: "src/ui/FieldPrimitives.stories.tsx",
    exportName: "Input",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--input-states",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason: "Input preserves native textbox keyboard behavior.",
        evidence:
          "The Input states Story covers empty, value, focus, error, disabled, read-only, and required semantics.",
        owner: "webui",
      },
      "state:interaction": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--input-states",
      },
      "state:value-error-disabled-readonly-required": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--input-states",
      },
    },
  },
  {
    componentId: "Textarea",
    source: "src/ui/Field.tsx",
    storySource: "src/ui/FieldPrimitives.stories.tsx",
    exportName: "Textarea",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--textarea-states",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason: "Textarea preserves native textbox keyboard behavior.",
        evidence:
          "The Textarea states Story covers empty, focus, long text, code, error, disabled, read-only, and required states.",
        owner: "webui",
      },
      "state:interaction": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--textarea-states",
      },
      "state:long-code-error-disabled-readonly-required": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--textarea-states",
      },
    },
  },
  {
    componentId: "Select",
    source: "src/ui/Field.tsx",
    storySource: "src/ui/FieldPrimitives.stories.tsx",
    exportName: "Select",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--select-states",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason: "Select preserves native combobox keyboard behavior.",
        evidence:
          "The Select states Story covers empty, selected, focus, long, error, disabled, and required states.",
        owner: "webui",
      },
      "state:interaction": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--select-states",
      },
      "state:empty-long-error-disabled-required": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--select-states",
      },
    },
  },
  {
    componentId: "SearchField",
    source: "src/ui/Field.tsx",
    storySource: "src/ui/FieldPrimitives.stories.tsx",
    exportName: "SearchField",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--search-states",
      },
      "a11y:composite-actions": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--search-states",
      },
      "state:empty-focus-value-clear-error-disabled": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--search-states",
      },
      "state:default-flush-unstyled": {
        status: "covered",
        storyId: "foundations-forms-field-primitives--control-variants",
      },
    },
  },
  {
    componentId: "StatusIndicator",
    source: "src/ui/StatusIndicator.tsx",
    storySource: "src/ui/StatusPrimitives.stories.tsx",
    exportName: "StatusIndicator",
    cells: {
      "render:default": {
        status: "covered",
        storyId:
          "foundations-feedback-status-and-loading--default",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason: "StatusIndicator is an inert live status, not an input control.",
        evidence:
          "The default Story asserts the named status role and the matrix covers all tones and displays.",
        owner: "webui",
      },
      "state:tones-and-display": {
        status: "covered",
        storyId:
          "foundations-feedback-status-and-loading--tone-and-display-matrix",
      },
      "boundary:long-label": {
        status: "covered",
        storyId:
          "foundations-feedback-status-and-loading--long-label",
      },
    },
  },
  {
    componentId: "Spinner",
    source: "src/ui/Spinner.tsx",
    storySource: "src/ui/StatusPrimitives.stories.tsx",
    exportName: "Spinner",
    cells: {
      "render:default": {
        status: "covered",
        storyId:
          "foundations-feedback-status-and-loading--spinner-inline-and-standalone",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason: "Spinner is an inert loading announcement, not an input control.",
        evidence:
          "The inline/standalone Story asserts aria-busy and named status semantics.",
        owner: "webui",
      },
      "state:sizes": {
        status: "covered",
        storyId:
          "foundations-feedback-status-and-loading--spinner-sizes",
      },
      "state:inline-standalone": {
        status: "covered",
        storyId:
          "foundations-feedback-status-and-loading--spinner-inline-and-standalone",
      },
      "a11y:reduced-motion": {
        status: "covered",
        storyId:
          "foundations-feedback-status-and-loading--spinner-reduced-motion",
      },
    },
  },
  {
    componentId: "FocusScope",
    source: "src/ui/FocusScope.tsx",
    exportName: "FocusScope",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--first-focus-selector",
      },
      "a11y:keyboard": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--tab-and-shift-tab-wrap",
      },
      "focus:ref-target": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--first-focus-ref",
      },
      "interaction:escape": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--escape",
      },
      "focus:restore": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--restore-focus-on-unmount",
      },
      "focus:no-target-fallback": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--no-focusable-fallback",
      },
      "focus:unavailable-targets": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--filters-unavailable-targets",
      },
      "focus:function-root": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--function-root-resolver",
      },
      "focus:suppressed-restore-transfer": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--suppressed-restore-transfer",
      },
      "focus:disconnected-trigger-fallback": {
        status: "covered",
        storyId: "foundations-behavior-focusscope--disconnected-trigger-fallback",
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
      "src/features/composer/ComposerController.tsx",
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
  coveredStateTarget({
    componentId: "ComposerView",
    source: "src/features/composer/ComposerView.tsx",
    renderStory: "components-input-composerview--default",
    keyboardStory: "components-input-composerview--keyboard-navigation",
  }),
  coveredStateTarget({
    componentId: "GoalLoopLauncher",
    source: "src/features/composer/GoalLoopLauncher.tsx",
    storySource: "src/components/Composer.stories.tsx",
    renderStory: "components-input-composer--goal-loop-launcher",
    keyboardStory: "components-input-composer--goal-loop-launcher",
  }),
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
  coveredBaseTarget(
    "Menu",
    "src/components/Menu.tsx",
    "components-overlays-menu",
  ),
  coveredBaseTarget(
    "MenuItem",
    "src/components/Menu.tsx",
    "components-overlays-menu",
  ),
  coveredBaseTarget(
    "MenuLabel",
    "src/components/Menu.tsx",
    "components-overlays-menu",
  ),
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
  withCells(target("Modal", "src/components/Modals.tsx"), {
    "render:default": {
      status: "covered",
      storyId: "components-overlays-modals--standalone-default",
    },
    "a11y:keyboard": {
      status: "covered",
      storyId: "components-overlays-modals--standalone-keyboard-navigation",
    },
  }),
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
  coveredPrefixedStateTarget(
    "ModelFields",
    "src/components/Modals.tsx",
    "components-overlays-modals",
    "model-fields-default",
    "model-fields-keyboard-navigation",
    ["model-fields-custom-model"],
  ),
  ...[
    "MainModal",
    "PromptModal",
    "RenameModal",
    "RunDetailsModal",
    "ViewerModal",
  ].map((componentId) =>
    coveredInteractiveLeafTarget(
      componentId,
      "src/components/Modals.tsx",
      "components-overlays-modals",
    ),
  ),
  ...[
    ["ConfirmModal", "confirm-modal"],
    ["ForkModal", "fork-modal"],
    ["NewSessionModal", "new-session-modal"],
    ["RunModal", "run-modal"],
  ].map(([componentId, storyName]) =>
    withCells(
      coveredInteractiveLeafTarget(
        componentId,
        "src/components/Modals.tsx",
        "components-overlays-modals",
      ),
      {
        "state:busy": {
          status: "covered",
          storyId: `components-overlays-modals--${storyName}-busy`,
        },
        "failure:action": {
          status: "covered",
          storyId: `components-overlays-modals--${storyName}-failure`,
        },
      },
    ),
  ),
  ...[
    ["AgentModal", "agent-modal"],
    ["TrustModal", "trust-modal"],
  ].map(([componentId, storyName]) =>
    withCells(
      coveredInteractiveLeafTarget(
        componentId,
        "src/components/Modals.tsx",
        "components-overlays-modals",
      ),
      {
        "state:busy": {
          status: "covered",
          storyId: `components-overlays-modals--${storyName}-busy`,
        },
        "failure:action": {
          status: "covered",
          storyId: `components-overlays-modals--${storyName}-failure`,
        },
      },
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
  coveredBaseTarget(
    "Popover",
    "src/components/Popover.tsx",
    "components-overlays-popover",
  ),
  coveredBaseTarget(
    "PopSection",
    "src/components/Popover.tsx",
    "components-overlays-popover",
  ),
  coveredBaseTarget(
    "PopItem",
    "src/components/Popover.tsx",
    "components-overlays-popover",
  ),
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
      "src/features/session/SessionView.tsx",
      "components-sessions-sessionview",
    ),
    {
      "state:loading": {
        status: "covered",
        storyId: "components-sessions-sessionview--loading",
      },
      "state:running": {
        status: "covered",
        storyId: "components-sessions-sessionview--running",
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
  coveredDirectLeafTarget(
    "SessionFeature",
    "src/features/session/SessionFeature.tsx",
    "components-sessions-sessionview--default",
    "SessionFeature owns runtime orchestration while the SessionView Stories exercise it through the production compatibility entry point.",
  ),
  ...[
    ["GoalBanner", "goal-banner"],
    ["ProgressSummary", "progress-summary"],
  ].map(([componentId, storyName]) =>
    coveredDirectLeafTarget(
      componentId,
      "src/features/session/SessionView.tsx",
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
        storyId:
          "components-supervision-supervisionpanel--failure-unknown-and-overflow",
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
    ["paused-self-certified", "settled-outcomes", "settled-echoed-compact"],
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
      "TimelineFeature",
      "src/features/timeline/TimelineFeature.tsx",
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
      "src/features/timeline/TimelineFeature.tsx",
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
    "src/features/composer/ComposerParts.tsx",
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
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "run-location-worktree",
    "run-location-local",
    ["run-location-background", "run-location-unavailable"],
  ),
  coveredPrefixedStateTarget(
    "BranchPicker",
    "src/features/composer/ComposerParts.tsx",
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
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "attachment-single-image",
    "attachment-image-and-file",
  ),
  coveredPrefixedStateTarget(
    "AttachmentList",
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "attachment-image-and-file",
    "attachment-single-image",
    ["attachment-empty"],
  ),
  coveredPrefixedStateTarget(
    "FileMentionMenu",
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "file-mention-results",
    "file-mention-no-matches",
    ["file-mention-unknown-workspace"],
  ),
  coveredPrefixedStateTarget(
    "SlashCommandMenu",
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "slash-command-results",
    "slash-command-results",
  ),
  coveredPrefixedStateTarget(
    "AddMenu",
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "add-menu-root",
    "add-menu-agents",
    ["add-menu-plan-active", "add-menu-session", "add-menu-automation"],
  ),
  coveredPrefixedStateTarget(
    "AccessPicker",
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "access-home-ask",
    "access-session-switchable",
    ["access-home-full", "access-session-unknown"],
  ),
  coveredPrefixedStateTarget(
    "ModelPicker",
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "model-picker-summary",
    "model-picker-models",
    ["model-picker-effort", "model-picker-advanced"],
  ),
  coveredPrefixedStateTarget(
    "GoalOptions",
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "goal-options-self-certified",
    "goal-options-verifier",
  ),
  coveredPrefixedStateTarget(
    "AssistActions",
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "assist-optimize",
    "assist-undo",
    ["assist-optimizing-and-transcribing"],
  ),
  coveredPrefixedStateTarget(
    "DeliveryModeControl",
    "src/features/composer/ComposerParts.tsx",
    "components-input-composer-parts",
    "delivery-queue",
    "delivery-steer",
  ),
  coveredPrefixedStateTarget(
    "SubmitButton",
    "src/features/composer/ComposerParts.tsx",
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
    "src/features/timeline/TimelineFeature.tsx",
    "components-timeline-timeline-chrome",
    "pending-queued",
    null,
    ["pending-steering", "pending-attachments-and-long-copy"],
    "src/components/TimelineParts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "TimelineTailActions",
    "src/features/timeline/TimelineFeature.tsx",
    "components-timeline-timeline-chrome",
    "tail-actions",
    "tail-actions-with-goal-verdict",
    ["tail-goal-verdict-only"],
    "src/components/TimelineParts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "TimelineJumpToLatest",
    "src/features/timeline/TimelineFeature.tsx",
    "components-timeline-timeline-chrome",
    "jump-state-matrix",
    "jump-keyboard-interaction",
    [],
    "src/components/TimelineParts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "TimelineLoadingState",
    "src/features/timeline/TimelineFeature.tsx",
    "components-timeline-timeline-chrome",
    "loading",
    null,
    [],
    "src/components/TimelineParts.stories.tsx",
  ),
  coveredPrefixedStateTarget(
    "TimelineEmptyState",
    "src/features/timeline/TimelineFeature.tsx",
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

// State Stories added during the interaction/boundary audit stay attached to
// the production component that owns the state. Keeping this delta separate
// from the long-lived base inventory makes review straightforward while the
// closure lint still treats every entry as a normal component coverage cell.
const additionalStateStoriesByComponent: Record<string, readonly string[]> = {
  ApprovalCard: [
    "components-attention-approvalcard--deny-reason-open",
    "components-attention-approvalcard--busy-decision",
    "components-attention-approvalcard--long-content-and-gates",
  ],
  AskForm: [
    "components-attention-askform--free-text-only",
    "components-attention-askform--busy-submitting",
    "components-attention-askform--empty-questions",
    "components-attention-askform--long-question-and-options",
    "components-attention-askform--semantic-pseudo-states",
  ],
  ChangesOutcome: ["components-changes-changesoutcome--image-lightbox-open"],
  CommandPalette: [
    "components-navigation-commandpalette--pointer-selection",
    "components-navigation-commandpalette--keyboard-selection",
    "components-navigation-commandpalette--search-results-with-archived",
    "components-navigation-commandpalette--no-matches",
    "components-navigation-commandpalette--attention-overflow",
  ],
  Composer: [
    "components-input-composer--long-draft-and-attachments",
    "components-input-composer--dragging-files",
    "components-input-composer--busy-pending-send",
    "components-input-composer--file-mention-keyboard",
    "components-input-composer--slash-command-keyboard-wrap-and-escape",
  ],
  GoalLoopLauncher: [
    "components-input-composer--goal-loop-mode-matrix",
    "components-input-composer--goal-loop-invalid-interval",
    "components-input-composer--goal-loop-empty-and-busy",
  ],
  ProjectPicker: [
    "components-input-composer-parts--project-picker-page-flow-keyboard",
    "components-input-composer-parts--project-picker-long-overflow",
  ],
  RunLocationPicker: [
    "components-input-composer-parts--run-location-background-worktree",
    "components-input-composer-parts--run-location-keyboard-selection",
  ],
  BranchPicker: [
    "components-input-composer-parts--branch-picker-long-narrow-overflow",
    "components-input-composer-parts--branch-picker-dialog-keyboard",
  ],
  AttachmentList: [
    "components-input-composer-parts--attachment-long-and-wrapping",
  ],
  FileMentionMenu: [
    "components-input-composer-parts--file-mention-pointer-and-overflow",
  ],
  SlashCommandMenu: [
    "components-input-composer-parts--slash-command-pointer-and-overflow",
  ],
  AddMenu: [
    "components-input-composer-parts--add-menu-goal-active",
    "components-input-composer-parts--add-menu-selected-persona",
    "components-input-composer-parts--add-menu-page-flow-keyboard",
  ],
  AccessPicker: [
    "components-input-composer-parts--access-closed-state-matrix",
    "components-input-composer-parts--access-home-keyboard-selection",
  ],
  ModelPicker: [
    "components-input-composer-parts--model-picker-custom-long-summary",
    "components-input-composer-parts--model-picker-page-flow-keyboard",
  ],
  GoalOptions: [
    "components-input-composer-parts--goal-options-long-boundary",
    "components-input-composer-parts--goal-options-exit-keyboard",
  ],
  AssistActions: [
    "components-input-composer-parts--assist-listening",
    "components-input-composer-parts--assist-hidden",
  ],
  DeliveryModeControl: [
    "components-input-composer-parts--semantic-control-pseudo-states",
  ],
  SubmitButton: [
    "components-input-composer-parts--submit-running-steer",
    "components-input-composer-parts--semantic-control-pseudo-states",
  ],
  Thumbs: [
    "components-timeline-timelineview--thumbs-unavailable",
  ],
  ContextMenu: ["components-overlays-contextmenu--viewport-edge-long-content"],
  ChangedFilesMenu: ["components-changes-diffparts--changed-files-no-matches"],
  DiffMoreActionsMenu: ["components-changes-diffparts--more-actions-empty"],
  CommitPushMenu: ["components-changes-diffparts--commit-busy"],
  UntrackedFile: ["components-changes-diffview--untracked-file-loading"],
  FileBody: ["components-changes-diffview--file-body-context-loading"],
  HomeStarterCard: [
    "components-home-home-starter-card--semantic-pseudo-states",
  ],
  IntentSuggestionList: [
    "components-home-intent-suggestion-list--many-suggestions",
    "components-home-intent-suggestion-list--semantic-pseudo-states",
  ],
  Lightbox: [
    "components-media-lightbox--single-image",
    "components-media-lightbox--maximum-zoom",
    "components-media-lightbox--image-unavailable",
  ],
  ImageCard: [
    "components-changes-changesoutcome--image-card-unavailable",
  ],
  CodeBlock: ["components-content-markdown--plain-code-block"],
  MdImage: ["components-content-markdown--md-image-failure"],
  Menu: [
    "components-overlays-menu--closed",
    "components-overlays-menu--keyboard-wrap-and-selection-return",
    "components-overlays-menu--long-overflow",
    "components-overlays-menu--semantic-pseudo-states",
  ],
  Popover: [
    "components-overlays-popover--keyboard-wrap-skip-and-selection-return",
    "components-overlays-popover--dialog-autofocus",
    "components-overlays-popover--downward-overflow",
  ],
  PopItem: ["components-overlays-popover--pop-item-state-matrix"],
  RunHeader: ["components-runs-run-header--running-without-stop"],
  RunLogItem: ["components-runs-run-log-item--iteration-verdict"],
  ScheduledRunItem: [
    "components-scheduled-parts--run-item-action-visibility-states",
    "components-scheduled-parts--run-item-focus-and-menu-open",
  ],
  SessionTopbar: [
    "components-sessions-sessionchrome--topbar-read-only-sub-agent",
  ],
  TerminalAlert: ["components-sessions-sessionchrome--terminal-tone-matrix"],
  GoalBanner: [
    "components-sessions-sessionview--goal-terminal-tone-matrix",
    "components-sessions-sessionview--goal-update-pending",
  ],
  SettingsAppearance: [
    "components-settings-appearance--diff-marker-selection",
    "components-settings-appearance--global-type-scale-application",
  ],
  SettingsConfiguration: ["components-settings-configuration--long-paths"],
  SettingsWorktrees: [
    "components-settings-worktrees--filtered-results",
    "components-settings-worktrees--pagination-collapsed",
  ],
  Sidebar: [
    "components-navigation-sidebar--resize-handle-keyboard",
    "components-navigation-sidebar--resize-handle-hover",
    "components-navigation-sidebar--resize-handle-focus-visible",
    "components-navigation-sidebar--resize-handle-dragging",
    "components-navigation-sidebar--scheduled-unread-notice",
    "components-navigation-sidebar--folded-sections",
    "components-navigation-sidebar--footer-menu-open",
    "components-navigation-sidebar--session-context-menu-open",
    "components-navigation-sidebar--archived-visibility",
    "components-navigation-sidebar--history-loading",
    "components-navigation-sidebar--project-group-overflow",
    "components-navigation-sidebar--workspace-less-session-overflow",
  ],
  SidebarSessionItem: [
    "components-navigation-sidebar-items--session-quick-actions-reveal",
    "components-navigation-sidebar-items--session-quick-actions-focus",
  ],
  SidebarProjectItem: [
    "components-navigation-sidebar-items--project-actions-hover",
    "components-navigation-sidebar-items--project-actions-focus-and-menu-open",
  ],
  SidebarProjectActions: [
    "components-navigation-sidebar-items--project-without-workspace-actions",
  ],
  MsgActions: [
    "components-timeline-timelineview--message-actions-hover-and-focus",
    "components-timeline-timelineview--message-actions-focus-within",
    "components-timeline-timelineview--msg-actions-busy-and-error",
  ],
  ToolCard: ["components-timeline-timelineview--tool-lifecycle-matrix"],
  ToastItem: ["components-feedback-toast-item--long-details-overflow"],
  WorktreeCard: ["components-settings-worktree-card--empty-sessions"],
  Home: [
    "pages-home--project-aware-long-headline",
    "pages-home--draft-without-intent",
  ],
  Scheduled: ["pages-scheduled--pagination"],
  ScheduleDetailPanel: [
    "pages-scheduled--schedule-detail-fallbacks",
    "pages-scheduled--schedule-detail-saving",
  ],
  ScheduleEditDialog: [
    "pages-scheduled--schedule-edit-cron-conflict",
    "pages-scheduled--schedule-edit-busy",
  ],
  Settings: ["pages-settings--initial-appearance-section"],
};

export const storyManifest = baseStoryManifest.map((component) => {
  const stories =
    additionalStateStoriesByComponent[component.componentId] ?? [];
  return withCells(
    component,
    Object.fromEntries(
      stories.map((storyId) => [
        `state:${storyId.split("--")[1]}`,
        { status: "covered" as const, storyId },
      ]),
    ),
  );
}) satisfies StoryManifest;
