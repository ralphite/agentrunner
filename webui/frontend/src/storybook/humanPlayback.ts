import { userEvent as storybookUserEvent } from "storybook/test";

export type StoryPlaybackPace = "human" | "automated" | "instant";

const DEFAULT_HUMAN_STEP_MS = 1600;
const AUTOMATED_STEP_MS = 400;
const HUMAN_TYPE_DELAY_MS = 48;
const HUMAN_KEY_DELAY_MS = 180;
export const pacedUserEventMethods = [
  "clear",
  "click",
  "copy",
  "cut",
  "dblClick",
  "deselectOptions",
  "hover",
  "keyboard",
  "paste",
  "pointer",
  "selectOptions",
  "tab",
  "tripleClick",
  "type",
  "unhover",
  "upload",
] as const;

const PACED_METHODS = new Set<string>(pacedUserEventMethods);

let selectedPace: StoryPlaybackPace = "human";

function isComponentTest(): boolean {
  return (
    import.meta.env.MODE === "test" ||
    String(import.meta.env.VITEST) === "true"
  );
}

export function setStoryPlaybackPace(pace: StoryPlaybackPace): void {
  selectedPace = pace;
}

export function getStoryPlaybackPace(): StoryPlaybackPace {
  return isComponentTest() ? "instant" : selectedPace;
}

export function resolveStoryPlaybackTiming(
  pace: StoryPlaybackPace,
  humanStepMs = DEFAULT_HUMAN_STEP_MS,
): {
  actionPauseMs: number;
  keyboardDelayMs: number;
  typeDelayMs: number;
} {
  if (pace === "instant") {
    return { actionPauseMs: 0, keyboardDelayMs: 0, typeDelayMs: 0 };
  }
  if (pace === "automated") {
    return {
      actionPauseMs: Math.min(humanStepMs, AUTOMATED_STEP_MS),
      keyboardDelayMs: 0,
      typeDelayMs: 0,
    };
  }
  return {
    actionPauseMs: humanStepMs,
    keyboardDelayMs: HUMAN_KEY_DELAY_MS,
    typeDelayMs: HUMAN_TYPE_DELAY_MS,
  };
}

// Story play functions are both executable QA and an in-browser explanation.
// Human pacing is the default even in a browser controlled by automation:
// navigator.webdriver describes the browser, not the viewer's intent. CI and
// visual QA must opt into automated/instant explicitly.
export async function humanPause(
  humanStepMs = DEFAULT_HUMAN_STEP_MS,
): Promise<void> {
  const { actionPauseMs } = resolveStoryPlaybackTiming(
    getStoryPlaybackPace(),
    humanStepMs,
  );
  if (actionPauseMs === 0) return;
  await new Promise<void>((resolve) => {
    globalThis.setTimeout(resolve, actionPauseMs);
  });
}

function withDelayOption(
  args: unknown[],
  optionsIndex: number,
  delay: number,
): unknown[] {
  if (delay === 0) return args;
  const options =
    args[optionsIndex] && typeof args[optionsIndex] === "object"
      ? (args[optionsIndex] as Record<string, unknown>)
      : {};
  if (typeof options.delay === "number") return args;
  const next = [...args];
  next[optionsIndex] = { ...options, delay };
  return next;
}

function withHumanInputDelay(
  property: string,
  args: unknown[],
): unknown[] {
  const { keyboardDelayMs, typeDelayMs } = resolveStoryPlaybackTiming(
    getStoryPlaybackPace(),
  );
  if (property === "type") {
    return withDelayOption(args, 2, typeDelayMs);
  }
  if (property === "keyboard") {
    // A descriptor sequence such as {End}{Enter} or
    // {Shift>}{Tab}{/Shift} is several observable human key actions, not one
    // atomic shortcut. user-event's direct API accepts the same delay option
    // and preserves modifier state while yielding between descriptors.
    return withDelayOption(args, 1, keyboardDelayMs);
  }
  return args;
}

function createPacedUserEventProxy<T extends object>(
  target: T,
  setupDelay?: number,
  setupInstance = false,
): T {
  return new Proxy(target, {
    get(currentTarget, property, receiver) {
      const value = Reflect.get(currentTarget, property, receiver);
      if (typeof property !== "string" || typeof value !== "function") {
        return value;
      }
      // setup() returns a new user-event instance. Proxy that instance too so
      // a Story cannot accidentally escape pacing by choosing the session API.
      if (property === "setup") {
        return (...inputArgs: unknown[]) => {
          const options =
            inputArgs[0] && typeof inputArgs[0] === "object"
              ? (inputArgs[0] as Record<string, unknown>)
              : undefined;
          return createPacedUserEventProxy(
            Reflect.apply(value, currentTarget, inputArgs) as object,
            typeof options?.delay === "number" ? options.delay : setupDelay,
            true,
          );
        };
      }
      if (!PACED_METHODS.has(property)) return value.bind(currentTarget);
      return async (...inputArgs: unknown[]) => {
        let actionTarget = currentTarget;
        let action = value;
        let args = inputArgs;
        if (
          setupInstance &&
          (property === "type" || property === "keyboard")
        ) {
          const { keyboardDelayMs, typeDelayMs } =
            resolveStoryPlaybackTiming(getStoryPlaybackPace());
          const delay =
            setupDelay ??
            (property === "type" ? typeDelayMs : keyboardDelayMs);
          // user-event's setup instance reads delay from its instance config;
          // unlike the direct API, type/keyboard call options do not override
          // that config. setupSub keeps the same keyboard/pointer System, so
          // modifier state survives while each method gets the right clock.
          const setupAction = Reflect.get(
            currentTarget,
            "setup",
          ) as (...args: unknown[]) => object;
          actionTarget = Reflect.apply(
            setupAction,
            currentTarget,
            [{ delay }],
          ) as T;
          action = Reflect.get(actionTarget, property) as typeof value;
        } else {
          args = withHumanInputDelay(property, inputArgs);
        }
        const result = await Reflect.apply(action, actionTarget, args);
        await humanPause();
        return result;
      };
    },
  });
}

// Every Story-visible user action goes through this proxy. Keeping the public
// shape identical to Storybook's userEvent lets existing play functions stay
// readable while one lintable seam owns human/automation timing.
export const pacedUserEvent = createPacedUserEventProxy(
  storybookUserEvent,
) as typeof storybookUserEvent;
