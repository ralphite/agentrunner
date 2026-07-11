import { describe, expect, it } from "vitest";
import { diffPath } from "./api";

describe("diffPath", () => {
  it("keeps Working tree and Last turn as explicit backend scopes", () => {
    expect(diffPath("session-1", "working-tree")).toBe("/sessions/session-1/diff?scope=working-tree");
    expect(diffPath("session-1", "last-turn")).toBe("/sessions/session-1/diff?scope=last-turn");
  });
});
