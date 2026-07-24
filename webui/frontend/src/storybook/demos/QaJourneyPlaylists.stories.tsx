import {
  useCallback,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type SyntheticEvent,
} from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { Button } from "../../ui/Button";
import type { StoryPlaybackPace } from "../humanPlayback";
import { ScenarioControls } from "../scenarios/ScenarioControls";
import {
  createDemoScenarioTiming,
  ScenarioRunner,
  type DemoStep,
} from "../scenarios/ScenarioRunner";

type QaJourneyId =
  | "session-delivery"
  | "attention-permissions"
  | "supervision"
  | "scheduled-work"
  | "changes-artifacts"
  | "navigation-recovery";

interface QaDemoScene {
  id: string;
  title: string;
  storyId: string;
  qaRefs: readonly string[];
  inspect: string;
  dwellMs: number;
}

interface QaJourneyPlaylist {
  id: QaJourneyId;
  title: string;
  summary: string;
  scenes: readonly QaDemoScene[];
}

export const qaJourneyPlaylists: Readonly<
  Record<QaJourneyId, QaJourneyPlaylist>
> = Object.freeze({
  "session-delivery": {
    id: "session-delivery",
    title: "Session & Delivery",
    summary:
      "From choosing context through queued, steered, stopped, and attachment-backed follow-ups.",
    scenes: [
      {
        id: "starter-intent",
        title: "Choose a starter intent",
        storyId: "pages-home--starter-intent-flow",
        qaRefs: ["QA-42", "QA-69"],
        inspect: "The choice should seed the composer without creating a session.",
        dwellMs: 7000,
      },
      {
        id: "project",
        title: "Choose project and run location",
        storyId: "components-input-composer--project-picker",
        qaRefs: ["QA-42", "QA-46"],
        inspect: "Project and worktree context stay visible after the picker closes.",
        dwellMs: 7000,
      },
      {
        id: "access",
        title: "Choose approval posture",
        storyId: "components-input-composer--access-and-approval",
        qaRefs: ["QA-44", "QA-51"],
        inspect: "The selected posture is clear without exposing runtime enums.",
        dwellMs: 7000,
      },
      {
        id: "model",
        title: "Choose model and effort",
        storyId: "components-input-composer--model-and-effort",
        qaRefs: ["QA-10", "QA-89"],
        inspect: "Model and effort remain two explicit, keyboard-reachable choices.",
        dwellMs: 7000,
      },
      {
        id: "queue",
        title: "Queue a follow-up",
        storyId: "components-input-composer--running-queued",
        qaRefs: ["QA-02", "QA-45"],
        inspect: "Queued delivery is visible before send and remains distinct from steer.",
        dwellMs: 8000,
      },
      {
        id: "steer",
        title: "Steer the active turn",
        storyId: "components-input-composer--running-steer",
        qaRefs: ["QA-05", "QA-45"],
        inspect: "Steer is a deliberate delivery choice, not a second send button.",
        dwellMs: 8000,
      },
      {
        id: "stop",
        title: "Stop the active turn",
        storyId: "components-input-composer--stop-active-turn",
        qaRefs: ["QA-06"],
        inspect: "The destructive action is isolated and the draft remains intact.",
        dwellMs: 7000,
      },
      {
        id: "attachments",
        title: "Continue with attachments",
        storyId: "components-input-composer--fork-draft-with-attachments",
        qaRefs: ["QA-07", "QA-82", "QA-90"],
        inspect: "Text and attachments stay attached to one durable follow-up.",
        dwellMs: 8000,
      },
    ],
  },
  "attention-permissions": {
    id: "attention-permissions",
    title: "Attention & Permissions",
    summary:
      "Approval details, allow/deny decisions, structured questions, and busy feedback.",
    scenes: [
      {
        id: "approval-details",
        title: "Inspect an approval request",
        storyId: "components-attention-approvalcard--details-open",
        qaRefs: ["QA-25", "QA-26"],
        inspect: "The human-readable request leads; raw command detail stays secondary.",
        dwellMs: 7000,
      },
      {
        id: "approve",
        title: "Approve from the keyboard",
        storyId: "components-attention-approvalcard--keyboard-approval",
        qaRefs: ["QA-26", "QA-62"],
        inspect: "Focus order and decision feedback make the completed action obvious.",
        dwellMs: 7000,
      },
      {
        id: "deny",
        title: "Explain a denial",
        storyId: "components-attention-approvalcard--deny-reason-open",
        qaRefs: ["QA-25", "QA-28"],
        inspect: "The reason field appears only after Deny and preserves the safe default.",
        dwellMs: 7000,
      },
      {
        id: "structured-question",
        title: "Answer structured questions",
        storyId: "components-attention-askform--multiple-answers",
        qaRefs: ["QA-13"],
        inspect: "Each question keeps its label, choices, and independent answer state.",
        dwellMs: 8000,
      },
      {
        id: "free-text",
        title: "Answer with free text",
        storyId: "components-attention-askform--free-text-only",
        qaRefs: ["QA-13"],
        inspect: "Free text is reachable without pretending the prompt has fixed choices.",
        dwellMs: 7000,
      },
      {
        id: "busy",
        title: "Keep the decision visible while submitting",
        storyId: "components-attention-askform--busy-submitting",
        qaRefs: ["QA-13", "QA-62"],
        inspect: "Busy feedback prevents duplicate action without erasing the answer.",
        dwellMs: 7000,
      },
    ],
  },
  supervision: {
    id: "supervision",
    title: "Goals, Agents & Supervision",
    summary:
      "Goal setup and lifecycle alongside child agents, attention, and generated artifacts.",
    scenes: [
      {
        id: "goal-launch",
        title: "Configure a goal",
        storyId: "components-input-composer--goal-launcher",
        qaRefs: ["QA-16", "QA-17", "QA-48"],
        inspect: "Goal controls reveal only the choices needed before launch.",
        dwellMs: 8000,
      },
      {
        id: "goal-active",
        title: "Inspect an active goal",
        storyId: "components-supervision-goal-section--active",
        qaRefs: ["QA-16", "QA-48"],
        inspect: "Objective, verifier, progress, and lifecycle actions remain distinguishable.",
        dwellMs: 8000,
      },
      {
        id: "goal-paused",
        title: "Pause and resume a goal",
        storyId: "components-supervision-goal-section--paused-self-certified",
        qaRefs: ["QA-17", "QA-66"],
        inspect: "Paused is a durable state with a clear resume path.",
        dwellMs: 7000,
      },
      {
        id: "subagents",
        title: "Navigate child agents",
        storyId: "components-supervision-subagents--keyboard-navigation",
        qaRefs: ["QA-04", "QA-05", "QA-68"],
        inspect: "Parent/child hierarchy, running state, and destination remain legible.",
        dwellMs: 8000,
      },
      {
        id: "attention",
        title: "Review supervision attention",
        storyId: "components-supervision-attention--all-notice-types",
        qaRefs: ["QA-05", "QA-08", "QA-66"],
        inspect: "Failures, approvals, and recovery notices do not collapse into one badge.",
        dwellMs: 8000,
      },
      {
        id: "artifacts",
        title: "Open generated artifacts",
        storyId: "components-supervision-artifacts--file-types-and-overflow",
        qaRefs: ["QA-03", "QA-07", "QA-38"],
        inspect: "File type, path, overflow, and open action are all visible.",
        dwellMs: 7000,
      },
    ],
  },
  "scheduled-work": {
    id: "scheduled-work",
    title: "Scheduled Work",
    summary:
      "List, detail, pause/resume, editing, conflicts, and recovery for recurring work.",
    scenes: [
      {
        id: "scheduled-list",
        title: "Scan scheduled work",
        storyId: "pages-scheduled--default",
        qaRefs: ["QA-58", "QA-74", "QA-88"],
        inspect: "Lifecycle, cadence, last run, and the next action scan as separate facts.",
        dwellMs: 8000,
      },
      {
        id: "detail",
        title: "Open schedule details",
        storyId: "pages-scheduled--schedule-detail",
        qaRefs: ["QA-74", "QA-77", "QA-88"],
        inspect: "Configuration and durable run history share one detail surface.",
        dwellMs: 8000,
      },
      {
        id: "paused",
        title: "Inspect a paused schedule",
        storyId: "pages-scheduled--paused-schedule-detail",
        qaRefs: ["QA-58", "QA-77"],
        inspect: "Paused, next-run absence, and Resume agree with each other.",
        dwellMs: 7000,
      },
      {
        id: "edit",
        title: "Edit cadence and overlap",
        storyId: "pages-scheduled--edit-schedule",
        qaRefs: ["QA-74", "QA-77"],
        inspect: "The editor exposes cadence and overlap without losing the original state.",
        dwellMs: 8000,
      },
      {
        id: "conflict",
        title: "Recover from an invalid cadence",
        storyId: "pages-scheduled--schedule-edit-cron-conflict",
        qaRefs: ["QA-58", "QA-77"],
        inspect: "The error points to the conflicting field and keeps correction in place.",
        dwellMs: 8000,
      },
      {
        id: "error",
        title: "Retry a failed detail load",
        storyId: "pages-scheduled--detail-error",
        qaRefs: ["QA-70", "QA-77"],
        inspect: "Failure does not erase list context and Retry remains obvious.",
        dwellMs: 7000,
      },
    ],
  },
  "changes-artifacts": {
    id: "changes-artifacts",
    title: "Changes & Artifacts",
    summary:
      "Review workspace changes, change scope, commit safely, inspect conflicts, and open images.",
    scenes: [
      {
        id: "outcome",
        title: "Review the last-turn outcome",
        storyId: "components-changes-changesoutcome--default",
        qaRefs: ["QA-41", "QA-60"],
        inspect: "The summary, changed files, and next action form one clear handoff.",
        dwellMs: 8000,
      },
      {
        id: "diff",
        title: "Inspect a real diff",
        storyId: "components-changes-diffview--default",
        qaRefs: ["QA-60", "QA-61", "QA-69"],
        inspect: "File navigation, line context, wrapping, and split/inline controls stay usable.",
        dwellMs: 9000,
      },
      {
        id: "scope",
        title: "Change the diff scope",
        storyId: "components-changes-diffparts--scope-picker-keyboard",
        qaRefs: ["QA-60", "QA-76"],
        inspect: "Working tree and last-turn scope remain explicit and keyboard reachable.",
        dwellMs: 7000,
      },
      {
        id: "commit",
        title: "Commit reviewed changes",
        storyId: "components-changes-diffparts--commit-ready",
        qaRefs: ["QA-60", "QA-69"],
        inspect: "Commit intent, branch context, and primary action agree before execution.",
        dwellMs: 8000,
      },
      {
        id: "conflict",
        title: "Explain a commit conflict",
        storyId: "components-changes-diffparts--commit-conflict",
        qaRefs: ["QA-60", "QA-76"],
        inspect: "The error preserves the reviewed diff and offers a concrete recovery path.",
        dwellMs: 8000,
      },
      {
        id: "image",
        title: "Open an image artifact",
        storyId: "components-changes-changesoutcome--image-lightbox-open",
        qaRefs: ["QA-07", "QA-38", "QA-69"],
        inspect: "Preview, accessible name, close action, and unavailable fallback stay coherent.",
        dwellMs: 8000,
      },
      {
        id: "retry",
        title: "Retry a failed diff request",
        storyId: "components-changes-diffparts--error-retry",
        qaRefs: ["QA-08", "QA-61"],
        inspect: "Retry restores the review surface without losing the selected context.",
        dwellMs: 7000,
      },
    ],
  },
  "navigation-recovery": {
    id: "navigation-recovery",
    title: "Navigation & Recovery",
    summary:
      "Session navigation, project recovery, search, daemon recovery, missing routes, and settings.",
    scenes: [
      {
        id: "session-nav",
        title: "Navigate sessions from the sidebar",
        storyId: "components-navigation-sidebar--session-navigation",
        qaRefs: ["QA-27", "QA-78", "QA-83"],
        inspect: "Current, running, unread, and project hierarchy stay visible while navigating.",
        dwellMs: 8000,
      },
      {
        id: "project-recovery",
        title: "Recover a removed project",
        storyId: "components-navigation-sidebar--removed-project-recovery",
        qaRefs: ["QA-56", "QA-78", "QA-84"],
        inspect: "Removal is reversible and does not silently discard sessions.",
        dwellMs: 8000,
      },
      {
        id: "search",
        title: "Find a session by keyboard",
        storyId: "components-navigation-commandpalette--keyboard-navigation",
        qaRefs: ["QA-27", "QA-53", "QA-86"],
        inspect: "Search, selection, archived state, and focus return form one path.",
        dwellMs: 8000,
      },
      {
        id: "daemon",
        title: "Recover daemon connectivity",
        storyId: "components-attention-daemonalert--keyboard-retry",
        qaRefs: ["QA-08", "QA-21", "QA-70"],
        inspect: "Offline is exceptional, Retry is explicit, and healthy state becomes quiet.",
        dwellMs: 7000,
      },
      {
        id: "missing-session",
        title: "Return from a missing session",
        storyId: "components-feedback-sessionnotfound--keyboard-back",
        qaRefs: ["QA-08", "QA-89"],
        inspect: "The route failure names the problem and provides one safe way back.",
        dwellMs: 7000,
      },
      {
        id: "settings",
        title: "Navigate product settings",
        storyId: "pages-settings--keyboard-navigation",
        qaRefs: ["QA-34", "QA-86", "QA-89"],
        inspect: "Section hierarchy, current location, search, and close behavior stay predictable.",
        dwellMs: 8000,
      },
    ],
  },
});

