import { afterEach, describe, expect, it, vi } from "vitest";
import { relTimeAgo } from "./time";

afterEach(() => vi.useRealTimers());

describe("relTimeAgo", () => {
  it("does not render just now ago", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-12T20:00:30Z"));
    expect(relTimeAgo(new Date("2026-07-12T20:00:00Z"))).toBe("just now");
  });

  it("adds ago to compact older stamps", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-12T20:02:00Z"));
    expect(relTimeAgo(new Date("2026-07-12T20:00:00Z"))).toBe("2m ago");
  });
});
