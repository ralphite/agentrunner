import { describe, expect, it } from "vitest";
import { buildArchivedModel, buildSidebarModel, daemonVersionLabel, dedupeInspectNodes, deNoiseSegment, projectDisplayName, projectLabel, projectSubtitle, projectSubtitles, quickSwitchSessions, scheduleLabel, scratchLabel, sessionNeedsAttention, visibleProjectSessions } from "./viewModels";
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
  it("groups by workspace, floats workspace-less sessions out, and avoids pinned duplicates", () => {
    const model = buildSidebarModel(sessions, {
      pinned: ["s2"],
      archived: [],
      showArchived: false,
      query: "",
      titleOf: (session) => session.title || session.id,
    });
    expect(model.pinned.map((session) => session.id)).toEqual(["s2"]);
    // Newest-first everywhere (W8): ids sort descending, so the scratch groups
    // lead. INC-78: each auto-created workspace is its own project — two
    // scratch sessions in two directories are two groups, not one pooled
    // "Scratch" folder mixing unrelated work.
    expect(model.projects.map((project) => project.label)).toEqual([
      expect.stringMatching(/^Scratch · \d{2}-\d{2} \d{2}:\d{2}$/),
      expect.stringMatching(/^Scratch · \d{2}-\d{2} \d{2}:\d{2}$/),
      "agentrunner",
    ]);
    // Groups are keyed on the real workspace (renames/folds land per directory).
    expect(model.projects.map((project) => project.key)).toEqual([
      "/tmp/wt1783658717524713999",
      "/tmp/ws1783658717524713000",
      "/Users/me/dev/agentrunner",
    ]);
    expect(model.projects.flatMap((project) => project.sessions.map((session) => session.id))).toEqual(["scratch-2", "scratch-1", "s1"]);
    expect(model.workspaceLessSessions.map((session) => session.id)).toEqual(["s3"]);
    expect(model.projects.flatMap((project) => project.sessions.map((session) => session.id))).not.toContain("driver");
    expect(model.workspaceLessSessions.map((session) => session.id)).not.toContain("driver");
  });

  // SB-13 · A folder icon is an assertion ("these live in a directory"). For a
  // session with no workspace it is a false one, so the model must not be able to
  // make it: workspace-less sessions are never a group, and never a duplicate.
  it("never puts a workspace-less session in a project group, pinned or not", () => {
    const model = buildSidebarModel(
      [
        { id: "n1", status: "idle", turns: 1, title: "No workspace" },
        { id: "n2", status: "idle", turns: 1, title: "Blank workspace", workspace: "   " },
        { id: "n3", status: "idle", turns: 1, title: "Pinned, no workspace" },
        { id: "p1", status: "idle", turns: 1, title: "Real project", workspace: "/repo/app" },
      ],
      { pinned: ["n3"], archived: [], showArchived: false, query: "", titleOf: (session) => session.title || session.id },
    );
    expect(model.projects.map((project) => project.key)).toEqual(["/repo/app"]);
    expect(model.projects.map((project) => project.label)).not.toContain("Other sessions");
    // Whitespace-only workspaces are workspace-less too — a group keyed on "   "
    // would be a folder icon over a path that does not exist.
    expect(model.workspaceLessSessions.map((session) => session.id)).toEqual(["n2", "n1"]);
    // A pinned session lives in exactly one section: Pinned. It must not also show
    // up in Sessions (that was the whole reason the group skipped pinned ids).
    expect(model.pinned.map((session) => session.id)).toEqual(["n3"]);
    expect(model.workspaceLessSessions.map((session) => session.id)).not.toContain("n3");
  });

  it("keeps archived workspace-less sessions reachable in the Archived browser", () => {
    // Settings → Archived only renders groups, so buildArchivedModel folds the
    // flat sessions back into one bucket — SB-13 changes what the *rail* asserts,
    // it does not hide archived sessions from the screen that exists to find them.
    const model = buildArchivedModel(
      [
        { id: "a1", status: "completed", turns: 1, title: "Archived, no workspace" },
        { id: "a2", status: "completed", turns: 1, title: "Archived, in a repo", workspace: "/repo/app" },
      ],
      ["a1", "a2"],
      "",
      (session) => session.title || session.id,
    );
    expect(model.workspaceLessSessions).toEqual([]);
    expect(model.projects.map((project) => project.key)).toEqual(["/repo/app", "__other__"]);
    expect(model.projects[1].sessions.map((session) => session.id)).toEqual(["a1"]);
  });

  it("orders sessions and projects by last update rather than creation id (W8)", () => {
    const model = buildSidebarModel(
      [
        { id: "20260701-000000-old-1", status: "idle", turns: 2, workspace: "/w/a", updatedAt: "2026-07-22T12:00:00Z" },
        { id: "20260710-090000-new-1", status: "idle", turns: 1, workspace: "/w/b", updatedAt: "2026-07-20T12:00:00Z" },
        { id: "20260709-000000-mid-1", status: "idle", turns: 1, workspace: "/w/a", updatedAt: "2026-07-21T12:00:00Z" },
      ],
      { pinned: [], archived: [], showArchived: false, query: "", titleOf: (s) => s.id },
    );
    expect(model.projects.map((p) => p.workspace)).toEqual(["/w/a", "/w/b"]);
    expect(model.projects[0].sessions.map((s) => s.id)).toEqual(["20260701-000000-old-1", "20260709-000000-mid-1"]);
  });

  it("preserves RFC3339 nanosecond update order inside one JavaScript millisecond", () => {
    const model = buildSidebarModel(
      [
        { id: "20260722-120000-newer-id", status: "idle", turns: 1, workspace: "/w/a", updatedAt: "2026-07-22T12:00:00.123100000Z" },
        { id: "20260701-120000-older-id", status: "idle", turns: 2, workspace: "/w/a", updatedAt: "2026-07-22T12:00:00.123900000Z" },
      ],
      { pinned: [], archived: [], showArchived: false, query: "", titleOf: (s) => s.id },
    );
    expect(model.projects[0].sessions.map((session) => session.id)).toEqual([
      "20260701-120000-older-id",
      "20260722-120000-newer-id",
    ]);
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
    expect(projectLabel("/x/y/ws-20260710-221530")).toBe("Scratch · 07-10 22:15");
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
    expect(projectLabel("/tmp/ws1783658717524713000")).toMatch(/^Scratch · /);
    expect(projectLabel("/tmp/wt1783658717524713999-fork-1234")).toMatch(/^Scratch · /);
    expect(projectLabel("/tmp/ws-20260710-221530")).toBe("Scratch · 07-10 22:15");
  });

  it("collapses managed fork chains to the root project name", () => {
    const managed = "/Users/me/.local/share/agentrunner/worktrees/rt1-ws-main-20260712-133500-main-20260712-170909";
    expect(projectLabel(managed)).toBe("rt1-ws");
    // Timestamp-shaped real repository names outside the managed worktree root
    // are user content and remain untouched.
    expect(projectLabel("/repos/rt1-ws-main-20260712-133500")).toBe("rt1-ws-main-20260712-133500");
  });

  // SB-13 · "no workspace" is the absence of a project, not a project named
  // "Other sessions". The empty string is falsy, so the `{hint && …}` and
  // `.filter(Boolean)` guards at the call sites drop it instead of painting a
  // folder that isn't there.
  it("says nothing when there is no workspace", () => {
    expect(projectLabel()).toBe("");
    expect(projectLabel("")).toBe("");
    expect(projectLabel("   ")).toBe("");
    expect(projectLabel("/")).toBe("");
  });

  it("uses product labels for driver schedules", () => {
    expect(scheduleLabel()).toBe("Goal");
    expect(scheduleLabel("cron")).toBe("Scheduled");
    expect(scheduleLabel("parallel")).toBe("Best of N");
  });
});

