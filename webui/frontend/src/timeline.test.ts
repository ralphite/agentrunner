import { describe, expect, it } from "vitest";
import { foldEvents } from "./timeline";

describe("timeline input projection", () => {
  it("keeps human input as a user message", () => {
    const folded = foldEvents([{ seq: 1, type: "input_received", payload: { source: "cli", text: "hello" } }]);
    expect(folded.items).toContainEqual(expect.objectContaining({ kind: "user", text: "hello" }));
  });

  it("projects program and agent input as collapsible runtime events", () => {
    const folded = foldEvents([
      { seq: 1, type: "input_received", payload: { source: "program", text: "<goal>keep going</goal>" } },
      { seq: 2, type: "input_received", payload: { source: "agent", text: "review complete" } },
    ]);
    expect(folded.items).toEqual([
      expect.objectContaining({ kind: "runtime", source: "program", text: "<goal>keep going</goal>" }),
      expect.objectContaining({ kind: "runtime", source: "agent", text: "review complete" }),
    ]);
    expect(folded.items.some((item) => item.kind === "user")).toBe(false);
  });
});