interface PlaylistContext {
  show(scene: QaDemoScene): void;
}

interface DisplayedScene {
  scene: QaDemoScene;
  revision: number;
}

type DemoFrameState = "loading" | "ready" | "failed";
const QA_FRAME_MESSAGE = "agentrunner-story-frame";
const STORY_INTERACTION_FRAME_DOCUMENT =
  '<div id="storybook-root"><main data-storybook-test-frame="ready">Canonical Story frame readiness fixture</main></div>';
const IS_STORY_INTERACTION_TEST = import.meta.env.MODE === "test";

export interface QaJourneyPlaybackProps {
  autoPlay: boolean;
  journey: QaJourneyId;
  playbackPace: StoryPlaybackPace;
}

function storyFrameSource(storyId: string, revision: number): string {
  const globals = encodeURIComponent(
    "theme:light;playbackPace:automated",
  );
  return `/iframe.html?id=${storyId}&viewMode=story&globals=${globals}&qaDemo=${revision}`;
}

export function inspectStoryFrameDocument(
  document: Document | null,
): "pending" | "ready" | "failed" {
  if (!document) return "pending";
  const bodyText = document.body?.innerText ?? "";
  if (
    document.querySelector(
      '[data-storybook="story-error"], .sb-errordisplay, #error-message, #error-stack',
    ) ||
    /couldn['’]t find story|error rendering component|story rendering failed/i.test(
      bodyText,
    )
  ) {
    return "failed";
  }
  const root = document.getElementById("storybook-root");
  return root && root.childNodes.length > 0 ? "ready" : "pending";
}

