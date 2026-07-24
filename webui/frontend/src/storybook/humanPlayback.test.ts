// @vitest-environment jsdom

import { describe, expect, it } from "vitest";
import { userEvent as storybookUserEvent } from "storybook/test";
import {
  getStoryPlaybackPace,
  humanPause,
  pacedUserEvent,
  pacedUserEventMethods,
  resolveStoryPlaybackTiming,
  setStoryPlaybackPace,
} from "./humanPlayback";

describe("Story playback timing", () => {
  it("keeps human, automated, and instant clocks distinct", () => {
    expect(resolveStoryPlaybackTiming("human")).toEqual({
      actionPauseMs: 1600,
      keyboardDelayMs: 180,
      typeDelayMs: 48,
    });
    expect(resolveStoryPlaybackTiming("automated", 3800)).toEqual({
      actionPauseMs: 400,
      keyboardDelayMs: 0,
      typeDelayMs: 0,
    });
    expect(resolveStoryPlaybackTiming("instant")).toEqual(
      {
        actionPauseMs: 0,
        keyboardDelayMs: 0,
        typeDelayMs: 0,
      },
    );
  });

  it("never lets a component test inherit the human browser clock", async () => {
    setStoryPlaybackPace("human");
    expect(getStoryPlaybackPace()).toBe("instant");
    await expect(humanPause()).resolves.toBeUndefined();
  });

  it("paces every direct user-event action and keeps setup inside the proxy", async () => {
    expect(
      Object.keys(storybookUserEvent).filter(
        (method) =>
          method !== "setup" &&
          !pacedUserEventMethods.includes(
            method as (typeof pacedUserEventMethods)[number],
          ),
      ),
    ).toEqual([]);
    expect(typeof pacedUserEvent.setup).toBe("function");

    const input = document.createElement("input");
    document.body.append(input);
    const typing = pacedUserEvent.setup({ delay: 50 }).type(input, "human");
    await new Promise((resolve) => globalThis.setTimeout(resolve, 75));
    expect(input.value.length).toBeLessThan("human".length);
    await typing;
    expect(input.value).toBe("human");
    input.remove();
  });
});
