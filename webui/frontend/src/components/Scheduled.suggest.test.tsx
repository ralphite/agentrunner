// @vitest-environment jsdom
//
// SC-18 — the screen was lying. Each Suggestion card advertised a cadence
// ("Weekdays at 8:00 AM") as a hand-typed caption, and clicking it opened the
// launcher on the Repeating preset's default `interval: 5m`: you asked for a
// weekday-morning brief and got a session that fires every five minutes, forever.
// The card's words and the thing it built came from two different places, and
// the one you could see was the decorative one.
//
// These tests drive the real click through the real store into the real modal —
// no stubs between them — and pin that what the card SAYS is what the form is
// ARMED with. The last case is the one that matters: change the spec, and the
// card's words move with it. There is only one fact now.
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import { Scheduled, SUGGESTIONS } from "./Scheduled";
import { Modals } from "./Modals";
import { useStore } from "../store";
import { cadenceText } from "../runPreset";
import type { Session } from "../types";

afterEach(cleanup);

const mount = () => {
  useStore.setState({
    runs: [],
    sessions: [],
    sessionsReady: true,
    unread: [],
    archived: [],
    pinned: [],
    renames: {},
    modal: null,
  });
  return render(
    <>
      <Scheduled />
      <Modals />
    </>,
  );
};

const clickCard = (title: string) => fireEvent.click(screen.getByText(title).closest("button")!);

const cronInput = () => screen.getByPlaceholderText("0 * * * * (min hr dom mon dow)") as HTMLInputElement;
const scheduleSelect = () => screen.getByTitle("how iterations are paced") as HTMLSelectElement;

const manyScheduled: Session[] = [
  ["20260713-040000-first", "Newest scheduled row"],
  ["20260713-030000-second", "Second scheduled row"],
  ["20260713-020000-third", "Third scheduled row"],
  ["20260713-010000-fourth", "Oldest scheduled row"],
].map(([id, title]) => ({
  id,
  title,
  status: "idle",
  turns: 1,
  workspace: "/repo/app",
  kind: "driver",
  schedule: "interval",
  cadence: "Every 1h",
  nextRunAt: "2099-01-01T00:00:00Z",
}));

