export type RunPreset = "one-time" | "goal" | "repeating" | "best-of-n";

export function runPresetDefaults(preset: RunPreset): { kind: "submit" | "drive"; schedule: "immediate" | "interval" | "parallel" } {
  if (preset === "one-time") return { kind: "submit", schedule: "immediate" };
  if (preset === "repeating") return { kind: "drive", schedule: "interval" };
  if (preset === "best-of-n") return { kind: "drive", schedule: "parallel" };
  return { kind: "drive", schedule: "immediate" };
}
