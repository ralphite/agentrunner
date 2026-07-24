import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { AppShell } from "../../App";
import { AppRuntime } from "../../app/AppRuntime";
import { createAppStore, type AppStore } from "../../store";
import {
  buildAssistantMessage,
  buildEnvelope,
  buildHealth,
  buildInspect,
  buildProjects,
  buildSession,
  buildTimeline,
  fixtureDefaults,
} from "../fixtures";
import { createStoryApiHandlers, type StoryApiHarness } from "../handlers";
import { createStoryAppServices } from "../appServices";
import { ScenarioClock } from "../scenarios/ScenarioClock";
import { ScenarioControls } from "../scenarios/ScenarioControls";
import {
  createDemoScenarioTiming,
  ScenarioRunner,
  type DemoStep,
} from "../scenarios/ScenarioRunner";
import {
  createScriptedStreamController,
  type ScriptedEventStream,
  type ScriptedStreamController,
} from "../streams";
import "./CoreSessionPlayback.css";

const HISTORY_SID = "20260722-190000-storybook-history";
const SESSION_SID = "story-session-1";
const SESSION_STREAM = `/api/sessions/${SESSION_SID}/stream`;
const PROMPT =
  "Build a deterministic Storybook demo for the core Agent Runner session journey.";
const IS_COMPONENT_TEST = String(import.meta.env.VITEST) === "true";
const DEFAULT_HUMAN_STEP_DELAY_MS = 1600;
const AUTOMATED_STEP_DELAY_MS = 400;
const HUMAN_TYPE_CHUNK_DELAY_MS = 48;
const TYPE_CHUNK_SIZE = 1;
const HUMAN_STEP_DELAY_MS: Readonly<Record<string, number>> = Object.freeze({
  "open-project": 1400,
  "select-project": 1400,
  "choose-build": 1800,
  "choose-build-ui": 1800,
  "type-request": 1200,
  "open-access": 1600,
  "select-access": 1600,
  "open-model": 1600,
  "open-model-list": 1800,
  "select-model": 1600,
  send: 2400,
  "stream-first-chunk": 2800,
  "stream-second-chunk": 2800,
  "persist-response": 3800,
  environment: 2400,
  "publish-completion": 3400,
  "complete-session": 3400,
  review: 3000,
  "return-to-session": 2800,
});

const historySession = buildSession({
  id: HISTORY_SID,
  title: "Prepare the Storybook component system",
  status: "completed",
  turns: 3,
  workspace: fixtureDefaults.workspace,
});

// One handler set owns this Story's isolated in-memory backend. Reset restores
// the pristine seed without replacing handlers already registered by MSW.
const demoApi = createStoryApiHandlers({
  health: buildHealth(),
  sessions: [historySession],
  runs: [],
  projects: buildProjects(),
  events: { [HISTORY_SID]: buildTimeline() },
  inspect: { [HISTORY_SID]: buildInspect() },
  backgroundWork: { [HISTORY_SID]: [] },
  queue: { [HISTORY_SID]: [] },
  blobs: {
    "src/Card.tsx": ['export const label = "After";'],
    "src/Card.stories.tsx": [
      'import { Card } from "./Card";',
      "",
      "export const Default = () => <Card />;",
    ],
  },
});

interface DemoContext {
  api: StoryApiHarness;
  clock: ScenarioClock;
  controller: ScriptedStreamController;
  typeChunkDelayMs: number;
  store: AppStore;
  services: ReturnType<typeof createStoryAppServices>["services"];
  root: HTMLElement | null;
  waitForStream(
    path: string,
    signal: AbortSignal,
  ): Promise<ScriptedEventStream>;
}

type ElementQuery<T extends Element> = (root: HTMLElement) => T | null;

