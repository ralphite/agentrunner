import { describe, expect, it } from "vitest";
import { parseSlash, SLASH } from "./slash";

describe("slash command table", () => {
  it("registers /optimize for both home and session", () => {
    const cmd = SLASH.find((c) => c.name === "optimize");
    expect(cmd).toBeTruthy();
    expect(cmd!.needsArgs).toBe(true);
    expect(cmd!.variants).toEqual(["home", "session"]);
  });

  it("parses /optimize <draft> into the command + draft", () => {
    expect(parseSlash("/optimize fix the bug", "home")).toEqual({ cmd: "optimize", rest: "fix the bug" });
    expect(parseSlash("/optimize clean it up", "session")).toEqual({ cmd: "optimize", rest: "clean it up" });
  });

  it("leaves a bare /optimize unrun so the menu can complete it (needsArgs)", () => {
    expect(parseSlash("/optimize", "home")).toBeNull();
    expect(parseSlash("/optimize   ", "home")).toBeNull();
  });

  it("does not match unknown or wrong-variant commands", () => {
    expect(parseSlash("/nope hi", "home")).toBeNull();
    // compact is session-only
    expect(parseSlash("/compact", "home")).toBeNull();
    expect(parseSlash("/compact", "session")).toEqual({ cmd: "compact", rest: "" });
    expect(parseSlash("/compact preserve API decisions", "session")).toEqual({
      cmd: "compact",
      rest: "preserve API decisions",
    });
  });
});
