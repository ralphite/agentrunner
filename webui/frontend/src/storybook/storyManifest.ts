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

export type StoryReviewAxis =
  | "role-name-state"
  | "keyboard-focus"
  | "pointer-touch"
  | "disabled-busy-error"
  | "live-region"
  | "motion"
  | "contrast-theme"
  | "zoom-overflow";

export type StoryReviewVerdict =
  | "ALIGNED"
  | "FIXED"
  | "GAP"
  | "INTENTIONAL";
export type CodexParityVerdict =
  | "PASS"
  | "UNTESTED"
  | "GAP"
  | "INTENTIONAL";

export interface StoryReviewFamily {
  reviewId: string;
  titlePrefix: string;
  axes: readonly StoryReviewAxis[];
  visualVerdict: StoryReviewVerdict;
  codexParity: CodexParityVerdict;
  decision: string;
  codexEvidence: string;
  agentEvidence: string;
  reviewedBy: readonly (
    | "visual-design"
    | "interaction-a11y"
    | "contract-evidence"
  )[];
  reviewedAt: string;
  reviewedDigest: string;
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

const ALL_STORY_REVIEW_AXES = [
  "role-name-state",
  "keyboard-focus",
  "pointer-touch",
  "disabled-busy-error",
  "live-region",
  "motion",
  "contrast-theme",
  "zoom-overflow",
] as const satisfies readonly StoryReviewAxis[];

// These digests are reviewer approvals, not generated baselines. The lint gate
// computes the current family digest from the exact built Story entries, Story
// sources, production sources, and shared visual foundations. Baseline update
// never rewrites this map: any drift requires a fresh review and an explicit
// digest edit.
const REVIEWED_FAMILY_DIGESTS: Readonly<Record<string, string>> = {
  "components-attention":
    "7baf8fa2c36c789615fc68c304b28ca0af93531333cd9b4f39aa6ce8de276725",
  "components-changes":
    "9856292feeb0284d5929eb2694f0376d1b9562eaf9a874a46930a9558846b131",
  "components-content":
    "b0e7ce69b5733774977f2d6a5d4532634b600225075ddcd790d7db034ac3a6e9",
  "components-feedback":
    "a1042f0cf51dd78488ef0e90753d07dea58f0c59e6145339dfd7aececc3f1920",
  "components-home":
    "bd0c3cf78592f913a98af5dece08895bd229ab4bf7ccafa390ef91ee167d8435",
  "components-input":
    "9e46c322ff6686c7d4b4fac83a9e8d687744f58546e31c5d7a73b5b2df5e74cd",
  "components-media":
    "f4dbf3dbbcbf139505ef2d7fe4d379863c4a572b0259026f4ca81300359bee26",
  "components-navigation":
    "0d8ff799f81c7b8d531e4f2b9d63c7f4940762cdf2bac389b4bc3875da45f578",
  "components-overlays":
    "9a65eeaebe7e44c447f1dafb866033f4fff4ad6c9e9ce1825f60000efc8e2649",
  "components-runs":
    "c5fc41c13661700b616f01b116be4b5fd509bc8fee2be2e2447bb961d09c40e6",
  "components-scheduled":
    "877488467d9cedb7d0dc0495443cb610efc1107515440831e5d2e9a913ea60e8",
  "components-sessions":
    "af04487d40ee816aff5bd085e27e2156cf1e09372e2439cbeeed271671980518",
  "components-settings":
    "003baa9b4139375c3375213ff1ef9122281506faf77dbfc2429c03fd92c4ec39",
  "components-supervision":
    "d5adc7b0485fa67959134a89249b7dc0963502577424b4d21adadf2aa9500dbc",
  "components-timeline":
    "ba5c8aee5a2a6887cfacebc79ac2cf708d1a9f41f23fe7dd933e53f7b0faee17",
  "cujs-core-session":
    "af36cde691bc552c9056e073110bd9dccc235e1dcafaaa5e133521caff46f7f9",
  "demos-core-session":
    "dbdee2b132f239faa0028ce18b87086a3c00b57c528d65789e1fd8fca07e0fd3",
  "demos-scenario-controls":
    "9fa2fbbe7ca32889676acf000a978f2170a17a01fd8bc837122cd6141a84b526",
  "foundations-actions":
    "8b30ada6ab179fc9f8eb32278d5ca50c385bbdbafaf7f7c4b0ded3cf03de2f9f",
  "foundations-behavior":
    "f2aa721040968463b31729ec87d486fdf4db3bb2b76631fcae890567714cd23f",
  "foundations-feedback":
    "24c5dbef33888c1acac30123c87eb512538e9de9a76ebc2dcfe58fbe4358d613",
  "foundations-forms":
    "28279096e96e3143ee0af649c7cd2ca80755f9a28779936bf292ad6b09325f76",
  "pages-appruntime":
    "298ab2a41fb41b54bdca3cc6f0ee9a8d906588e913048ad262b0352ac959453a",
  "pages-appshell":
    "6d93ac29e1d4a07e3ec5f9af493bedece2ee33042a43bcd1f7eb68db60c2a76c",
  "pages-home":
    "43d40e9289089105b6c933d96bf60c54e2976d36e1a079a348f78500e45795af",
  "pages-pagehost":
    "31ba558d1e1bf0b84344edc789a08db62a9fb123f1398c302058c370999afb41",
  "pages-scheduled":
    "5f74620f0bacac0a8d8ad61e98ea711e34ecd87fb98e813b10addccd681f0dcb",
  "pages-settings":
    "5bb814e73ef3a152f1c5abe62fdcc58ba7dd70e5edab566a453194c0e6cba559",
};

function reviewedFamily(
  reviewId: string,
  titlePrefix: string,
  visualVerdict: StoryReviewVerdict,
  codexParity: CodexParityVerdict,
  decision: string,
  codexEvidence: string,
  agentEvidence: string,
  reviewedAt = "2026-07-23",
): StoryReviewFamily {
  return {
    reviewId,
    titlePrefix,
    axes: ALL_STORY_REVIEW_AXES,
    visualVerdict,
    codexParity,
    decision,
    codexEvidence,
    agentEvidence,
    reviewedBy: [
      "visual-design",
      "interaction-a11y",
      "contract-evidence",
    ],
    reviewedAt,
    reviewedDigest: REVIEWED_FAMILY_DIGESTS[reviewId] ?? "",
    owner: "webui",
  };
}

// INC-101 review ledger. Each built Story must resolve to exactly one of these
// manually decided families. The lint gate joins this catalogue with the built
// index and every manifest cell, then writes the exact per-Story/per-target
// ledger to storybook-missing-baseline.json. A new title cannot inherit a PASS:
// it is an orphan until a reviewer deliberately assigns and decides its family.
export const storyReviewFamilies = [
  reviewedFamily(
    "foundations-actions",
    "Foundations/Actions",
    "FIXED",
    "UNTESTED",
    "Buttons and icon actions now share semantic control geometry, quiet hover treatment, and one focus-visible contract; ordinary controls no longer gain elevation.",
    "Codex has no public component-library state grid; exact component parity remains UNTESTED.",
    "QA-92 Action primitives Stories plus production Button/IconButton/IconLink review.",
  ),
  reviewedFamily(
    "foundations-forms",
    "Foundations/Forms",
    "ALIGNED",
    "UNTESTED",
    "Field, select, checkbox, helper, disabled, invalid, and focus states were reviewed against the shared control/type/radius tokens without adding Story-only styling.",
    "No same-state Codex field matrix is available; parity remains UNTESTED.",
    "QA-92 Field primitives family and final interaction/a11y review.",
  ),
  reviewedFamily(
    "foundations-behavior",
    "Foundations/Behavior",
    "ALIGNED",
    "UNTESTED",
    "Focus containment, initial focus, tab order, restore, nested scope, and empty-scope fallbacks retain their production interaction contract. Escape ownership now uses its own top-layer stack, so temporary cursor ContextMenus and anchored Popover/Menus can dismiss above a parent Tab-trapping scope while a subsequently opened modal still takes precedence.",
    "Codex focus internals are not observable as a complete same-state matrix; parity remains UNTESTED.",
    "FocusScope ten-state Story family, ContextMenu/Popover interaction coverage, and real mobile AppShell nested-layer review through both pointer ContextMenu and touch More Popover/Menu: Escape from the active item closes only the temporary menu, preserves the parent sidebar, and returns the exact durable invoker; the top menu layer makes the parent Tab trap yield so Tab closes and hands off to the next page-order control; explicit durable return focus survives Rename modal close. Nested modal and standalone FocusScope behavior remain green.",
  ),
  reviewedFamily(
    "foundations-feedback",
    "Foundations/Feedback",
    "FIXED",
    "UNTESTED",
    "Lifecycle, spinner, progress, loading, and reduced-motion glyphs use one semantic status language with live-region ownership kept at composition boundaries.",
    "Codex status glyphs informed the shared language, but no complete same-state matrix exists; parity remains UNTESTED.",
    "QA-92 screenshots 04 and LifecycleStatus/Status primitives Story families.",
  ),
  reviewedFamily(
    "components-attention",
    "Components/Attention",
    "FIXED",
    "UNTESTED",
    "Approval, structured question, and daemon attention surfaces preserve safety hierarchy, 44px mobile actions, readable dark contrast, and labelled status without changing decisions.",
    "Approval evidence is not a complete same-state Codex matrix; parity remains UNTESTED.",
    "QA-92 screenshots 18-19, measured phone targets, and Approval/AskForm/DaemonAlert Stories.",
  ),
  reviewedFamily(
    "components-changes",
    "Components/Changes",
    "ALIGNED",
    "GAP",
    "Diff and artifact surfaces inherit the shared typography, border, focus, overflow, and action primitives; INC-101 deliberately avoids a second Story-only diff layout. Undo remains a neutral quiet action at rest, reserving danger emphasis for the destructive confirmation step.",
    "CODEX-PARITY G13/Changes rows retain product workflow GAP; current-source Changes is explicitly UNTESTED in QA-92.",
    "ChangesOutcome, DiffParts, and DiffView Story families plus contract review and R82 golden/live probe: Undo changed from resting red to neutral ink while its confirmed destructive workflow remains unchanged.",
  ),
  reviewedFamily(
    "components-navigation",
    "Components/Navigation",
    "GAP",
    "UNTESTED",
    "Desktop primary navigation now shares the rail's 32px/13px row rhythm, using icons and weight rather than oversized rows for hierarchy; mobile and coarse-pointer navigation keeps 44px touch height. At mobile or coarse-input breakpoints, each session row exposes one stable 44×44 More action without selecting the session. Its viewport-clamped menu preserves Pin, Rename, Mark read/unread, and Archive actions, meaningful focus handoff and return, long-title context, and running, worktree, unread, and attention states. Desktop retains quiet hover quick actions and the context-menu path without a persistent ellipsis. Pointer/keyboard ContextMenu and touch More Popover/Menu entries name the action surface, use enabled-only roving focus and Tab handoff, and return Escape, ordinary selections, and closed destination dialogs to the durable invoker. These navigation rhythm, touch-action, and temporary-menu focus defects are fixed, while other navigation-family gaps remain, so the family stays GAP.",
    "CODEX-PARITY GL-03/GL-07 pass selected interactions, while GL-05/GL-06 remain UNTESTED; the family therefore remains UNTESTED.",
    "Fresh cumulative visual and accessibility review of components-navigation-sidebar-items--session-interaction (“Mobile session actions”) at 390×844 plus 04-real-sidebar-context-390x500.png, R82 desktop rail evidence, and the production Sidebar/ContextMenu callsites: desktop primary nav and session rows share 32px/13px rhythm; a latest-main 390×500 live probe confirms both primary nav rows remain 44px/13px on mobile with zero overflow. Session row and touch trigger measure 44×44; the 220px panel remains viewport-contained; both the cursor ContextMenu and touch More Popover/Menu expose the full session-title aria-label, dismiss preview, and give Pin the unique enabled roving focus. The real Pages/AppShell run verifies both entry paths: Escape from the active menuitem closes only the temporary menu and returns the exact invoker, Tab closes and advances to the next page-order target (the neighboring session or footer More options, depending on the invoking row), and Rename dialog Escape returns to that same invoker while the mobile sidebar stays open throughout; ordinary selections retain the same durable return. Console warnings/errors remain zero. Running, worktree, unread, and attention states remain visible. Independent code and design reviewers APPROVE with scoped P0/P1/P2=0.",
    "2026-07-24",
  ),
  reviewedFamily(
    "components-input",
    "Components/Input",
    "GAP",
    "UNTESTED",
    "Model Picker and Project Picker compact-canvas containment are fixed. Project Picker now keeps search and footer actions visible around one independently scrolling result list, uses 44px mobile/coarse-pointer targets, contains long CJK paths, and preserves open, Back, selection, and Escape focus return. The family remains GAP: several other Composer controls miss 44px touch geometry, long attachments can crowd out the action bar, and async failure states need honest recovery UI.",
    "CODEX-PARITY NS input rows contain mixed PASS/GAP/UNTESTED states; no family-wide PASS is claimed.",
    "Fresh Project Picker design, interaction, and code review at 390x500, 390x844, and 1280x800: panel margins stay >=8px; search, trigger, New project, projectless, and Back meet 44px on mobile; panel scroll is zero while the 14-row list scrolls; long English/CJK labels truncate; document overflow and browser console errors are zero; Storybook 67/67 and shared-runtime focus-chain checks pass. Evidence: 11-after-component-local-390x500.png, 10-after-long-overflow-390x844.png, 12-after-desktop-recent-1280x800.png, 13-real-shared-runtime-390x500.png. Fresh visual and code reviewers APPROVE with P0/P1/P2=0.",
    "2026-07-24",
  ),
  reviewedFamily(
    "components-overlays",
    "Components/Overlays",
    "GAP",
    "UNTESTED",
    "Model and Project Picker containment remain fixed. Anchored Popover, Menu, and cursor ContextMenu now share one focus contract: pointer and keyboard entry focus the first available item; exactly one enabled item owns roving focus; hidden and disabled content is skipped; Tab exits relative to the durable invoker; and Escape or ordinary selection restores meaningful focus without stealing it from a destination dialog. ContextMenu additionally names its surface, keeps internal long-menu scrolling open, clamps to 8px viewport gutters, and uses 44px mobile/coarse actions. The family remains GAP because dialog-popover edge containment and 44px comfort across every generic mobile anchored overlay are not yet complete.",
    "Only selected overlay journeys have same-state Codex evidence; family parity remains UNTESTED.",
    "Fresh final Product Design/UX review of 01-after-default-390x500.png through 05-real-scheduled-context-390x500.png plus ContextMenu, Menu, menuFocus, Popover, and the production Sidebar/Scheduled entry points: the Codex-like 220px hierarchy, 12px radius, 13px actions, 12px labels, token contrast, visible focus, disabled treatment, and danger hierarchy are coherent; mobile/coarse ContextMenu items hold at least 44px, long menus scroll internally and remain within 8px viewport gutters, and real Sidebar/Scheduled menus preserve context. Exactly one available item owns roving focus; hidden, disabled, inert, CSS-hidden, and closed-details rows are excluded; Tab/Shift+Tab honor positive tabindex and checked-radio order; dynamic removal/loading and empty-menu exit recover; plain selections restore the durable invoker; and destination FocusScope retains focus before returning to that invoker on close. A separate top-layer Escape stack lets the cursor menu and anchored Popover/Menu dismiss above a parent mobile-sidebar FocusScope, while the menu's non-trapping top focus layer makes the parent yield Tab and nested modals retain precedence; explicit ephemeral return targets survive StrictMode remounts. Real Pages/AppShell at 390×500 and integration coverage verify both pointer ContextMenu and touch More Popover/Menu through menuitem Escape, Tab, and Rename-dialog Escape without closing the sidebar or losing focus. Independent code and design reviewers APPROVE with scoped P0/P1/P2=0.",
    "2026-07-24",
  ),
  reviewedFamily(
    "components-feedback",
    "Components/Feedback",
    "ALIGNED",
    "UNTESTED",
    "Error, not-found, toast, long-detail, busy, dismiss, and stacked states retain clear tone, role/name, focus, and overflow contracts under shared tokens.",
    "No complete same-state Codex feedback grid exists; parity remains UNTESTED.",
    "ErrorBoundary, SessionNotFound, ToastItem, and Toasts Story families.",
  ),
  reviewedFamily(
    "components-home",
    "Components/Home",
    "FIXED",
    "UNTESTED",
    "Starter and suggestion surfaces use flat quiet cards, consistent spacing/type, keyboard clearing, and long-text containment without Story-only shadows.",
    "CODEX-PARITY NS-01/NS-04 cover selected Home states, not every component variant; family parity remains UNTESTED.",
    "Home component Stories and QA-92 current-source Home review.",
  ),
  reviewedFamily(
    "components-media",
    "Components/Media",
    "ALIGNED",
    "UNTESTED",
    "Lightbox loading/error/image controls, zoom, keyboard dismissal, accessible naming, and viewport containment remain production-owned and token-consistent.",
    "Codex media states lack a same-state evidence set; parity remains UNTESTED.",
    "Lightbox six-state Story family and interaction/a11y review.",
  ),
  reviewedFamily(
    "components-content",
    "Components/Content",
    "ALIGNED",
    "UNTESTED",
    "Markdown and Mermaid headings, code, links, media, long content, errors, roles, and overflow retain the production content hierarchy under shared tokens.",
    "Codex content examples are not a complete same-state component matrix; parity remains UNTESTED.",
    "Markdown and Mermaid Story families plus visual review.",
  ),
  reviewedFamily(
    "components-runs",
    "Components/Runs",
    "FIXED",
    "UNTESTED",
    "Run header and log lifecycle states now use shared status semantics while loading, empty, long output, keyboard actions, and error containment remain unchanged.",
    "No complete Codex run-log state matrix exists; parity remains UNTESTED.",
    "RunHeader, RunLogItem, and RunView Story families plus LifecycleStatus review.",
  ),
  reviewedFamily(
    "components-scheduled",
    "Components/Scheduled",
    "FIXED",
    "UNTESTED",
    "Scheduled rows use the shared lifecycle glyph, correct settled/crash precedence, 44px action geometry, labelled status/cadence, and quiet current-row selection. Their cursor action menu now exposes a title-derived accessible name, 44px mobile/coarse rows, enabled-only roving focus, internal long-menu scroll, Tab/Shift+Tab handoff, and durable Escape, ordinary-selection, and dialog-close focus return.",
    "CODEX-PARITY GL-11 passes the shell, but detailed scheduled states remain mixed; family parity remains UNTESTED.",
    "QA-92 screenshot 06 and Scheduled Parts Stories plus fresh 05-real-scheduled-context-390x500.png/live production-callsite review: the menu exposes the full schedule-title aria-label, gives Pause the unique enabled initial focus at 44px, remains within 8px viewport gutters with zero document overflow, and returns Escape and Pin selection to the exact More trigger. Independent code and design reviewers APPROVE with scoped P0/P1/P2=0.",
  ),
  reviewedFamily(
    "components-sessions",
    "Components/Sessions",
    "FIXED",
    "UNTESTED",
    "Session chrome and thread states share lifecycle semantics and review surfaces; approval dark-phone, loading, errors, goals, queue, keyboard, composer, and overflow were reviewed without changing session behavior.",
    "CODEX-PARITY thread rows contain mixed PASS/UNTESTED evidence; family parity remains UNTESTED.",
    "QA-92 screenshots 18-19 and SessionChrome/SessionView Story families.",
  ),
  reviewedFamily(
    "components-settings",
    "Components/Settings",
    "ALIGNED",
    "UNTESTED",
    "Appearance, archived, configuration, general, Git, shortcuts, worktrees, toggles, disabled states, keyboard, and mobile overflow keep one calm settings hierarchy.",
    "No complete same-state Codex settings matrix exists; parity remains UNTESTED.",
    "QA-92 current-source Settings evidence and all component Settings Story families.",
  ),
  reviewedFamily(
    "components-supervision",
    "Components/Supervision",
    "FIXED",
    "UNTESTED",
    "Agents, attention, artifacts, goal, progress, background, resting/loading, and panel states use shared lifecycle semantics and preserve labelled live updates and overflow.",
    "Codex supervision internals lack a same-state evidence matrix; parity remains UNTESTED.",
    "All Supervision Story families and LifecycleStatus composition review.",
  ),
  reviewedFamily(
    "components-timeline",
    "Components/Timeline",
    "FIXED",
    "UNTESTED",
    "Timeline chrome, tools, activity, user/assistant content, lifecycle, hover/focus actions, pending/typing, long content, jump, and scroll states retain hierarchy; hidden speaker text avoids visual noise.",
    "CODEX-PARITY thread evidence covers selected compositions, not every timeline state; family parity remains UNTESTED.",
    "TimelineView/Timeline Chrome Story families plus visual and interaction review.",
  ),
  reviewedFamily(
    "pages-appshell",
    "Pages/AppShell",
    "FIXED",
    "UNTESTED",
    "The composed shell uses chromeless Home current treatment, glyph-only running status, shared tokens, and deliberate keyboard navigation. Desktop primary navigation follows the 32px/13px rail rhythm while mobile/coarse navigation remains 44px tall. On mobile, both a pointer ContextMenu and the touch More Popover/Menu own Escape and yield Tab above the sidebar focus scope: Escape closes only the temporary menu and returns to the exact invoker, Tab closes and advances relative to that invoker, and Rename dialog close returns to the same durable invoker without collapsing the sidebar.",
    "CODEX-PARITY GL-01/NS-01 cover clean shell states, but the keyboard Story has no same-state Codex capture; family parity remains UNTESTED.",
    "QA-92 screenshot 05, R82 desktop rail probe, and AppShell Stories plus fresh Pages/AppShell live review at 390×500 and App.mobile-focus integration coverage: desktop nav/session rows share 32px/13px rhythm; mobile New session and Scheduled rows measure 44px/13px. Both pointer ContextMenu and touch More Popover/Menu pass the same contract: Escape from the active item leaves the temporary menu absent, sidebar expanded, Show sidebar absent, and exact invoker focused; Tab leaves the menu absent and advances to the next page-order target (neighboring session or footer More options, depending on the invoking row); Rename input Escape leaves the dialog absent and restores the exact invoker. The sidebar remains expanded, document overflow stays zero, and console warnings/errors stay zero in all three paths.",
  ),
  reviewedFamily(
    "pages-appruntime",
    "Pages/AppRuntime",
    "ALIGNED",
    "UNTESTED",
    "Runtime composition and keyboard states inherit the production shell without Story-only visual ownership or ambient service access.",
    "Runtime wiring is not a visible Codex parity surface; visual parity remains UNTESTED.",
    "AppRuntime Stories and AppServices boundary lint.",
  ),
  reviewedFamily(
    "pages-home",
    "Pages/Home",
    "FIXED",
    "UNTESTED",
    "Home hierarchy, project-aware and long headline, draft, starters, keyboard, responsive overflow, and chromeless New session current semantics align with the production landing page.",
    "CODEX-PARITY NS-01/NS-04/NS-12 pass selected Home states; remaining Story variants keep the family verdict UNTESTED.",
    "QA-88 NS-01 evidence, QA-92 current-source Home, and Home Stories.",
  ),
  reviewedFamily(
    "pages-scheduled",
    "Pages/Scheduled",
    "FIXED",
    "UNTESTED",
    "Scheduled list/detail/edit/loading/empty/search/filter/pagination/conflict/busy/error states share lifecycle, selection, touch, label, and responsive contracts. The real list-row action path also shares the named, enabled-only roving, viewport-contained, Tab-handoff, and durable focus-return contract with other cursor menus.",
    "CODEX-PARITY GL-11 passes the shell; detailed states and current-source mobile remain UNTESTED.",
    "QA-92 screenshots 06/09/10 and Scheduled Stories plus fresh 05-real-scheduled-context-390x500.png/live list-row review: the real More trigger opens a full-title-labelled menu, gives Pause the unique 44px initial focus, stays within 8px viewport gutters with zero document overflow, and regains focus after Escape or Pin selection. ContextMenu interaction tests cover Tab handoff and destination-dialog ownership; independent code and design reviewers APPROVE with scoped P0/P1/P2=0.",
  ),
  reviewedFamily(
    "pages-settings",
    "Pages/Settings",
    "ALIGNED",
    "UNTESTED",
    "Settings full-page composition, section navigation, initial appearance, keyboard, light/dark, and responsive containment preserve the production information architecture.",
    "No complete same-state Codex Settings family evidence exists; parity remains UNTESTED.",
    "QA-92 screenshots 12-13 and Settings Stories.",
  ),
  reviewedFamily(
    "pages-pagehost",
    "Pages/PageHost",
    "ALIGNED",
    "UNTESTED",
    "Home, Scheduled, Run, and missing-session route projections use the production PageHost and remain visually governed by their reviewed page families.",
    "Route precedence is an AgentRunner product contract; same-state Codex evidence is incomplete.",
    "PageHost route Stories and current-source deep-link/reload QA-92 checks.",
  ),
  reviewedFamily(
    "cujs-core-session",
    "CUJs/Core Session Journeys",
    "INTENTIONAL",
    "INTENTIONAL",
    "Five deterministic production journeys are audit/playback harnesses, not a second product surface; their visible steps inherit the reviewed Home, Session, Environment, and Changes families.",
    "Codex has no equivalent public Storybook CUJ harness; direct harness parity is intentionally out of scope.",
    "Core Session Journeys five Story IDs and Storybook interaction gate.",
  ),
  reviewedFamily(
    "demos-core-session",
    "Demos/Core Session Playback",
    "INTENTIONAL",
    "INTENTIONAL",
    "The human-paced playback is a deterministic reviewer tool over production components; transport controls are intentionally workbench-only.",
    "Codex has no equivalent public Storybook playback surface; direct harness parity is intentionally out of scope.",
    "Core Session Playback Story and 19-step production-component journey.",
  ),
  reviewedFamily(
    "demos-scenario-controls",
    "Demos/Scenario Controls",
    "INTENTIONAL",
    "INTENTIONAL",
    "Playback controls expose the deterministic scenario state machine for reviewers and do not define production UI.",
    "Codex has no equivalent public Storybook scenario controller; direct harness parity is intentionally out of scope.",
    "Scenario Controls default and all-playback-states Stories.",
  ),
] satisfies readonly StoryReviewFamily[];

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
    componentId: "LifecycleStatus",
    source: "src/ui/LifecycleStatus.tsx",
    storySource: "src/ui/LifecycleStatus.stories.tsx",
    exportName: "LifecycleStatus",
    cells: {
      "render:default": {
        status: "covered",
        storyId: "foundations-feedback-lifecycle-status--default",
      },
      "a11y:keyboard": {
        status: "n-a",
        reason: "LifecycleStatus is a non-interactive status glyph, not an input control.",
        evidence:
          "The default Story verifies separate visible copy, accessible name, and busy semantics.",
        owner: "webui",
      },
      "state:lifecycle-matrix": {
        status: "covered",
        storyId: "foundations-feedback-lifecycle-status--lifecycle-matrix",
      },
      "state:icon-only": {
        status: "covered",
        storyId: "foundations-feedback-lifecycle-status--icon-only",
      },
      "boundary:long-label": {
        status: "covered",
        storyId: "foundations-feedback-lifecycle-status--long-visible-copy",
      },
      "a11y:reduced-motion": {
        status: "covered",
        storyId: "foundations-feedback-lifecycle-status--reduced-motion",
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
