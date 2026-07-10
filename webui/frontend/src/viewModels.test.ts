import { describe, expect, it } from "vitest";
import { describeApproval } from "./approvalPresentation";
import { buildSidebarModel, dedupeInspectNodes, projectLabel } from "./viewModels";
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
    expect(model.projects.map((project) => project.label)).toEqual(["agentrunner", "Other sessions"]);
    expect(model.projects.flatMap((project) => project.sessions.map((session) => session.id))).toEqual(["s1", "s3"]);
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
