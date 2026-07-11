import { beforeEach, describe, expect, it, vi } from "vitest";
import { AR } from "./api";
import { mergeSessionRows, useStore } from "./store";
import type { Session } from "./types";

const row = (id: string, turns = 1): Session => ({ id, status: "completed", turns, title: id });

describe("progressive session hydration", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    useStore.setState({ sessions: [], sessionsReady: false, sessionsLoadingOlder: false, unread: [] });
  });

  it("loads a recent page first, appends history, and preserves history on later refresh", async () => {
    const recent = Array.from({ length: 40 }, (_v, i) => row(`recent-${i}`));
    const older = [row("older-a"), row("older-b")];
    const calls: Array<[number, number]> = [];
    const sessionsSpy = vi.spyOn(AR, "sessions").mockImplementation(async (limit = 0, offset = 0) => {
      calls.push([limit, offset]);
      if (offset === 0) return recent;
      if (offset === 40) return older;
      return [];
    });

    await useStore.getState().refreshSessions();
    expect(calls).toEqual([[40, 0], [80, 40]]);
    expect(useStore.getState().sessions).toHaveLength(42);
    expect(useStore.getState().sessionsReady).toBe(true);
    expect(useStore.getState().sessionsLoadingOlder).toBe(false);

    calls.length = 0;
    sessionsSpy.mockClear();
    const updated = [row("recent-0", 2), ...recent.slice(1)];
    sessionsSpy.mockResolvedValue(updated);
    await useStore.getState().refreshSessions();
    expect(sessionsSpy).toHaveBeenCalledTimes(1);
    expect(sessionsSpy).toHaveBeenCalledWith(40, 0);
    expect(useStore.getState().sessions).toHaveLength(42);
    expect(useStore.getState().sessions.find((session) => session.id === "older-a")).toBeTruthy();
    expect(useStore.getState().sessions[0].turns).toBe(2);
  });

  it("coalesces overlapping interval refreshes into one request chain", async () => {
    let release!: (rows: Session[]) => void;
    const pending = new Promise<Session[]>((resolve) => { release = resolve; });
    const request = vi.spyOn(AR, "sessions").mockReturnValue(pending);

    const first = useStore.getState().refreshSessions();
    const second = useStore.getState().refreshSessions();
    expect(request).toHaveBeenCalledTimes(1);
    release([row("only")]);
    await Promise.all([first, second]);
    expect(request).toHaveBeenCalledTimes(1);
  });

  it("deduplicates rows while preserving head order", () => {
    expect(mergeSessionRows([row("b"), row("a")], [row("a"), row("old")]).map((session) => session.id))
      .toEqual(["b", "a", "old"]);
  });
});
