import { describe, expect, it } from "vitest";
import { scheduleFieldError, validCron, validGoDuration } from "./scheduleValidate";

describe("validGoDuration", () => {
  it("accepts Go duration shapes", () => {
    for (const ok of ["5m", "30s", "1h", "1h30m", "1.5h", "1000ms", "0.5s500ms", " 5m "]) {
      expect(validGoDuration(ok), ok).toBe(true);
    }
  });
  it("rejects prose and bare numbers", () => {
    for (const bad of ["", "5", "every 5 minutes", "5 m", "m5", "5mm", "1d", "300ms", "999999999ns"]) {
      expect(validGoDuration(bad), bad).toBe(false);
    }
  });
});

describe("validCron", () => {
  it("accepts 5-field expressions", () => {
    for (const ok of ["0 * * * *", "0 8 * * 1-5", "*/15 9-17 * * MON-FRI"]) {
      expect(validCron(ok), ok).toBe(true);
    }
  });
  it("rejects wrong field counts and prose", () => {
    for (const bad of ["", "* * * *", "0 0 * * * *", "every monday", "0 8 * * 1-5 extra"]) {
      expect(validCron(bad), bad).toBe(false);
    }
  });
});

describe("scheduleFieldError", () => {
  it("stays quiet on empty input", () => {
    expect(scheduleFieldError("interval", "")).toBe("");
    expect(scheduleFieldError("cron", "  ")).toBe("");
  });
  it("names the expected shape once something was typed", () => {
    expect(scheduleFieldError("interval", "every day")).toContain("Go duration");
    expect(scheduleFieldError("interval", "500ms")).toContain("at least 1s");
    expect(scheduleFieldError("cron", "0 0 * * * *")).toContain("5 space-separated");
    expect(scheduleFieldError("interval", "5m")).toBe("");
    expect(scheduleFieldError("cron", "0 * * * *")).toBe("");
  });
});
