import { describe, expect, it } from "vitest";
import { cadenceText, cronPhrase, goDurationSeconds, runFormDefaults, runPresetDefaults } from "./runPreset";
import { SUGGESTIONS } from "./components/Scheduled";

describe("Scheduled Create presets", () => {
  it.each([
    ["one-time", { kind: "submit", schedule: "immediate" }],
    ["goal", { kind: "drive", schedule: "immediate" }],
    ["repeating", { kind: "drive", schedule: "interval" }],
    ["best-of-n", { kind: "drive", schedule: "parallel" }],
  ] as const)("maps %s to the real run launcher state", (preset, expected) => {
    expect(runPresetDefaults(preset)).toEqual(expected);
  });
});

// cadenceText mirrors webui/schedule.go's cadenceOf/cronPhrase. These are the Go
// tests' own cases (schedule_test.go): if the two renderers ever drift, a card
// promises one rhythm and the row it creates reports another.
describe("cadenceText (mirror of webui/schedule.go cadenceOf)", () => {
  it.each([
    [{ schedule: "cron", cron: "0 4 * * 6" }, "Saturdays at 4:00 AM"],
    [{ schedule: "cron", cron: "0 8 * * 1-5" }, "Weekdays at 8:00 AM"],
    [{ schedule: "cron", cron: "0 16 * * 5" }, "Fridays at 4:00 PM"],
    [{ schedule: "cron", cron: "0 */6 * * *" }, "Every 6 hours"],
    [{ schedule: "cron", cron: "*/15 * * * *" }, "Every 15 minutes"],
    [{ schedule: "cron", cron: "* * * * *" }, "Every minute"],
    [{ schedule: "cron", cron: "30 * * * *" }, "Hourly at :30"],
    [{ schedule: "cron", cron: "0 9 * * 0,6" }, "Weekends at 9:00 AM"],
    [{ schedule: "cron", cron: "0 12 * * 1,3" }, "Mon, Wed at 12:00 PM"],
    [{ schedule: "cron", cron: "0 0 1 * *" }, "Monthly on the 1st at 12:00 AM"],
    [{ schedule: "interval", interval: "30m" }, "Every 30m"],
    [{ schedule: "interval", interval: "1h30m" }, "Every 1h30m"],
    [{ schedule: "interval", interval: "24h" }, "Every 1d"],
    [{ schedule: "interval", interval: "" }, "Continuously"],
    [{ schedule: "parallel", n: 4 }, "Best of 4"],
    [{ schedule: "parallel" }, "Best of N"],
    [{ schedule: "immediate" }, "Runs once"],
  ] as const)("renders %j as its human phrase", (spec, want) => {
    expect(cadenceText(spec)).toBe(want);
  });

  // A shape we cannot phrase degrades to the expression itself — honest, never
  // an invented rhythm.
  it("degrades an unphraseable / invalid cron to the raw expression", () => {
    expect(cronPhrase("0 8 * *")).toBe("Cron 0 8 * *"); // four fields
    expect(cronPhrase("0 8,17 3 * 2")).toBe("Cron 0 8,17 3 * 2");
    expect(cadenceText({ schedule: "cron", cron: "nonsense" })).toBe("Cron nonsense");
  });

  it("parses the Go duration dialect the driver spec uses", () => {
    expect(goDurationSeconds("6h")).toBe(21600);
    expect(goDurationSeconds("1h30m")).toBe(5400);
    expect(goDurationSeconds("90s")).toBe(90);
    expect(goDurationSeconds("later")).toBeNull();
  });
});

// SC-18 — the screen must not lie. A suggestion card advertises a rhythm; the
// launcher it opens has to be armed with THAT rhythm, not the Repeating preset's
// generic `interval: 5m`.
describe("SC-18 · a suggestion's cadence survives the click into the launcher", () => {
  it("prefills the launcher from the cadence instead of the preset default", () => {
    const daily = SUGGESTIONS.find((s) => s.title === "Daily brief")!;
    expect(cadenceText(daily.cadence)).toBe("Weekdays at 8:00 AM");

    const form = runFormDefaults("repeating", daily.cadence);
    expect(form).toEqual({ kind: "drive", schedule: "cron", cron: "0 8 * * 1-5", interval: "5m", n: 3 });
  });

  it.each([
    ["Daily brief", "Weekdays at 8:00 AM"],
    ["Weekly review", "Fridays at 4:00 PM"],
    ["Follow-up monitor", "Every 6 hours"],
  ])("card %s promises %s — and arms the form with exactly that", (title, phrase) => {
    const s = SUGGESTIONS.find((x) => x.title === title)!;
    expect(cadenceText(s.cadence)).toBe(phrase); // what the card SAYS
    const form = runFormDefaults("repeating", s.cadence);
    expect(form.schedule).toBe(s.cadence.schedule);
    expect(form.cron).toBe(s.cadence.cron);
    // …and what the form is armed with renders back to the same promise.
    expect(cadenceText({ schedule: form.schedule, cron: form.cron, interval: form.interval })).toBe(phrase);
  });

  it("leaves the plain presets untouched when no cadence is handed over", () => {
    expect(runFormDefaults("repeating")).toEqual({
      kind: "drive",
      schedule: "interval",
      interval: "5m",
      cron: "0 * * * *",
      n: 3,
    });
    expect(runFormDefaults("one-time")).toMatchObject({ kind: "submit", schedule: "immediate" });
    expect(runFormDefaults("best-of-n")).toMatchObject({ kind: "drive", schedule: "parallel", n: 3 });
  });
});
