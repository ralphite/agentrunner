import { describe, expect, it } from "vitest";
import {
  askUserDetail,
  editDetail,
  foldEvents,
  globDetail,
  grepDetail,
  groupIcon,
  lineDiff,
  parseMaybeJSON,
  readDetail,
  semanticDetail,
  spawnDetail,
  toolCategory,
  webFetchDetail,
  type BubbleItem,
  type ChipItem,
  type ToolItem,
} from "./timeline";

const tool = (name: string, args: any = {}, result?: any): ToolItem => ({
  kind: "tool",
  key: "act" + name,
  name,
  args,
  background: false,
  status: "done",
  statusText: "done",
  result,
});

describe("A1 — activity categories (groupLabel → icon)", () => {
  it("maps each tool family to a category", () => {
    expect(toolCategory("bash")).toBe("bash");
    expect(toolCategory("read_file")).toBe("read");
    expect(toolCategory("edit_file")).toBe("edit");
    expect(toolCategory("write_file")).toBe("edit");
    expect(toolCategory("grep")).toBe("search");
    expect(toolCategory("glob")).toBe("search");
    expect(toolCategory("semantic_search")).toBe("search");
    expect(toolCategory("web_fetch")).toBe("web");
    expect(toolCategory("spawn_agent")).toBe("spawn");
    expect(toolCategory("send_message")).toBe("message");
    expect(toolCategory("ask_user")).toBe("ask");
    expect(toolCategory("goal_status")).toBe("progress");
    expect(toolCategory("mcp__whatever__thing")).toBe("other");
  });

  it("picks the group icon from the first tool (first-appearance order)", () => {
    expect(groupIcon([tool("edit_file"), tool("read_file"), tool("bash")])).toBe("edit");
    expect(groupIcon([tool("bash"), tool("bash")])).toBe("bash");
    expect(groupIcon([])).toBe("other");
  });
});

describe("A2 — lineDiff", () => {
  it("trims common prefix/suffix and marks the changed middle", () => {
    const old = "// keep\nfunc Add(a, b int) int {\n\treturn a * b\n}";
    const neu = "// keep\nfunc Add(a, b int) int {\n\treturn a + b\n}";
    expect(lineDiff(old, neu)).toEqual([
      { kind: "ctx", text: "// keep" },
      { kind: "ctx", text: "func Add(a, b int) int {" },
      { kind: "del", text: "\treturn a * b" },
      { kind: "add", text: "\treturn a + b" },
      { kind: "ctx", text: "}" },
    ]);
  });

  it("renders a pure insertion with no del rows", () => {
    expect(lineDiff("a\nb", "a\nb\nc")).toEqual([
      { kind: "ctx", text: "a" },
      { kind: "ctx", text: "b" },
      { kind: "add", text: "c" },
    ]);
  });
});

