import { describe, expect, it } from "vitest";
import { foldEvents } from "../../timeline";
import {
  buildDiff,
  buildEnvelope,
  buildInspect,
  buildSession,
  buildTimeline,
  cloneFixture,
} from "./builders";

describe("Storybook fixture builders", () => {
  it("returns independent nested values on every build", () => {
    const first = buildSession();
    const second = buildSession();

    first.attention!.approvals = 3;

    expect(second.attention).toEqual({ approvals: 0, answers: 0 });
    expect(first).not.toBe(second);
    expect(first.attention).not.toBe(second.attention);
  });

  it("merges typed nested overrides without sharing their references", () => {
    const payload = { text: "A fixture-only prompt", metadata: { source: "story" } };
    const envelope = buildEnvelope({ payload });
    const inspect = buildInspect({
      usage: { billed: 42 },
    });

    payload.metadata.source = "mutated";

    expect(envelope.payload.metadata.source).toBe("story");
    expect(inspect.usage).toEqual({
      input_tokens: 900,
      output_tokens: 300,
      cache_read: 0,
      cache_write: 0,
      billed: 42,
    });
  });

  it("clones arrays and maps represented as JSON objects", () => {
    const diff = buildDiff();
    const cloned = cloneFixture(diff);

    diff.untracked.push("src/Another.stories.tsx");
    diff.untrackedReasons!["src/Card.stories.tsx"] = "large";

    expect(cloned.untracked).toEqual(["src/Card.stories.tsx"]);
    expect(cloned.untrackedReasons).toEqual({});
  });

  it("builds journal events that project into real user and assistant rows", () => {
    const projected = foldEvents(buildTimeline());

    expect(projected.items).toEqual(expect.arrayContaining([
      expect.objectContaining({
        kind: "user",
        text: "Show the reusable component states.",
      }),
      expect.objectContaining({
        kind: "assistant",
        text: "The component states are ready for review.",
      }),
    ]));
  });
});