function waitForElement<T extends Element>(
  context: DemoContext,
  query: ElementQuery<T>,
  signal: AbortSignal,
): Promise<T> {
  const root = context.root;
  if (!root) throw new Error("Demo canvas is not mounted");
  const current = query(root);
  if (current) return Promise.resolve(current);

  return new Promise<T>((resolve, reject) => {
    const observer = new MutationObserver(() => {
      const match = query(root);
      if (!match) return;
      cleanup();
      resolve(match);
    });
    const guard = globalThis.setTimeout(() => {
      cleanup();
      reject(new Error("Timed out waiting for the production UI state"));
    }, 5000);
    const onAbort = () => {
      cleanup();
      reject(new DOMException("Scenario reset", "AbortError"));
    };
    const cleanup = () => {
      observer.disconnect();
      globalThis.clearTimeout(guard);
      signal.removeEventListener("abort", onAbort);
    };
    observer.observe(root, {
      childList: true,
      subtree: true,
      attributes: true,
    });
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

function buttonWithText(root: HTMLElement, text: string) {
  return (
    [...root.querySelectorAll<HTMLButtonElement>("button")].find((button) =>
      button.textContent?.replace(/\s+/g, " ").trim().includes(text),
    ) ?? null
  );
}

function activate(control: HTMLElement) {
  control.focus();
  control.click();
}

function replaceText(
  editor: HTMLTextAreaElement,
  value: string,
  inserted = value,
) {
  editor.focus();
  const setValue = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    "value",
  )?.set;
  setValue?.call(editor, value);
  editor.dispatchEvent(
    new InputEvent("input", {
      bubbles: true,
      data: inserted,
      inputType: "insertText",
    }),
  );
}

function waitForPresentation(
  delayMs: number,
  signal: AbortSignal,
): Promise<void> {
  if (signal.aborted) {
    return Promise.reject(new DOMException("Scenario reset", "AbortError"));
  }
  if (delayMs === 0) return Promise.resolve();
  return new Promise<void>((resolve, reject) => {
    const timer = globalThis.setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, delayMs);
    const onAbort = () => {
      globalThis.clearTimeout(timer);
      signal.removeEventListener("abort", onAbort);
      reject(new DOMException("Scenario reset", "AbortError"));
    };
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

async function typePrompt(
  editor: HTMLTextAreaElement,
  value: string,
  typeChunkDelayMs: number,
  signal: AbortSignal,
) {
  if (typeChunkDelayMs === 0) {
    replaceText(editor, value);
    return;
  }
  replaceText(editor, "");
  let previousEnd = 0;
  while (previousEnd < value.length) {
    if (signal.aborted) {
      throw new DOMException("Scenario reset", "AbortError");
    }
    const nextEnd = Math.min(previousEnd + TYPE_CHUNK_SIZE, value.length);
    replaceText(
      editor,
      value.slice(0, nextEnd),
      value.slice(previousEnd, nextEnd),
    );
    previousEnd = nextEnd;
    if (nextEnd === value.length) return;
    await waitForPresentation(typeChunkDelayMs, signal);
  }
}

function createDemoContext(
  api: StoryApiHarness,
  typeChunkDelayMs: number,
): DemoContext {
  api.reset();
  const clock = new ScenarioClock(Date.parse(fixtureDefaults.time));
  const controller = createScriptedStreamController({
    [SESSION_STREAM]: [
      {
        type: "message",
        data: {
          kind: "text_delta",
          text: "I’m wiring the production shell to deterministic fixtures…",
        },
      },
      {
        type: "message",
        data: {
          kind: "text_delta",
          text: " the core journey is now ready for browser review.",
        },
      },
      { type: "message", data: { kind: "discard" } },
      { type: "end" },
    ],
  });
  const streamWaiters = new Map<
    string,
    Set<(stream: ScriptedEventStream) => void>
  >();
  const servicesHarness = createStoryAppServices({
    clock,
    local: {
      "arwebui.supervision": "0",
    },
    streams: {
      open(path) {
        const stream = controller.open(path);
        for (const resolve of streamWaiters.get(path) ?? []) resolve(stream);
        streamWaiters.delete(path);
        return stream;
      },
    },
  });
  const store = createAppStore(servicesHarness.services);

  const context: DemoContext = {
    api,
    clock,
    controller,
    typeChunkDelayMs,
    store,
    services: servicesHarness.services,
    root: null,
    waitForStream(path, signal) {
      const openedStreams = controller.streams(path);
      let opened: ScriptedEventStream | undefined;
      for (let index = openedStreams.length - 1; index >= 0; index -= 1) {
        const candidate = openedStreams[index];
        if (!candidate.closed && candidate.onmessage !== null) {
          opened = candidate;
          break;
        }
      }
      if (opened) return Promise.resolve(opened);
      return new Promise<ScriptedEventStream>((resolve, reject) => {
        const listeners =
          streamWaiters.get(path) ??
          new Set<(stream: ScriptedEventStream) => void>();
        const settle = (stream: ScriptedEventStream) => {
          signal.removeEventListener("abort", onAbort);
          resolve(stream);
        };
        const onAbort = () => {
          listeners.delete(settle);
          reject(new DOMException("Scenario reset", "AbortError"));
        };
        listeners.add(settle);
        streamWaiters.set(path, listeners);
        signal.addEventListener("abort", onAbort, { once: true });
      });
    },
  };
  return context;
}

const steps: readonly DemoStep<DemoContext>[] = [
  {
    id: "open-project",
    title: "Open the project picker",
    async run(context, signal) {
      const trigger = await waitForElement(
        context,
        (root) =>
          root.querySelector<HTMLButtonElement>(
            'button[title="Select project"]',
          ),
        signal,
      );
      activate(trigger);
      await waitForElement(
        context,
        (root) => root.querySelector(".cx-project-popover"),
        signal,
      );
    },
  },
  {
    id: "select-project",
    title: "Choose storybook-demo",
    async run(context, signal) {
      const project = await waitForElement(
        context,
        (root) => {
          const picker = root.querySelector<HTMLElement>(".cx-project-popover");
          return picker ? buttonWithText(picker, "storybook-demo") : null;
        },
        signal,
      );
      activate(project);
    },
  },
  {
    id: "choose-build",
    title: "Choose Build",
    async run(context, signal) {
      const build = await waitForElement(
        context,
        (root) => buttonWithText(root, "Build a new feature, app, or tool"),
        signal,
      );
      activate(build);
      await waitForElement(
        context,
        (root) => buttonWithText(root, "Build UI changes"),
        signal,
      );
    },
  },
  {
    id: "choose-build-ui",
    title: "Choose Build UI changes",
    async run(context, signal) {
      const followup = await waitForElement(
        context,
        (root) => buttonWithText(root, "Build UI changes"),
        signal,
      );
      activate(followup);
      await waitForElement(
        context,
        (root) =>
          root.querySelector<HTMLTextAreaElement>(
            ".home-empty-state .cx-home textarea",
          ),
        signal,
      );
    },
  },
  {
    id: "type-request",
    title: "Type the request",
    async run(context, signal) {
      const editor = await waitForElement(
        context,
        (root) =>
          root.querySelector<HTMLTextAreaElement>(
            ".home-empty-state .cx-home textarea",
          ),
        signal,
      );
      await typePrompt(editor, PROMPT, context.typeChunkDelayMs, signal);
    },
  },
  {
    id: "open-access",
    title: "Open access options",
    async run(context, signal) {
      const access = await waitForElement(
        context,
        (root) => root.querySelector<HTMLButtonElement>(".cx-home .cx-mode"),
        signal,
      );
      activate(access);
      await waitForElement(
        context,
        (root) => buttonWithText(root, "Auto-accept edits"),
        signal,
      );
    },
  },
  {
    id: "select-access",
    title: "Select Auto-accept edits",
    async run(context, signal) {
      const acceptEdits = await waitForElement(
        context,
        (root) => buttonWithText(root, "Auto-accept edits"),
        signal,
      );
      activate(acceptEdits);
    },
  },
  {
    id: "open-model",
    title: "Open model options",
    async run(context, signal) {
      const model = await waitForElement(
        context,
        (root) => root.querySelector<HTMLButtonElement>(".cx-home .cx-model"),
        signal,
      );
      activate(model);
      await waitForElement(
        context,
        (root) =>
          [...root.querySelectorAll<HTMLButtonElement>("button")].find(
            (button) =>
              button.textContent
                ?.replace(/\s+/g, " ")
                .trim()
                .startsWith("Model"),
          ) ?? null,
        signal,
      );
    },
  },
  {
    id: "open-model-list",
    title: "Open the model list",
    async run(context, signal) {
      const modelPage = await waitForElement(
        context,
        (root) =>
          [...root.querySelectorAll<HTMLButtonElement>("button")].find(
            (button) =>
              button.textContent
                ?.replace(/\s+/g, " ")
                .trim()
                .startsWith("Model"),
          ) ?? null,
        signal,
      );
      activate(modelPage);
      await waitForElement(
        context,
        (root) => buttonWithText(root, "Gemini Pro"),
        signal,
      );
    },
  },
  {
    id: "select-model",
    title: "Select Gemini Pro",
    async run(context, signal) {
      const geminiPro = await waitForElement(
        context,
        (root) => buttonWithText(root, "Gemini Pro"),
        signal,
      );
      activate(geminiPro);
    },
  },
  {
    id: "send",
    title: "Send the request",
    async run(context, signal) {
      const send = await waitForElement(
        context,
        (root) =>
          root.querySelector<HTMLButtonElement>(
            'button[aria-label="Send message"]',
          ),
        signal,
      );
      activate(send);
      await waitForElement(
        context,
        (root) => root.querySelector(".session-topbar"),
        signal,
      );
      await context.waitForStream(SESSION_STREAM, signal);
    },
  },
  {
    id: "stream-first-chunk",
    title: "Stream the first response chunk",
    async run(context, signal) {
      const stream = await context.waitForStream(SESSION_STREAM, signal);
      if (!context.controller.next(stream)) {
        throw new Error("First response chunk was not available");
      }
    },
  },
  {
    id: "stream-second-chunk",
    title: "Stream the second response chunk",
    async run(context, signal) {
      const stream = await context.waitForStream(SESSION_STREAM, signal);
      if (!context.controller.next(stream)) {
        throw new Error("Second response chunk was not available");
      }
    },
  },
  {
    id: "persist-response",
    title: "Persist the streamed response",
    async run(context, signal) {
      // The stream owns the transient typing projection. The matching durable
      // journal page arrives independently, exactly as it does in production;
      // advancing the fixed clock triggers SessionView's real poll without a
      // sleep or a direct React/store mutation.
      context.api.appendEvents(SESSION_SID, [
        buildEnvelope({
          seq: 2,
          type: "generation_started",
          command_id: "story-command-1",
          payload: { gen_step: 1 },
        }),
        buildAssistantMessage({
          seq: 3,
          command_id: "story-command-1",
          payload: {
            item_id: "story-assistant-stream",
            turn_id: "story-turn-demo",
            message: {
              parts: [
                {
                  text: "I’m wiring the production shell to deterministic fixtures while the session remains active.",
                },
              ],
            },
          },
        }),
      ]);
      await context.clock.advanceBy(1000);
      await waitForElement(
        context,
        (root) =>
          [...root.querySelectorAll<HTMLElement>("*")].find((element) =>
            element.textContent?.includes("while the session remains active"),
          ) ?? null,
        signal,
      );
    },
  },
  {
    id: "environment",
    title: "Open Environment",
    async run(context, signal) {
      const environment = await waitForElement(
        context,
        (root) =>
          root.querySelector<HTMLButtonElement>(
            'button[aria-label="Environment"]',
          ),
        signal,
      );
      activate(environment);
      await waitForElement(
        context,
        (root) => root.querySelector('aside[aria-label="Environment"]'),
        signal,
      );
    },
  },
  {
    id: "publish-completion",
    title: "Publish the completion message",
    async run(context, signal) {
      context.api.appendEvents(SESSION_SID, [
        buildAssistantMessage({
          seq: 4,
          command_id: "story-command-1",
          payload: {
            item_id: "story-assistant-demo",
            turn_id: "story-turn-demo",
            message: {
              parts: [
                {
                  text: "Implemented the deterministic Core Session Playback demo with production components, stream states, and workspace review.",
                },
              ],
            },
          },
        }),
      ]);
      await context.clock.advanceBy(1000);
      await waitForElement(
        context,
        (root) =>
          [...root.querySelectorAll<HTMLElement>("*")].find((element) =>
            element.textContent?.includes(
              "Implemented the deterministic Core Session Playback demo",
            ),
          ) ?? null,
        signal,
      );
    },
  },
  {
    id: "complete-session",
    title: "Complete the session",
    async run(context, signal) {
      context.api.appendEvents(SESSION_SID, [
        buildEnvelope({
          seq: 5,
          type: "waiting_entered",
          command_id: "story-command-1",
          payload: { kind: "input" },
        }),
      ]);
      context.api.updateSession(SESSION_SID, {
        status: "completed",
        turns: 1,
      });
      context.api.setInspect(
        SESSION_SID,
        buildInspect({
          progress: [
            {
              id: "implementation",
              title: "Build production demo",
              status: "done",
            },
            {
              id: "browser",
              title: "Review in browser",
              status: "done",
            },
          ],
        }),
      );
      const stream = await context.waitForStream(SESSION_STREAM, signal);
      context.controller.next(stream);
      context.controller.next(stream);
      await context.clock.advanceBy(4000);
      await waitForElement(
        context,
        (root) => root.querySelector('button[aria-label="Send message"]'),
        signal,
      );
    },
  },
  {
    id: "review",
    title: "Open Changes",
    async run(context, signal) {
      const changes = await waitForElement(
        context,
        (root) =>
          root.querySelector<HTMLButtonElement>(
            'button[title="Review workspace changes"]',
          ),
        signal,
      );
      activate(changes);
      await waitForElement(
        context,
        (root) => root.querySelector(".diffwrap"),
        signal,
      );
    },
  },
  {
    id: "return-to-session",
    title: "Close Changes and return to the session",
    async run(context, signal) {
      const close = await waitForElement(
        context,
        (root) =>
          root.querySelector<HTMLButtonElement>(
            'button[aria-label="Close changes"]',
          ),
        signal,
      );
      activate(close);
      await waitForElement(
        context,
        (root) =>
          root.querySelector<HTMLButtonElement>(
            'button[aria-label="Send message"]',
          ),
        signal,
      );
    },
  },
];

export interface CoreSessionPlaybackProps {
  autoPlay: boolean;
  label?: string;
  playbackPace?: "human" | "automated" | "instant";
  stepLimit?: number;
}

export function CoreSessionPlayback({
  autoPlay: initialAutoPlay,
  label = "Core session playback",
  playbackPace = "human",
  stepLimit,
}: CoreSessionPlaybackProps) {
  const instantPlayback = playbackPace === "instant" || IS_COMPONENT_TEST;
  const stepDelayMs = instantPlayback
    ? 0
    : playbackPace === "automated"
      ? AUTOMATED_STEP_DELAY_MS
      : ({ step }: { step: DemoStep<DemoContext> }) =>
          HUMAN_STEP_DELAY_MS[step.id] ?? DEFAULT_HUMAN_STEP_DELAY_MS;
  const typeChunkDelayMs = instantPlayback
    ? 0
    : playbackPace === "automated"
      ? 0
      : HUMAN_TYPE_CHUNK_DELAY_MS;
  const [runner] = useState(
    () =>
      new ScenarioRunner<DemoContext>({
        context: createDemoContext(demoApi, typeChunkDelayMs),
        steps: steps.slice(0, stepLimit ?? steps.length),
        timing: createDemoScenarioTiming(stepDelayMs),
        recreateContext: () =>
          createDemoContext(demoApi, typeChunkDelayMs),
        disposeContext: async (context) => {
          context.controller.reset();
          context.root = null;
          // Reset publishes a new epoch before the fixture backend is
          // recreated. Give React one paint to unmount the previous AppRuntime
          // so its pollers cannot race the pristine API reset with stale sids.
          await new Promise<void>((resolve) => {
            globalThis.requestAnimationFrame(() => resolve());
          });
        },
      }),
  );
  const snapshot = useSyncExternalStore(
    runner.subscribe.bind(runner),
    runner.getSnapshot,
    runner.getSnapshot,
  );
  const [autoPlay, setAutoPlay] = useState(initialAutoPlay);
  const context = runner.getContext();
  const handleAutoPlayChange = useCallback(
    (enabled: boolean) => {
      setAutoPlay(enabled);
      if (!enabled) return;
      const current = runner.getSnapshot();
      if (current.status !== "idle" && current.status !== "paused") return;
      void runner.play("autoplay").catch(() => {
        // The runner publishes durable failure details in the transport.
      });
    },
    [runner],
  );

  useEffect(() => {
    if (!autoPlay) return;
    const current = runner.getSnapshot();
    if (current.status !== "idle" && current.status !== "paused") return;
    void runner.play("autoplay").catch(() => {
      // The runner publishes durable failure details in the transport.
    });
  }, [autoPlay, runner]);

  useEffect(
    () => () => {
      void runner.dispose();
    },
    [runner],
  );

  return (
    <div className="core-session-playback">
      <ScenarioControls
        runner={runner}
        label={label}
        autoPlay={autoPlay || snapshot.owner === "autoplay"}
        onAutoPlayChange={handleAutoPlayChange}
      />
      <div
        key={snapshot.epoch}
        className="core-session-playback-app"
        ref={(node) => {
          context.root = node;
        }}
      >
        {snapshot.status === "resetting" ? (
          <div className="grid min-h-[320px] place-items-center" role="status">
            Resetting demo…
          </div>
        ) : (
          <AppRuntime services={context.services} store={context.store}>
            <AppShell />
          </AppRuntime>
        )}
      </div>
    </div>
  );
}

const meta = {
  title: "Demos/Core Session Playback",
  component: CoreSessionPlayback,
  excludeStories: ["CoreSessionPlayback", "coreSessionDemoHandlers"],
  tags: ["!test"],
  parameters: {
    layout: "fullscreen",
    msw: { handlers: demoApi.handlers },
  },
  args: {
    autoPlay: false,
    playbackPace: "human",
  },
  argTypes: {
    autoPlay: {
      control: "boolean",
      description: "Start the deterministic journey automatically.",
    },
    playbackPace: {
      control: "inline-radio",
      options: ["human", "automated", "instant"],
      description:
        "Human-readable playback by default; faster modes are reserved for QA.",
    },
  },
} satisfies Meta<typeof CoreSessionPlayback>;

export const coreSessionDemoHandlers = demoApi.handlers;

export default meta;
type Story = StoryObj<typeof meta>;

export const Demo: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("region", { name: "Core session playback" }),
    ).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Play" })).toBeEnabled();
    await expect(
      await canvas.findByText("Build a new feature, app, or tool"),
    ).toBeVisible();
  },
};
