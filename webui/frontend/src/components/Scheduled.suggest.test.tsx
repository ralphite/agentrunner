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

describe("SC-18 · clicking a suggestion builds the cadence it advertises", () => {
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
