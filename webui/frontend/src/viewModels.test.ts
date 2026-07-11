import { describe, expect, it } from "vitest";
import { buildSidebarModel, dedupeInspectNodes, deNoiseSegment, projectDisplayName, projectLabel, projectSubtitle, projectSubtitles, quickSwitchTasks, scheduleLabel, scratchLabel, sessionNeedsAttention, visibleProjectSessions } from "./viewModels";
import type { ProjectGroup } from "./viewModels";
import { compactWorkspaceName, describeApproval } from "./approvalPresentation";
import { conciseTitle, displayTitle, titleFromSessionId } from "./title";
import { foldEvents } from "./timeline";
import type { Session } from "./types";

const sessions: Session[] = [
  { id: "s1", status: "idle", turns: 2, title: "Alpha", workspace: "/Users/me/dev/agentrunner" },
  { id: "s2", status: "running", turns: 1, title: "Beta", workspace: "/Users/me/dev/agentrunner/" },
  { id: "s3", status: "failed", turns: 3, title: "Gamma" },
  { id: "driver", status: "satisfied", turns: 3, title: "Nightly", workspace: "/tmp/repo", kind: "driver" },
  { id: "scratch-1", status: "completed", turns: 1, title: "Scratch one", workspace: "/tmp/ws1783658717524713000" },
  { id: "scratch-2", status: "completed", turns: 1, title: "Scratch two", workspace: "/tmp/wt1783658717524713999" },
];

describe("project sidebar model", () => {
  it("groups by workspace, keeps unknown workspaces, and avoids pinned duplicates", () => {
    const model = buildSidebarModel(sessions, {
      pinned: ["s2"],
      archived: [],
      showArchived: false,
      query: "",
      titleOf: (session) => session.title || session.id,
    });
    expect(model.pinned.map((session) => session.id)).toEqual(["s2"]);
    // Newest-first everywhere (W8): ids sort descending, so s3 leads.
    expect(model.projects.map((project) => project.label)).toEqual(["Scratch", "Other sessions", "agentrunner"]);
    expect(model.projects.flatMap((project) => project.sessions.map((session) => session.id))).toEqual(["scratch-2", "scratch-1", "s3", "s1"]);
    expect(model.projects.flatMap((project) => project.sessions.map((session) => session.id))).not.toContain("driver");
  });

  it("orders sessions newest-first and groups by their newest session (W8)", () => {
    const model = buildSidebarModel(
      [
        { id: "20260701-000000-old-1", status: "idle", turns: 1, workspace: "/w/a" },
        { id: "20260710-090000-new-1", status: "idle", turns: 1, workspace: "/w/b" },
        { id: "20260709-000000-mid-1", status: "idle", turns: 1, workspace: "/w/a" },
      ],
      { pinned: [], archived: [], showArchived: false, query: "", titleOf: (s) => s.id },
    );
    expect(model.projects.map((p) => p.workspace)).toEqual(["/w/b", "/w/a"]);
    expect(model.projects[1].sessions.map((s) => s.id)).toEqual(["20260709-000000-mid-1", "20260701-000000-old-1"]);
  });

  it("disambiguates same-basename groups with a short de-noised parent hint (W4)", () => {
    const model = buildSidebarModel(
      [
        { id: "b", status: "idle", turns: 1, workspace: "/tmp/team/ws" },
        { id: "a", status: "idle", turns: 1, workspace: "/home/me/ws" },
      ],
      { pinned: [], archived: [], showArchived: false, query: "", titleOf: (s) => s.id },
    );
    expect(model.projects.map((p) => p.hint)).toEqual(["team", "me"]);
  });

  it("leaves uniquely-named sidebar groups without a hint (W4)", () => {
    const model = buildSidebarModel(
      [
        { id: "b", status: "idle", turns: 1, workspace: "/x/ws-iso" },
        { id: "a", status: "idle", turns: 1, workspace: "/x/ws-shared" },
      ],
      { pinned: [], archived: [], showArchived: false, query: "", titleOf: (s) => s.id },
    );
    expect(model.projects.map((p) => p.hint)).toEqual([undefined, undefined]);
  });

  it("labels auto-created workspaces as readable scratch names (W2/W42)", () => {
    expect(scratchLabel("ws-20260710-221530")).toBe("Scratch · 07-10 22:15");
    expect(scratchLabel("wt-20260710-221530")).toBe("Scratch · 07-10 22:15");
    expect(scratchLabel("ws1783659626368076000")).toMatch(/^Scratch · \d{2}-\d{2} \d{2}:\d{2}$/);
    expect(scratchLabel("agentrunner")).toBe("");
    expect(projectLabel("/x/y/ws-20260710-221530")).toBe("Scratch");
  });

  it("renders team mail as a peer message, not something you typed (W19)", () => {
    const folded = foldEvents([
      {
        seq: 5,
        type: "input_received",
        ts: "2026-07-10T05:00:00Z",
        payload: { text: "[message from worker (20260710-x-sub-call_6_0-a1)] Hi developer!", source: "tool" },
      },
    ]);
    const bubble = folded.items.find((i) => i.kind === "user") as any;
    expect(bubble.text).toBe("Hi developer!");
    expect(bubble.source).toBe("worker");
    expect(bubble.peerSession).toBe("20260710-x-sub-call_6_0-a1");
    expect(bubble.ts).toBe("2026-07-10T05:00:00Z");
  });

  it("filters archived sessions and searches workspace paths", () => {
    const model = buildSidebarModel(sessions, {
      pinned: [],
      archived: ["s1"],
      showArchived: false,
      query: "agentrunner",
      titleOf: (session) => session.title || session.id,
    });
    expect(model.projects.flatMap((project) => project.sessions.map((session) => session.id))).toEqual(["s2"]);
  });

  it("derives a stable label from a trailing-slash path", () => {
    expect(projectLabel("/tmp/work/repo/")).toBe("repo");
    expect(projectLabel("/tmp/ws1783658717524713000")).toBe("Scratch");
    expect(projectLabel("/tmp/wt1783658717524713999-fork-1234")).toBe("Scratch");
    expect(projectLabel("/tmp/ws-20260710-221530")).toBe("Scratch");
    expect(projectLabel()).toBe("Other sessions");
  });

  it("uses product labels for driver schedules", () => {
    expect(scheduleLabel()).toBe("Goal");
    expect(scheduleLabel("cron")).toBe("Scheduled");
    expect(scheduleLabel("parallel")).toBe("Best of N");
  });
});

