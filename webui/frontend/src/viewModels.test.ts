import { describe, expect, it } from "vitest";
import { describeApproval } from "./approvalPresentation";
import { buildSidebarModel, dedupeInspectNodes, projectLabel, scratchLabel } from "./viewModels";
import { foldEvents } from "./timeline";
import type { Session } from "./types";

const sessions: Session[] = [
  { id: "s1", status: "idle", turns: 2, title: "Alpha", workspace: "/Users/me/dev/agentrunner" },
  { id: "s2", status: "running", turns: 1, title: "Beta", workspace: "/Users/me/dev/agentrunner/" },
  { id: "s3", status: "failed", turns: 3, title: "Gamma" },
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
    expect(model.projects.map((project) => project.label)).toEqual(["Other sessions", "agentrunner"]);
    expect(model.projects.flatMap((project) => project.sessions.map((session) => session.id))).toEqual(["s3", "s1"]);
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

  it("disambiguates same-basename groups with a parent hint (W20)", () => {
    const model = buildSidebarModel(
      [
        { id: "b", status: "idle", turns: 1, workspace: "/tmp/team/ws" },
        { id: "a", status: "idle", turns: 1, workspace: "/home/me/ws" },
      ],
      { pinned: [], archived: [], showArchived: false, query: "", titleOf: (s) => s.id },
    );
    expect(model.projects.map((p) => p.hint)).toEqual(["…/team", "…/me"]);
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
    expect(projectLabel()).toBe("Other sessions");
  });
});

describe("approval presentation", () => {
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
    expect(friendlyStatus("max_generation_steps")).toEqual({ text: "stopped: step limit", cls: "stranded" });
    expect(friendlyStatus("budget_exceeded")).toEqual({ text: "stopped: budget limit", cls: "stranded" });
    expect(friendlyStatus("killed")).toEqual({ text: "stopped by parent", cls: "closed" });
    expect(friendlyStatus("completed").text).toBe("completed");
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
