import { describe, expect, it } from "vitest";
import { buildArchivedModel, buildSidebarModel, quickSwitchTasks, scheduledUnread } from "./viewModels";
import { PROJECT_GROUP_LIMIT, paletteTaskGroups, visibleProjectGroups } from "./viewModels.nav";
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

describe("command palette task groups (RH-3)", () => {
  // Ids sort as creation stamps: t12 > t11 > … > t01 lexicographically here
  // because they are zero-padded.
  const task = (id: string, status: string): Session => ({ id, status, turns: 1, title: `Task ${id}` });
  // 12 tasks all waiting on approval — the exact shape that made the old
  // palette show zero badges and no Tasks group at all.
  const allAttention: Session[] = Array.from({ length: 12 }, (_, i) =>
    task(`t${String(12 - i).padStart(2, "0")}`, "waiting_approval"),
  );

  it("badges all nine quick-switch rows even when every one needs attention", () => {
    const { quick, unread } = paletteTaskGroups(allAttention);
    expect(quick.map((s) => s.id)).toEqual(["t12", "t11", "t10", "t09", "t08", "t07", "t06", "t05", "t04"]);
    // The three that fell past the ninth digit become the Unread tasks group.
    expect(unread.map((s) => s.id)).toEqual(["t03", "t02", "t01"]);
  });

  it("keeps the badge honest: quick[i] is exactly what ⌘(i+1) opens", () => {
    // App.tsx's cmd-digit handler indexes quickSwitchTasks; the palette badges
    // paletteTaskGroups().quick. If these ever diverge, a badge lies.
    const mixed: Session[] = [
      task("t05", "idle"),
      task("t04", "waiting_approval"),
      task("t03", "running"),
      task("t02", "crashed"),
      task("t01", "completed"),
    ];
    expect(paletteTaskGroups(mixed).quick.map((s) => s.id)).toEqual(
      quickSwitchTasks(mixed).map((s) => s.id),
    );
  });

  it("leaves Unread tasks empty when nothing overflows the nine digits", () => {
    const calm: Session[] = [task("t02", "idle"), task("t01", "completed")];
    const { quick, unread } = paletteTaskGroups(calm);
    expect(quick.map((s) => s.id)).toEqual(["t02", "t01"]);
    expect(unread).toEqual([]);
  });

  it("drops drivers and archived tasks from both groups", () => {
    const sessions: Session[] = [
      ...allAttention,
      { id: "t99", status: "waiting_approval", turns: 1, kind: "driver" },
    ];
    const { quick, unread } = paletteTaskGroups(sessions, { archived: ["t12", "t03"] });
    // Archiving the head of the list pulls one row up out of the overflow…
    expect(quick.map((s) => s.id)).toEqual(["t11", "t10", "t09", "t08", "t07", "t06", "t05", "t04", "t02"]);
    // …and neither the archived rows nor the driver appear anywhere.
    expect(unread.map((s) => s.id)).toEqual(["t01"]);
    expect([...quick, ...unread].map((s) => s.id)).not.toContain("t99");
    expect([...quick, ...unread].map((s) => s.id)).not.toContain("t03");
  });

  it("never puts a task in both groups", () => {
    const { quick, unread } = paletteTaskGroups(allAttention);
    const ids = new Set(quick.map((s) => s.id));
    expect(unread.some((s) => ids.has(s.id))).toBe(false);
  });
});

describe("archived settings model (J4)", () => {
  const sessions: Session[] = [
    { id: "a", status: "completed", turns: 1, title: "Alpha task", workspace: "/repo/one" },
    { id: "b", status: "completed", turns: 1, title: "Beta task", workspace: "/repo/two" },
    { id: "c", status: "completed", turns: 1, title: "Gamma task", workspace: "/repo/one" },
  ];

  it("contains only archived tasks and keeps project grouping/search", () => {
    const model = buildArchivedModel(sessions, ["a", "b"], "alpha", (session) => session.title || session.id);
    expect(model.pinned).toEqual([]);
    expect(model.projects).toHaveLength(1);
    expect(model.projects[0].sessions.map((session) => session.id)).toEqual(["a"]);
  });
});

describe("visibleProjectGroups (SB-4)", () => {
  const groups = (n: number) =>
    Array.from({ length: n }, (_v, i) => ({
      key: `/repo/p${i}`,
      label: `p${i}`,
      workspace: `/repo/p${i}`,
      sessions: [{ id: `s${i}`, status: "idle", turns: 1 } as Session],
    }));

  it("renders every group when the list is already short", () => {
    const all = groups(PROJECT_GROUP_LIMIT);
    const { groups: shown, hidden } = visibleProjectGroups(all);
    expect(shown).toHaveLength(PROJECT_GROUP_LIMIT);
    expect(hidden).toBe(0);
  });

  it("truncates to the limit and reports the remainder", () => {
    const { groups: shown, hidden } = visibleProjectGroups(groups(127));
    expect(shown).toHaveLength(8);
    expect(hidden).toBe(119);
    // Newest-first order is preserved — the cut is a tail cut, not a reshuffle.
    expect(shown.map((g) => g.key)).toEqual(groups(8).map((g) => g.key));
  });

  it("expanded shows everything with nothing hidden", () => {
    const { groups: shown, hidden } = visibleProjectGroups(groups(127), { expanded: true });
    expect(shown).toHaveLength(127);
    expect(hidden).toBe(0);
  });

  it("always renders the group holding the current task, even past the limit", () => {
    const { groups: shown, hidden } = visibleProjectGroups(groups(127), { current: "s40" });
    expect(shown).toHaveLength(9);
    // Appended at the tail: the first 8 rows never shuffle under the user.
    expect(shown[8].key).toBe("/repo/p40");
    expect(hidden).toBe(118);
  });

  it("does not duplicate the current group when it is already inside the limit", () => {
    const { groups: shown, hidden } = visibleProjectGroups(groups(127), { current: "s2" });
    expect(shown).toHaveLength(8);
    expect(shown.filter((g) => g.key === "/repo/p2")).toHaveLength(1);
    expect(hidden).toBe(119);
  });
});