describe("project name disambiguation (W4)", () => {
  it("de-noises timestamped parent dirs while keeping rows distinct", () => {
    const subs = projectSubtitles([
      "/x/qa39-20260710-004434/ws",
      "/x/qa39-20260710-004023/ws",
      "/x/qa38-20260710-001743/ws",
    ]);
    expect(subs.get("/x/qa39-20260710-004434/ws")).toBe("qa39-004434");
    expect(subs.get("/x/qa39-20260710-004023/ws")).toBe("qa39-004023");
    expect(subs.get("/x/qa38-20260710-001743/ws")).toBe("qa38-001743");
  });

  it("omits a subtitle for uniquely-named projects", () => {
    const subs = projectSubtitles(["/a/ws-iso", "/b/ws-shared", "/c/repo"]);
    expect(subs.size).toBe(0);
  });

  it("distinguishes two Scratch workspaces by their creation time", () => {
    const subs = projectSubtitles(["/tmp/ws-20260710-221530", "/tmp/ws-20260709-100000"]);
    expect(subs.get("/tmp/ws-20260710-221530")).toBe("07-10 22:15");
    expect(subs.get("/tmp/ws-20260709-100000")).toBe("07-09 10:00");
  });

  it("walks deeper up the path when the nearest parent still collides", () => {
    const subs = projectSubtitles(["/team/alpha/src/ws", "/team/beta/src/ws"]);
    expect(subs.get("/team/alpha/src/ws")).toBe("alpha/src");
    expect(subs.get("/team/beta/src/ws")).toBe("beta/src");
  });

  it("tolerates trailing slashes and shared parents", () => {
    const subs = projectSubtitles(["/root/one/ws/", "/root/two/ws"]);
    expect(subs.get("/root/one/ws")).toBe("one");
    expect(subs.get("/root/two/ws")).toBe("two");
  });

  it("de-noises a bare date segment by keeping the raw token", () => {
    expect(deNoiseSegment("20260710")).toBe("20260710");
    expect(deNoiseSegment("qa39-20260710-004434")).toBe("qa39-004434");
    expect(deNoiseSegment("plain")).toBe("plain");
  });

  it("returns no detail for a lone project (single-item group)", () => {
    expect(projectSubtitle("/only/repo", ["/only/repo"])).toBe("only");
  });
});