export function QaJourneyPlayback({
  autoPlay: initialAutoPlay,
  journey,
  playbackPace,
}: QaJourneyPlaybackProps) {
  const playlist = qaJourneyPlaylists[journey];
  const transitionScenes = playlist.scenes.slice(1);
  const revision = useRef(0);
  const [displayed, setDisplayed] = useState<DisplayedScene>({
    scene: playlist.scenes[0],
    revision: revision.current,
  });
  const frameCheck = useRef(0);
  const frameResolved = useRef(false);
  const [frameState, setFrameState] =
    useState<DemoFrameState>("loading");
  const show = useCallback((scene: QaDemoScene) => {
    revision.current += 1;
    setDisplayed({ scene, revision: revision.current });
  }, []);
  const [runner] = useState(
    () =>
      new ScenarioRunner<PlaylistContext>({
        context: { show },
        steps: transitionScenes.map(
          (scene): DemoStep<PlaylistContext> => ({
            id: scene.id,
            title: scene.title,
            run(context) {
              context.show(scene);
            },
          }),
        ),
        timing: createDemoScenarioTiming(
          playbackPace === "instant"
            ? 0
            : playbackPace === "automated"
              ? 400
              : ({ stepIndex }) =>
                  playlist.scenes[stepIndex]?.dwellMs ?? 7000,
        ),
        recreateContext: () => {
          show(playlist.scenes[0]);
          return { show };
        },
      }),
  );
  const snapshot = useSyncExternalStore(
    runner.subscribe.bind(runner),
    runner.getSnapshot,
    runner.getSnapshot,
  );
  const [autoPlay, setAutoPlay] = useState(initialAutoPlay);
  const displayedIndex = playlist.scenes.findIndex(
    (scene) => scene.id === displayed.scene.id,
  );

  useEffect(() => {
    frameCheck.current += 1;
    frameResolved.current = false;
    setFrameState("loading");
  }, [displayed.revision]);

  useEffect(() => {
    const handleMessage = (event: MessageEvent<unknown>) => {
      if (event.origin !== globalThis.window.location.origin) return;
      if (!event.data || typeof event.data !== "object") return;
      const data = event.data as Record<string, unknown>;
      if (
        data.type !== QA_FRAME_MESSAGE ||
        data.storyId !== displayed.scene.storyId ||
        data.frameRevision !== String(displayed.revision) ||
        (data.status !== "ready" && data.status !== "failed")
      ) {
        return;
      }
      frameResolved.current = true;
      frameCheck.current += 1;
      setFrameState(data.status);
    };
    globalThis.window.addEventListener("message", handleMessage);
    return () => globalThis.window.removeEventListener("message", handleMessage);
  }, [displayed.revision, displayed.scene.storyId]);

  useEffect(
    () => () => {
      frameCheck.current += 1;
    },
    [],
  );

  useEffect(() => {
    if (!autoPlay) return;
    const current = runner.getSnapshot();
    if (current.status !== "idle" && current.status !== "paused") return;
    void runner.play("autoplay").catch(() => {
      // ScenarioRunner keeps the failure visible in the shared transport.
    });
  }, [autoPlay, runner]);

  useEffect(
    () => () => {
      void runner.dispose();
    },
    [runner],
  );

  const handleAutoPlayChange = useCallback(
    (enabled: boolean) => {
      setAutoPlay(enabled);
      if (!enabled) return;
      const current = runner.getSnapshot();
      if (current.status !== "idle" && current.status !== "paused") return;
      void runner.play("autoplay").catch(() => {
        // ScenarioRunner keeps the failure visible in the shared transport.
      });
    },
    [runner],
  );

  const handleFrameLoad = useCallback(
    (event: SyntheticEvent<HTMLIFrameElement>) => {
      const frame = event.currentTarget;
      const checkId = ++frameCheck.current;
      let initial: ReturnType<typeof inspectStoryFrameDocument> = "pending";
      try {
        initial = inspectStoryFrameDocument(frame.contentDocument);
      } catch {
        // Some browser-isolation modes intentionally hide contentDocument.
      }
      if (
        initial === "failed" ||
        (IS_STORY_INTERACTION_TEST && initial === "ready")
      ) {
        frameResolved.current = true;
        frameCheck.current += 1;
        setFrameState(initial);
        return;
      }
      globalThis.setTimeout(() => {
        if (
          frameCheck.current !== checkId ||
          frameResolved.current
        ) {
          return;
        }
        let fallback: ReturnType<typeof inspectStoryFrameDocument> = "pending";
        try {
          fallback = inspectStoryFrameDocument(frame.contentDocument);
        } catch {
          // Some browser-isolation modes intentionally hide contentDocument.
          // The preview-side postMessage handshake remains authoritative.
        }
        frameResolved.current = true;
        frameCheck.current += 1;
        setFrameState(fallback === "ready" ? "ready" : "failed");
      }, 15_000);
    },
    [],
  );

  const retryFrame = useCallback(() => {
    show(displayed.scene);
  }, [displayed.scene, show]);

  return (
    <div className="grid h-[100dvh] min-h-0 grid-rows-[auto_auto_minmax(0,1fr)] overflow-hidden bg-bg text-ink">
      <ScenarioControls
        runner={runner}
        label={`${playlist.title} QA demo`}
        autoPlay={autoPlay || snapshot.owner === "autoplay"}
        onAutoPlayChange={handleAutoPlayChange}
        displayProgress={{
          index: displayedIndex,
          count: playlist.scenes.length,
          title: displayed.scene.title,
        }}
      />
      <section
        className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-line bg-panel px-4 py-2"
        aria-label="Demo checkpoint"
      >
        <strong className="text-[13px]">{displayed.scene.title}</strong>
        <span className="text-[11px] text-dim">
          {displayed.scene.qaRefs.join(" · ")}
        </span>
        <span className="text-[11px] tabular-nums text-dim">
          Checkpoint {displayedIndex + 1} / {playlist.scenes.length}
        </span>
        <span className="min-w-[280px] flex-1 text-[12px] text-ink-2">
          {displayed.scene.inspect}
        </span>
        <output
          className="text-[11px] text-dim"
          aria-label="Demo frame status"
          aria-live="polite"
        >
          {frameState === "ready"
            ? "Ready"
            : frameState === "failed"
              ? "Failed to load"
              : "Loading…"}
        </output>
        {frameState === "failed" && (
          <Button size="sm" variant="ghost" onClick={retryFrame}>
            Retry frame
          </Button>
        )}
      </section>
      <iframe
        key={displayed.revision}
        className="h-full min-h-0 w-full border-0 bg-bg"
        src={
          IS_STORY_INTERACTION_TEST
            ? undefined
            : storyFrameSource(
                displayed.scene.storyId,
                displayed.revision,
              )
        }
        srcDoc={
          IS_STORY_INTERACTION_TEST
            ? STORY_INTERACTION_FRAME_DOCUMENT
            : undefined
        }
        title={`${playlist.title}: ${displayed.scene.title}`}
        onLoad={handleFrameLoad}
        onError={() => setFrameState("failed")}
      />
    </div>
  );
}

