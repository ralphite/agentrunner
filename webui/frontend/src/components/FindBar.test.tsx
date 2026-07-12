// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import { FindBar } from "./FindBar";

// A11Y-3 — ⌘F used to drop focus on <body> when it closed, so the next Tab
// restarted from the top of the document (the sidebar alone is hundreds of
// stops). These pin the Modal contract: whatever was focused before the find
// bar opened gets focus back when it goes away, however it goes away.

// Mirrors SessionView: the find bar is conditionally rendered, so closing it
// unmounts it.
function Harness() {
  const [open, setOpen] = useState(false);
  return (
    <>
      {/* fireEvent.click does not move focus in jsdom, so the button stays the
          activeElement while it opens the bar — exactly the ⌘F situation. */}
      <button onClick={() => setOpen(true)}>New task</button>
      {open && <FindBar scope={() => null} onClose={() => setOpen(false)} />}
    </>
  );
}

function openFind(): { trigger: HTMLElement; input: HTMLElement } {
  render(<Harness />);
  const trigger = screen.getByText("New task");
  trigger.focus();
  expect(document.activeElement).toBe(trigger);
  fireEvent.click(trigger);
  const input = screen.getByPlaceholderText("Search chat…");
  expect(document.activeElement).toBe(input);
  return { trigger, input };
}

afterEach(cleanup);

describe("FindBar focus return (A11Y-3)", () => {
  it("returns focus to the pre-open element on Escape", () => {
    const { trigger, input } = openFind();
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByPlaceholderText("Search chat…")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("returns focus to the pre-open element when the ✕ button closes it", () => {
    const { trigger } = openFind();
    fireEvent.click(screen.getByLabelText("Close find"));
    expect(document.activeElement).toBe(trigger);
  });

  it("returns focus when unmounted by something else (e.g. a session switch)", () => {
    const trigger = document.createElement("button");
    trigger.textContent = "New task";
    document.body.appendChild(trigger);
    trigger.focus();

    const { unmount } = render(<FindBar scope={() => null} onClose={() => {}} />);
    expect(document.activeElement).toBe(screen.getByPlaceholderText("Search chat…"));

    unmount();
    expect(document.activeElement).toBe(trigger);
    trigger.remove();
  });

  it("does not throw when the pre-open element left the DOM", () => {
    const trigger = document.createElement("button");
    document.body.appendChild(trigger);
    trigger.focus();

    const { unmount } = render(<FindBar scope={() => null} onClose={() => {}} />);
    trigger.remove();
    expect(() => unmount()).not.toThrow();
  });
});
