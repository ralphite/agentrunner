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
  it("labels each section so the model can tell a path from a half-typed draft", () => {
    expect(
      helperContext({
        workspace: "  /repo/auth ",
        recent: [
          { role: "user", text: "deploy arwebui to 8809" },
          { role: "assistant", text: "Done — the daemon is on the versioned path." },
        ],
        draft: "把 daemon 那个",
      }),
    ).toBe(
      "# Project\n/repo/auth\n\n" +
        "# Recent conversation\nuser: deploy arwebui to 8809\nassistant: Done — the daemon is on the versioned path.\n\n" +
        "# Draft so far\n把 daemon 那个",
    );
  });

  it("drops empty sections entirely — no bare headers", () => {
    expect(helperContext({ workspace: "/repo/auth" })).toBe("# Project\n/repo/auth");
    expect(helperContext({ draft: "  just typing  " })).toBe("# Draft so far\njust typing");
    expect(helperContext({})).toBe("");
    expect(helperContext({ workspace: " ", recent: [], draft: null })).toBe("");
  });

  it("keeps only the last few turns, oldest-first, one line each", () => {
    const recent = Array.from({ length: 10 }, (_, i) => ({
      role: (i % 2 ? "assistant" : "user") as "user" | "assistant",
      text: `turn ${i}`,
    }));
    const out = helperContext({ recent });
    expect(out).toBe(
      "# Recent conversation\n" +
        ["user: turn 4", "assistant: turn 5", "user: turn 6", "assistant: turn 7", "user: turn 8", "assistant: turn 9"].join("\n"),
    );
  });

  it("collapses newlines inside a turn so one turn stays one line", () => {
    const out = helperContext({ recent: [{ role: "assistant", text: "line one\n\nline two" }] });
    expect(out).toBe("# Recent conversation\nassistant: line one line two");
  });

  it("clips a long agent reply instead of letting it flood the prompt", () => {
    const out = helperContext({ recent: [{ role: "assistant", text: "x".repeat(5000) }] });
    const body = out.split("\n")[1];
    expect(body.length).toBeLessThan(250); // 200-char cap + "assistant: " + ellipsis
    expect(body.endsWith("…")).toBe(true);
  });

  it("spends the budget newest-first, dropping the oldest turns", () => {
    // Six max-length turns overrun the section budget, so the oldest fall off
    // — what was just said is what the transcriber needs.
    const recent = Array.from({ length: 6 }, (_, i) => ({
      role: "user" as const,
      text: `turn${i} ` + "a".repeat(300),
    }));
    const out = helperContext({ recent });
    expect(out).toContain("user: turn5 "); // newest kept
    expect(out).not.toContain("user: turn0 "); // oldest dropped
    expect(out.length).toBeLessThan(1600); // section budget honored
    // Still in reading order, oldest of the survivors first.
    expect(out.indexOf("turn2")).toBeLessThan(out.indexOf("turn5"));
  });

  it("caps the draft too — a pasted essay can't blow the arg limit", () => {
    const out = helperContext({ draft: "y".repeat(5000) });
    expect(out.length).toBeLessThan(900);
    expect(out.endsWith("…")).toBe(true);
  });
});
