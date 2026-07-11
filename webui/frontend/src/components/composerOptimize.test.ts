import { describe, expect, it, vi } from "vitest";
import { helperContext, runOptimize, undoOptimize } from "./composerOptimize";

function harness(optimize: (d: string, c: string) => Promise<{ text: string }>) {
  const calls = {
    text: [] as string[],
    undo: [] as (string | null)[],
    toasts: [] as string[],
    errors: [] as string[],
  };
  const io = {
    optimize,
    setText: (t: string) => calls.text.push(t),
    setUndo: (o: string | null) => calls.undo.push(o),
    toast: (m: string) => calls.toasts.push(m),
    onError: (m: string) => calls.errors.push(m),
  };
  return { io, calls };
}

describe("runOptimize", () => {
  it("swaps in the rewrite and stashes the restore draft for undo", async () => {
    const { io, calls } = harness(async () => ({ text: "  Fix the auth-token refresh.  " }));
    await runOptimize(io, "fix the thing", "fix the thing", "ctx");

    expect(calls.text).toEqual(["Fix the auth-token refresh."]); // trimmed rewrite
    expect(calls.undo).toEqual(["fix the thing"]); // undo snapshot = the restore text
    expect(calls.errors).toEqual([]);
    expect(calls.toasts.length).toBe(1);
  });

  it("passes the draft + context through to ar", async () => {
    const optimize = vi.fn(async () => ({ text: "clearer" }));
    const { io } = harness(optimize);
    await runOptimize(io, "  draft  ", "restore", "working in auth");
    expect(optimize).toHaveBeenCalledWith("draft", "working in auth"); // draft trimmed
  });

  it("no-ops on an empty draft (no ar call, no undo state)", async () => {
    const optimize = vi.fn(async () => ({ text: "x" }));
    const { io, calls } = harness(optimize);
    await runOptimize(io, "   ", "   ", "");
    expect(optimize).not.toHaveBeenCalled();
    expect(calls.text).toEqual([]);
    expect(calls.undo).toEqual([]);
  });

  it("leaves the draft untouched when the model returns nothing", async () => {
    const { io, calls } = harness(async () => ({ text: "   " }));
    await runOptimize(io, "draft", "draft", "");
    expect(calls.text).toEqual([]); // never overwrote the draft with empty
    expect(calls.undo).toEqual([]); // no undo affordance for a no-op
    expect(calls.toasts.length).toBe(1); // "returned nothing" notice
  });

  it("surfaces an ar failure and never mutates the draft", async () => {
    const { io, calls } = harness(async () => {
      throw new Error("ar optimize: daemon unreachable");
    });
    await runOptimize(io, "draft", "draft", "");
    expect(calls.errors).toEqual(["ar optimize: daemon unreachable"]);
    expect(calls.text).toEqual([]);
    expect(calls.undo).toEqual([]);
  });
});

describe("undoOptimize", () => {
  it("restores the original draft and clears the affordance", () => {
    const calls = { text: [] as string[], undo: [] as (string | null)[] };
    undoOptimize({ setText: (t) => calls.text.push(t), setUndo: (o) => calls.undo.push(o) }, "my original draft");
    expect(calls.text).toEqual(["my original draft"]);
    expect(calls.undo).toEqual([null]);
  });
});

describe("helperContext", () => {
  it("joins non-empty, trimmed fragments and drops the blanks", () => {
    expect(helperContext(["  /repo/auth ", "", null, undefined, "draft so far"])).toBe("/repo/auth\ndraft so far");
    expect(helperContext([" ", "", null])).toBe("");
  });
});
