import { describe, expect, it } from "vitest";
import { ACCESS_LEVELS, runtimeModeTarget } from "./specs";

// The session mode pill (INC-54) is only as honest as this map: which access
// levels the daemon will accept as a mid-session switch (INC-42's
// ValidTransition = default↔acceptEdits), and which are launch-time only.
describe("runtimeModeTarget", () => {
  it("maps Ask to approve to the /mode default target", () => {
    expect(runtimeModeTarget("ask")).toBe("default");
  });

  it("maps Auto-accept edits to the /mode acceptEdits target", () => {
    expect(runtimeModeTarget("acceptEdits")).toBe("acceptEdits");
  });

  it("refuses Full access at runtime (launch-time spec posture, not a fold mode)", () => {
    expect(runtimeModeTarget("full")).toBeNull();
  });

  it("refuses Plan at runtime (exits via exit_plan_mode approval, not a switch)", () => {
    expect(runtimeModeTarget("plan")).toBeNull();
  });

  it("keeps the two clickable levels a strict subset of the offered rows", () => {
    const clickable = ACCESS_LEVELS.filter((a) => runtimeModeTarget(a.id) !== null).map((a) => a.id);
    expect(clickable).toEqual(["ask", "acceptEdits"]);
  });
});