const meta = {
  title: "Demos/QA Journey Playlists",
  component: QaJourneyPlayback,
  excludeStories: [
    "QaJourneyPlayback",
    "inspectStoryFrameDocument",
    "qaJourneyPlaylists",
  ],
  parameters: {
    layout: "fullscreen",
  },
  args: {
    autoPlay: false,
    journey: "session-delivery",
    playbackPace: "human",
  },
  argTypes: {
    journey: {
      table: { disable: true },
    },
    playbackPace: {
      table: { disable: true },
    },
  },
} satisfies Meta<typeof QaJourneyPlayback>;

export default meta;
type Story = StoryObj<typeof meta>;

const verifyPlaylist = async (
  canvasElement: HTMLElement,
  label: string,
  checkpoint: string,
) => {
  const canvas = within(canvasElement);
  await expect(
    canvas.getByRole("region", { name: `${label} QA demo` }),
  ).toBeVisible();
  const checkpointRegion = canvas.getByRole("region", {
    name: "Demo checkpoint",
  });
  await expect(
    within(checkpointRegion).getByText(checkpoint, {
      selector: "strong",
    }),
  ).toBeVisible();
  await expect(
    canvas.getByRole("button", { name: "Play" }),
  ).toBeEnabled();
  const frameStatus = canvas.getByRole("status", {
    name: "Demo frame status",
  });
  await waitFor(
    () => expect(frameStatus).toHaveTextContent("Ready"),
    { timeout: 10_000 },
  );
};

