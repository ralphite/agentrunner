import { describe, expect, it } from "vitest";
import { compactCount, summarizeInspect } from "./inspectPresentation";

describe("inspect presentation", () => {
  it("summarizes approval runs without surfacing command-line answer instructions", () => {
    const summary = summarizeInspect({
      spec: "reviewer",
      model: "gemini-flash-latest",
      mode: "default",
      status: "waiting",
      gen_steps: 3,
      turns: 1,
      usage: { input_tokens: 1200, output_tokens: 34, billed: 1234 },
      waiting: { kind: "approval", tool: "bash", args: '{"command":"git status"}', answer_with: "secret cli command" },
      entries: [
        { kind: "llm", name: "complete" },
        { kind: "tool", name: "bash", detail: "git status", verdict: "deny" },
      ],
    });
    expect(summary.status).toMatchObject({ text: "Needs approval", cls: "appr" });
    expect(summary.waiting).toEqual({ title: "Approval required", subject: "git status" });
    expect(summary.activity).toMatchObject({ modelCalls: 1, toolCalls: 1, blocked: 1 });
    expect(JSON.stringify(summary)).not.toContain("secret cli command");
  });

  it("projects provider capabilities and tolerates unknown payloads", () => {
    expect(summarizeInspect(null)).toMatchObject({ spec: "Default agent", steps: 0, agents: 0 });
    expect(summarizeInspect({ provider_capabilities: {
      provider: "gemini",
      input_modalities: ["text", "image"],
      capabilities: { thinking: true, files: false },
    } })).toMatchObject({ provider: "gemini", modalities: ["text", "image"], capabilities: ["thinking"] });
  });

  it("deduplicates revived agents and keeps ordinary input waits quiet", () => {
    const summary = summarizeInspect({
      reason: "completed",
      waiting: { kind: "input" },
      children: [
        { session: "child-1", call_id: "old" },
        { session: "child-1", call_id: "revived" },
        { session: "child-2", call_id: "other" },
      ],
    });
    expect(summary.agents).toBe(2);
    expect(summary.waiting).toBeUndefined();
    expect(summary.status.text).toBe("Completed");
  });

  it("uses the restart-aware session status over a stale inspect liveness shape", () => {
    expect(summarizeInspect({ status: "running" }, "stranded").status).toEqual({
      text: "Needs recovery",
      cls: "stranded",
    });
  });

  it("formats large counts compactly", () => {
    expect(compactCount(1234)).toMatch(/1[.,]2K/i);
    expect(compactCount(12)).toBe("12");
  });
});
