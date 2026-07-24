import { describe, expect, it, vi } from "vitest";
import { ScenarioClock } from "./ScenarioClock";

describe("ScenarioClock", () => {
  it("advances timeouts and intervals in deterministic due order", async () => {
    const clock = new ScenarioClock(1_000);
    const calls: string[] = [];
    const interval = clock.setInterval(() => calls.push(`interval:${clock.now()}`), 50);
    clock.setTimeout(() => calls.push(`timeout:${clock.now()}`), 75);

    await clock.advanceBy(110);
    clock.clearInterval(interval);
    await clock.advanceBy(100);

    expect(calls).toEqual([
      "interval:1050",
      "timeout:1075",
      "interval:1100",
    ]);
    expect(clock.now()).toBe(1_210);
  });

  it("clears pending callbacks and rejects invalid advances", async () => {
    const clock = new ScenarioClock(0);
    const callback = vi.fn();
    const handle = clock.setTimeout(callback, 10);
    clock.clearTimeout(handle);
    await clock.advanceBy(20);
    expect(callback).not.toHaveBeenCalled();
    await expect(clock.advanceBy(-1)).rejects.toThrow(/non-negative/);
  });
});