export const SessionAndDelivery: Story = {
  args: { journey: "session-delivery" },
  play: ({ canvasElement }) =>
    verifyPlaylist(canvasElement, "Session & Delivery", "Choose a starter intent"),
};

export const AttentionAndPermissions: Story = {
  args: { journey: "attention-permissions" },
  play: ({ canvasElement }) =>
    verifyPlaylist(
      canvasElement,
      "Attention & Permissions",
      "Inspect an approval request",
    ),
};

export const GoalsAgentsAndSupervision: Story = {
  args: { journey: "supervision" },
  play: ({ canvasElement }) =>
    verifyPlaylist(
      canvasElement,
      "Goals, Agents & Supervision",
      "Configure a goal",
    ),
};

export const ScheduledWork: Story = {
  args: { journey: "scheduled-work" },
  play: ({ canvasElement }) =>
    verifyPlaylist(canvasElement, "Scheduled Work", "Scan scheduled work"),
};

export const ChangesAndArtifacts: Story = {
  args: { journey: "changes-artifacts" },
  play: ({ canvasElement }) =>
    verifyPlaylist(
      canvasElement,
      "Changes & Artifacts",
      "Review the last-turn outcome",
    ),
};

export const NavigationAndRecovery: Story = {
  args: { journey: "navigation-recovery" },
  play: ({ canvasElement }) =>
    verifyPlaylist(
      canvasElement,
      "Navigation & Recovery",
      "Navigate sessions from the sidebar",
    ),
};