describe("project name disambiguation (W4)", () => {
  it("distinguishes managed fork generations without exposing the chain", () => {
    const root = "/Users/me/.local/share/agentrunner/worktrees/rt1-ws";
    const first = `${root}-main-20260712-133500`;
    const latest = `${first}-main-20260712-170909`;
    const subs = projectSubtitles([root, first, latest]);
    expect(subs.get(root)).toBe("Root");
    expect(subs.get(first)).toBe("07-12 13:35");
    expect(subs.get(latest)).toBe("07-12 17:09");
  });

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

  it("distinguishes same-minute Scratch twins by seconds, and stays quiet otherwise", () => {
    // INC-78 gave every scratch group a minute-level label of its own, so
    // different-minute groups need no hint at all; only dirs created within
    // the same minute (QA-0719 review #8: twin "Scratch · 07-13 21:23"
    // groups) collide, and their hint carries the seconds that tell them apart.
    const distinct = projectSubtitles(["/tmp/ws-20260710-221530", "/tmp/ws-20260709-100000"]);
    expect(distinct.size).toBe(0);
    const twins = projectSubtitles(["/tmp/ws-20260713-212300", "/tmp/ws-20260713-212347"]);
    expect(twins.get("/tmp/ws-20260713-212300")).toBe("07-13 21:23:00");
    expect(twins.get("/tmp/ws-20260713-212347")).toBe("07-13 21:23:47");
    // A fork dir inherits the source timestamp to the second (QA-0719 #17) —
    // only its fork segment tells the two apart.
    const forks = projectSubtitles(["/w/ws-20260713-212334", "/w/ws-20260713-212334-fork-61de"]);
    expect(forks.get("/w/ws-20260713-212334")).toBe("07-13 21:23:34");
    expect(forks.get("/w/ws-20260713-212334-fork-61de")).toBe("07-13 21:23:34 · fork 61de");
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
  const sessions: Session[] = [
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

  it("floats attention sessions to the front so ⌘1/⌘2 land on the ones needing you", () => {
    const order = quickSwitchSessions(sessions).map((s) => s.id);
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

  it("excludes drivers and archived sessions", () => {
    const order = quickSwitchSessions(sessions, { archived: ["20260710-040000-arch"] }).map((s) => s.id);
    expect(order).not.toContain("20260710-050000-drv"); // driver → Scheduled page
    expect(order).not.toContain("20260710-040000-arch"); // archived
  });

  it("caps the quick-switch list at nine ⌘-digit slots, newest-first", () => {
    const many: Session[] = Array.from({ length: 14 }, (_, i) => ({
      id: `20260710-${String(140000 - i).padStart(6, "0")}-t${i}`,
      status: "completed",
      turns: 1,
    }));
    const order = quickSwitchSessions(many);
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
    expect(describeApproval("spawn_agent", { prompt: "Review the auth boundary" }).subject).toBe("Review the auth boundary");
    expect(describeApproval("custom_tool", {})).toMatchObject({
      title: "Allow action",
      subject: "custom_tool",
      scope: "Current session",
    });
  });
});

describe("session titles", () => {
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
    expect(titleFromSessionId("20260710-053059-review-auth-boundary-0123456789abcdef")).toBe("review auth boundary");
    expect(titleFromSessionId("20260713-070914-run-the-three-worker-qa-delega-3dcf-sub-call_1_2-a1")).toBe("Sub-agent · call 1 2");
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

  it("renders ps rows as sentences and never a dangling prompt= (W7)", async () => {
    const { backgroundLabel } = await import("./components/SupervisionPanel");
    expect(backgroundLabel({ handle: "call_6_0", tool: "spawn_agent", detail: "running agent=worker prompt=" }))
      .toBe("agent “worker” is working in the background");
    expect(backgroundLabel({ handle: "h", tool: "spawn_agent", detail: "running agent=worker prompt=write hello.py" }))
      .toBe("agent “worker” — write hello.py");
    expect(backgroundLabel({ handle: "h2", tool: "bash", detail: "sleep 60" })).toBe("bash · sleep 60");
  }, 15_000);
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

  it("keeps the current session past the cap but respects an explicit fold (INC-90)", () => {
    // The 7th session is beyond cap=6 — it still has to appear, or the sidebar
    // shows no trace of the session the user is actually looking at.
    expect(visibleProjectSessions(group, { current: "s6" }).map((s) => s.id))
      .toEqual(["s0", "s1", "s2", "s3", "s4", "s5", "s6"]);
    // A current session already inside the cap window is not duplicated.
    expect(visibleProjectSessions(group, { current: "s1" }).map((s) => s.id))
      .toEqual(["s0", "s1", "s2", "s3", "s4", "s5"]);
    // A manual fold wins even when this group owns the current session.
    const foldedShown = visibleProjectSessions(group, { folded: true, current: "s7" });
    expect(foldedShown).toEqual([]);
    // A folded group without it also stays collapsed.
    expect(visibleProjectSessions(group, { folded: true, current: "elsewhere" })).toEqual([]);
    // A current session from another project never leaks into this group.
    expect(visibleProjectSessions(group, { current: "elsewhere" }).map((s) => s.id))
      .toEqual(["s0", "s1", "s2", "s3", "s4", "s5"]);
  });
});

describe("daemon connection label", () => {
  it("never leaks an unknown build stamp into the product footer", () => {
    expect(daemonVersionLabel("unknown")).toBe("local");
    expect(daemonVersionLabel("")).toBe("local");
    expect(daemonVersionLabel("agentrunner dev build")).toBe("dev");
    expect(daemonVersionLabel("0a38b5a")).toBe("0a38b5a");
  });
});