describe("A2 — tool detail extractors", () => {
  it("read_file: path + line count from content", () => {
    expect(readDetail({ path: "a/b.go" }, { content: "l1\nl2\nl3\n", truncated: false })).toEqual({
      path: "a/b.go",
      range: undefined,
      lineCount: 3,
      truncated: false,
    });
  });

  it("edit_file: mini diff rows + result note", () => {
    const d = editDetail(
      "edit_file",
      { path: "calc.go", old: "return a * b", new: "return a + b" },
      { output: "edited calc.go" },
    );
    expect(d.path).toBe("calc.go");
    expect(d.note).toBe("edited calc.go");
    expect(d.rows).toEqual([
      { kind: "del", text: "return a * b" },
      { kind: "add", text: "return a + b" },
    ]);
  });

  it("write_file: all-add rows, trailing newline dropped", () => {
    const d = editDetail("write_file", { path: "x.txt", content: "QA42_WORKTREE_OK\n" }, { output: "wrote x.txt (17 bytes)" });
    expect(d.rows).toEqual([{ kind: "add", text: "QA42_WORKTREE_OK" }]);
    expect(d.note).toBe("wrote x.txt (17 bytes)");
  });

  it("grep: match/file counts + grouping by file", () => {
    const d = grepDetail(
      { pattern: "verifiers" },
      {
        files_scanned: 31,
        truncated: false,
        matches: [
          { path: "a.yaml", line: 2, text: "verifiers:" },
          { path: "a.yaml", line: 9, text: "  verifiers x" },
          { path: "b.yaml", line: 6, text: "verifiers:" },
        ],
      },
    );
    expect(d.matchCount).toBe(3);
    expect(d.fileCount).toBe(2);
    expect(d.scanned).toBe(31);
    expect(d.byFile.map((f) => f.path)).toEqual(["a.yaml", "b.yaml"]);
    expect(d.byFile[0].hits).toHaveLength(2);
  });

  it("glob: pattern + paths", () => {
    expect(globDetail({ pattern: "*" }, { paths: ["go.mod", "main.go"], truncated: false })).toEqual({
      pattern: "*",
      paths: ["go.mod", "main.go"],
      truncated: false,
    });
  });

  it("semantic_search: query + hit paths", () => {
    const d = semanticDetail(
      { query: "write file" },
      { hits: [{ path: "a.yaml", line: 1, score: 3.9 }, { path: "b.yaml", line: 1 }] },
    );
    expect(d.query).toBe("write file");
    expect(d.hits).toEqual([{ path: "a.yaml", line: 1 }, { path: "b.yaml", line: 1 }]);
  });

  it("spawn_agent: agent + task + child session link", () => {
    const d = spawnDetail(
      { agent: "worker", task: "map the routes" },
      { agent: "worker", child_session: "sub-1", reason: "completed", report: "done" },
    );
    expect(d).toMatchObject({ agent: "worker", task: "map the routes", childSession: "sub-1", reason: "completed" });
  });

  it("web_fetch: url + bytes, untrusted defaults true", () => {
    expect(webFetchDetail({ url: "https://x.dev" }, { title: "X", content: "abcd" })).toEqual({
      url: "https://x.dev",
      title: "X",
      bytes: 4,
      untrusted: true,
    });
    expect(webFetchDetail({ url: "https://x.dev" }, { untrusted: false }).untrusted).toBe(false);
  });

  it("ask_user: question text from any field", () => {
    expect(askUserDetail({ question: "which one?" }).question).toBe("which one?");
    expect(askUserDetail({ prompt: "pick" }).question).toBe("pick");
  });

  it("parseMaybeJSON tolerates strings and objects", () => {
    expect(parseMaybeJSON('{"a":1}')).toEqual({ a: 1 });
    expect(parseMaybeJSON({ a: 1 })).toEqual({ a: 1 });
    expect(parseMaybeJSON("not json")).toBe("not json");
  });
});

describe("A5 — Sent as goal note", () => {
  it("marks the user message whose text is the goal, and drops the chip", () => {
    const { items } = foldEvents([
      { seq: 1, type: "input_received", payload: { source: "cli", text: "Create goal-r2.txt with DONE" } },
      { seq: 2, type: "goal_attached", payload: { goal: "Create goal-r2.txt with DONE" } },
    ]);
    const user = items.find((i) => i.kind === "user") as BubbleItem;
    expect(user.sentAsGoal).toBe(true);
    expect(items.some((i) => i.kind === "chip" && /goal attached/.test((i as ChipItem).text))).toBe(false);
  });

  it("keeps the chip when no user message matches the goal", () => {
    const { items } = foldEvents([
      { seq: 1, type: "input_received", payload: { source: "cli", text: "hi there" } },
      { seq: 2, type: "goal_attached", payload: { goal: "some unrelated goal set via cli" } },
    ]);
    const user = items.find((i) => i.kind === "user") as BubbleItem;
    expect(user.sentAsGoal).toBeUndefined();
    expect(items.some((i) => i.kind === "chip" && /goal attached/.test((i as ChipItem).text))).toBe(true);
  });
});

describe("A3 — compaction as an activity row", () => {
  it("renders context_compacted as a fold-able activity chip", () => {
    const { items } = foldEvents([{ seq: 4, type: "context_compacted", payload: { upto_gen_step: 3 } }]);
    const chip = items.find((i) => i.kind === "chip") as ChipItem;
    expect(chip).toMatchObject({ text: "Context automatically compacted", fold: true, activity: true });
  });
});
