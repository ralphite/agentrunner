import { describe, expect, it } from "vitest";
import { buildSidebarModel, scheduledUnread } from "./viewModels";
import type { Session } from "./types";

const opts = (over: Partial<Parameters<typeof buildSidebarModel>[1]>) => ({
  pinned: [] as string[],
  archived: [] as string[],
  showArchived: false,
  query: "",
  titleOf: (s: Session) => s.title || s.id,
  ...over,
});

describe("sidebar Pinned group (E1)", () => {
  const sessions: Session[] = [
    { id: "a", status: "idle", turns: 1, title: "Alpha", workspace: "/w/one" },
    { id: "b", status: "idle", turns: 1, title: "Beta", workspace: "/w/two" },
    { id: "c", status: "idle", turns: 1, title: "Gamma", workspace: "/w/one" },
  ];

  it("flattens pinned tasks into one group, drawn from across projects", () => {
    const model = buildSidebarModel(sessions, opts({ pinned: ["a", "b"] }));
    // Both pinned tasks appear in the flat Pinned list, in pin order.
    expect(model.pinned.map((s) => s.id)).toEqual(["a", "b"]);
    // ...and are lifted out of their project groups (no duplicates below).
    const inProjects = model.projects.flatMap((p) => p.sessions.map((s) => s.id));
    expect(inProjects).not.toContain("a");
    expect(inProjects).not.toContain("b");
    expect(inProjects).toContain("c");
  });

  it("returns an unpinned task to its original project group", () => {
    // Pin "c" (project /w/one) then unpin: it falls back under /w/one with "a".
    const pinnedModel = buildSidebarModel(sessions, opts({ pinned: ["c"] }));
    expect(pinnedModel.pinned.map((s) => s.id)).toEqual(["c"]);
    const unpinnedModel = buildSidebarModel(sessions, opts({ pinned: [] }));
    expect(unpinnedModel.pinned).toEqual([]);
    const one = unpinnedModel.projects.find((p) => p.workspace === "/w/one");
    expect(one?.sessions.map((s) => s.id).sort()).toEqual(["a", "c"]);
  });

  it("leaves the Pinned group empty when nothing is pinned", () => {
    expect(buildSidebarModel(sessions, opts({})).pinned).toEqual([]);
  });

  it("ignores pins that point at archived-and-hidden or missing sessions", () => {
    const model = buildSidebarModel(sessions, opts({ pinned: ["a", "ghost"], archived: ["a"] }));
    // "a" is archived and hidden, "ghost" doesn't exist → neither shows.
    expect(model.pinned).toEqual([]);
  });
});

describe("scheduled unread (E3 / F2)", () => {
  const sessions: Session[] = [
    { id: "d1", status: "running", turns: 1, kind: "driver" },
    { id: "d2", status: "satisfied", turns: 1, kind: "driver" },
    { id: "t1", status: "idle", turns: 1 },
  ];

  it("counts only driver sessions with unread activity", () => {
    expect(scheduledUnread(sessions, ["d1", "t1"])).toEqual(["d1"]);
  });

  it("ignores non-driver tasks even when they are unread", () => {
    expect(scheduledUnread(sessions, ["t1"])).toEqual([]);
  });

  it("is empty when nothing is unread", () => {
    expect(scheduledUnread(sessions, [])).toEqual([]);
  });
});