describe("command palette quick-switch (W8)", () => {
  const tasks: Session[] = [
    { id: "20260710-090000-a", status: "completed", turns: 1 },
    { id: "20260710-080000-b", status: "waiting:approval", turns: 1 },
    { id: "20260710-070000-c", status: "running", turns: 1 },
    { id: "20260710-060000-d", status: "max_iterations", turns: 1 },
    { id: "20260710-050000-drv", status: "satisfied", turns: 1, kind: "driver" },
    { id: "20260710-040000-arch", status: "completed", turns: 1 },
  ];

  it("classifies which statuses need attention (approval / stranded / limit / crash)", () => {
    expect(sessionNeedsAttention("waiting:approval")).toBe(true);
    expect(sessionNeedsAttention("stranded")).toBe(true);
    expect(sessionNeedsAttention("max_iterations")).toBe(true);
    expect(sessionNeedsAttention("max_generation_steps")).toBe(true);
    expect(sessionNeedsAttention("failed")).toBe(true);
    expect(sessionNeedsAttention("completed")).toBe(false);
    expect(sessionNeedsAttention("satisfied")).toBe(false);
    expect(sessionNeedsAttention("running")).toBe(false);
  });

  it("floats attention tasks to the front so ⌘1/⌘2 land on the ones needing you", () => {
    const order = quickSwitchTasks(tasks).map((s) => s.id);
    // Attention (b=approval, d=limit) lead, newest-first among themselves; the
    // rest follow newest-first. ⌘N = index+1 in this order.
    expect(order).toEqual([
      "20260710-080000-b", // ⌘1
      "20260710-060000-d", // ⌘2
      "20260710-090000-a", // ⌘3
      "20260710-070000-c", // ⌘4
      "20260710-040000-arch", // ⌘5
    ]);
  });

  it("excludes drivers and archived tasks", () => {
    const order = quickSwitchTasks(tasks, { archived: ["20260710-040000-arch"] }).map((s) => s.id);
    expect(order).not.toContain("20260710-050000-drv"); // driver → Scheduled page
    expect(order).not.toContain("20260710-040000-arch"); // archived
  });

  it("caps the quick-switch list at nine ⌘-digit slots, newest-first", () => {
    const many: Session[] = Array.from({ length: 14 }, (_, i) => ({
      id: `20260710-${String(140000 - i).padStart(6, "0")}-t${i}`,
      status: "completed",
      turns: 1,
    }));
    const order = quickSwitchTasks(many);
    expect(order).toHaveLength(9);
    expect(order[0].id).toBe(many[0].id); // newest keeps ⌘1
  });
});

describe("approval presentation", () => {
  it("keeps approval location recognizable without exposing a full temp path", () => {
    expect(compactWorkspaceName("/private/tmp/runtime/scratch/userhome-main/permcheck/")).toBe("permcheck");
    expect(compactWorkspaceName()).toBe("");
  });

  it("summarizes shell commands without exposing raw gate names", () => {
    expect(describeApproval("bash", { command: "go test ./..." })).toMatchObject({
      title: "Run command",
      subject: "go test ./...",
      scope: "Current workspace",
    });
  });

  it("summarizes file and unknown actions", () => {
    expect(describeApproval("edit_file", { path: "src/App.tsx" }).subject).toBe("src/App.tsx");
    expect(describeApproval("custom_tool", {})).toMatchObject({
      title: "Allow action",
      subject: "custom_tool",
      scope: "Current session",
    });
  });
});