describe("SC-18 · clicking a suggestion builds the cadence it advertises", () => {
  it("keeps the two newest rows primary, then exposes Suggestions without dropping the remaining list", () => {
    useStore.setState({
      runs: [],
      sessions: manyScheduled,
      sessionsReady: true,
      unread: [],
      archived: [],
      pinned: [],
      renames: {},
      modal: null,
    });
    const { container } = render(
      <>
        <Scheduled />
        <Modals />
      </>,
    );

    const list = container.querySelector(".scheduled-list")!;
    const children = [...list.children];
    expect(children).toHaveLength(5);
    expect(children[0].textContent).toContain("Newest scheduled row");
    expect(children[1].textContent).toContain("Second scheduled row");
    expect(children[2]).toBe(container.querySelector("[data-testid='scheduled-suggestions']"));
    expect(children[3].textContent).toContain("Third scheduled row");
    expect(children[4].textContent).toContain("Oldest scheduled row");
    expect(container.querySelectorAll(".scheduled-row")).toHaveLength(4);

    const firstSuggestion = children[2].querySelector(".sched-suggest")!;
    const body = firstSuggestion.querySelector(".sched-suggest-body")!;
    const head = firstSuggestion.querySelector(".sched-suggest-head")!;
    const description = firstSuggestion.querySelector(".sched-suggest-desc")!;
    expect(body.classList).toContain("flex-col");
    expect(body.classList).toContain("gap-1");
    expect(head.classList).toContain("flex");
    expect(head.classList).toContain("flex-wrap");
    expect(head.classList).toContain("items-baseline");
    expect(head.classList).toContain("gap-x-2");
    expect(description.classList).toContain("block");
    expect((body as HTMLElement).style.cssText).toContain("display: flex");
    expect((body as HTMLElement).style.cssText).toContain("flex-direction: column");
    expect((head as HTMLElement).style.cssText).toContain("flex-wrap: wrap");
    expect((head as HTMLElement).style.cssText).toContain("column-gap: 8px");
    expect((description as HTMLElement).style.display).toBe("block");
    expect([...body.children]).toEqual([head, description]);

    fireEvent.click(firstSuggestion);
    expect(screen.getByRole("dialog", { name: "Schedule a run" })).toBeTruthy();
    expect(scheduleSelect().value).toBe("cron");
    expect(cronInput().value).toBe("0 8 * * 1-5");
  });

  it.each([
    ["Daily brief", "Weekdays at 8:00 AM", "0 8 * * 1-5"],
    ["Weekly review", "Fridays at 4:00 PM", "0 16 * * 5"],
    ["Follow-up monitor", "Every 6 hours", "0 */6 * * *"],
  ])("%s → the launcher opens on %s, not on Every 5m", (title, phrase, cron) => {
    mount();
    expect(screen.getByText(phrase)).toBeTruthy(); // the card's promise

    clickCard(title);

    // The launcher is armed with the card's rhythm…
    expect(scheduleSelect().value).toBe("cron");
    expect(cronInput().value).toBe(cron);
    // …and says so in the same words the card used.
    expect(screen.getByTestId("cadence-echo").textContent).toBe(phrase);
    // The 5m interval field — the old default — is not even on screen.
    expect(screen.queryByPlaceholderText("5m · 30s · 1h")).toBeNull();
  });

  it("carries the card's prompt text in as before", () => {
    mount();
    clickCard("Daily brief");
    const prompt = screen.getByPlaceholderText("Describe the outcome you want") as HTMLTextAreaElement;
    expect(prompt.value).toBe("Start each weekday with a summary of your priorities");
  });

  it("leaves the cadence editable — it is a default, not a decision", () => {
    mount();
    clickCard("Weekly review");
    expect(screen.getByTestId("cadence-echo").textContent).toBe("Fridays at 4:00 PM");

    fireEvent.change(cronInput(), { target: { value: "0 4 * * 6" } });
    expect(screen.getByTestId("cadence-echo").textContent).toBe("Saturdays at 4:00 AM");

    // And the user can still walk away from cron entirely.
    fireEvent.change(scheduleSelect(), { target: { value: "interval" } });
    expect(screen.getByTestId("cadence-echo").textContent).toBe("Every 5m");
  });

  it("still opens the bare Repeating preset on the interval default", () => {
    mount();
    fireEvent.click(screen.getByRole("button", { name: /Create/ }));
    fireEvent.click(screen.getByRole("menuitem", { name: /Repeating/ }));
    expect(scheduleSelect().value).toBe("interval");
    expect(screen.getByTestId("cadence-echo").textContent).toBe("Every 5m");
  });

  it("labels prompt and workspace independently and retains rapid input", () => {
    mount();
    fireEvent.click(screen.getByRole("button", { name: /Create/ }));
    fireEvent.click(screen.getByRole("menuitem", { name: /Repeating/ }));

    const prompt = screen.getByRole("textbox", { name: "Prompt" }) as HTMLTextAreaElement;
    const workspace = screen.getByRole("textbox", { name: "Workspace" }) as HTMLInputElement;
    fireEvent.change(prompt, { target: { value: "Say hello" } });
    fireEvent.change(workspace, { target: { value: "abc" } });

    expect(prompt.value).toBe("Say hello");
    expect(workspace.value).toBe("abc");
  });

  // The single-source-of-truth check: the card's caption is DERIVED. If someone
  // edits a cron below, the card's words follow — they cannot drift apart,
  // because the caption is not stored anywhere.
  it("renders every card's caption from the spec it will hand to the launcher", () => {
    mount();
    for (const s of SUGGESTIONS) {
      expect(screen.getByText(cadenceText(s.cadence))).toBeTruthy();
    }
  });
});
