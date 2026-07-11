import { describe, expect, it } from "vitest";
import { runPresetDefaults } from "./runPreset";

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