describe("task titles", () => {
  it("puts the distinguishing command or reply before repeated boilerplate", () => {
    expect(conciseTitle("Use the bash tool to run exactly: touch concurrent-4.txt.")).toBe("touch concurrent-4.txt");
    expect(conciseTitle("Reply with exactly: STANDING BY")).toBe("Reply · STANDING BY");
    expect(conciseTitle("用 bash 执行这条命令，原样、不加参数：date")).toBe("date");
  });

  it("preserves ordinary titles and keeps manual renames authoritative", () => {
    expect(conciseTitle("Review the authentication boundary")).toBe("Review the authentication boundary");
    expect(displayTitle({ s1: "Release blocker" }, "s1", "Use bash to run exactly: make test")).toBe("Release blocker");
  });

  it("turns metadata-less durable ids into readable fallback titles", () => {
    expect(titleFromSessionId("20260710-053059-review-auth-boundary-6a2b")).toBe("review auth boundary");
    expect(displayTitle({}, "20260710-053059-review-auth-boundary-6a2b")).not.toContain("20260710");
  });

  // INC-52 (HANDA-PARITY #14): the journal-backed auto title arrives as the
  // session's rawTitle (the CLI `title` field). It flows through displayTitle
  // as-is, and a manual rename (localStorage) still overrides it — auto never
  // wins over manual, at the display layer just as in the fold.
  it("shows the journal-backed auto title and still lets a manual rename win", () => {
    expect(displayTitle({}, "s1", "Refactor the auth boundary")).toBe("Refactor the auth boundary");
    expect(displayTitle({ s1: "My name for it" }, "s1", "Refactor the auth boundary")).toBe("My name for it");
  });
});

describe("supervision agent model", () => {
  it("keeps one row per child session and uses the freshest inspect entry", () => {
    expect(
      dedupeInspectNodes([
        { session: "child-1", status: "running" },
        { session: "child-2", status: "completed" },
        { session: "child-1", status: "completed", gen_steps: 7 },
      ]),
    ).toEqual([
      { session: "child-1", status: "completed", gen_steps: 7 },
      { session: "child-2", status: "completed" },
    ]);
  });
});

describe("status and background labels", () => {
  it("translates raw terminal reasons instead of leaking enums (W6)", async () => {
    const { friendlyStatus } = await import("./components/pill");
    expect(friendlyStatus("max_generation_steps")).toEqual({ text: "Step limit reached", cls: "stranded" });
    expect(friendlyStatus("budget_exceeded")).toEqual({ text: "Budget limit reached", cls: "stranded" });
    expect(friendlyStatus("killed")).toEqual({ text: "Stopped by parent", cls: "closed" });
    expect(friendlyStatus("completed").text).toBe("Completed");
  });

  it("renders ps rows as sentences and never a dangling task= (W7)", async () => {
    const { backgroundLabel } = await import("./components/SupervisionPanel");
    expect(backgroundLabel({ handle: "call_6_0", tool: "spawn_agent", detail: "running agent=worker task=" }))
      .toBe("agent “worker” is working in the background");
    expect(backgroundLabel({ handle: "h", tool: "spawn_agent", detail: "running agent=worker task=write hello.py" }))
      .toBe("agent “worker” — write hello.py");
    expect(backgroundLabel({ handle: "h2", tool: "bash", detail: "sleep 60" })).toBe("bash · sleep 60");
  });
});

describe("project overlay (INC-53)", () => {
  const group: ProjectGroup = {
    key: "/repo/app",
    label: "app",
    workspace: "/repo/app",
    sessions: Array.from({ length: 8 }, (_v, i) => ({ id: `s${i}`, status: "idle", turns: 1 })),
  };

  it("uses the overlay display name when set, else the derived label", () => {
    expect(projectDisplayName(group)).toBe("app");
    expect(projectDisplayName(group, {})).toBe("app");
    expect(projectDisplayName(group, { displayName: "  " })).toBe("app"); // blank falls back
    expect(projectDisplayName(group, { displayName: "My App" })).toBe("My App");
  });

  it("folds a group's sessions, caps unfolded groups, and lets search override fold", () => {
    // Unfolded, not expanded: first `cap` only.
    expect(visibleProjectSessions(group, {}).map((s) => s.id)).toEqual(["s0", "s1", "s2", "s3", "s4", "s5"]);
    // Unfolded + expanded: all.
    expect(visibleProjectSessions(group, { expanded: true }).length).toBe(8);
    // Folded: none shown.
    expect(visibleProjectSessions(group, { folded: true })).toEqual([]);
    // Folded but searching: fold is overridden so matches are never hidden.
    expect(visibleProjectSessions(group, { folded: true, searching: true }).length).toBe(8);
    // Custom cap respected.
    expect(visibleProjectSessions(group, { cap: 2 }).map((s) => s.id)).toEqual(["s0", "s1"]);
  });
});
