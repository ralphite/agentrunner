// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { TimelineView } from "./Timeline";
import type { BubbleItem } from "../timeline";

afterEach(cleanup);

const assistant = (key: string, text = "Done."): BubbleItem => ({
  kind: "assistant",
  key,
  text,
  ts: "2026-07-11T18:35:00Z",
});

describe("TR-1 — turn separator (Tailwind migration)", () => {
  it("renders timeline with multiple messages", () => {
    const { container } = render(
      <TimelineView
        items={[assistant("a1", "First."), assistant("a2", "Second.")]}
        pending={[]}
        typing=""
        showSys={false}
        onContinue={() => {}}
      />,
    );
    const messages = container.querySelectorAll(".msg");
    expect(messages.length).toBeGreaterThanOrEqual(2);
  });
});
