import { describe, expect, it } from "vitest";
import { SCHEDULED_TITLE_MAX, scheduledTitle } from "./scheduledTitle";

describe("scheduledTitle (SC-13)", () => {
  it("keeps a name that is already a name", () => {
    expect(scheduledTitle("Weekly status update draft")).toBe("Weekly status update draft");
    expect(scheduledTitle("cloc")).toBe("cloc");
  });

  it("drops the tooling parenthetical the prompt ends with", () => {
    expect(
      scheduledTitle("Append a line to notes.md (use write_file or bash)"),
    ).toBe("Append a line to notes.md");
    // …and the same with the full stop outside the bracket.
    expect(
      scheduledTitle("Append a line to notes.md (use write_file or bash)."),
    ).toBe("Append a line to notes.md");
  });

  it("takes the first sentence — the rest of a prompt is elaboration", () => {
    // The live row: three rows all began "Append one line with the current…".
    expect(
      scheduledTitle(
        "Update the changelog. Then commit it and push to origin so the release notes stay current.",
      ),
    ).toBe("Update the changelog");
  });

  it("never breaks a sentence inside a filename or a version", () => {
    expect(scheduledTitle("Rewrite notes.md every hour")).toBe("Rewrite notes.md every hour");
    expect(scheduledTitle("Bump to v1.2 in the manifest")).toBe("Bump to v1.2 in the manifest");
  });

  it("does not cut at an abbreviation stub", () => {
    // "e.g." is a sentence end by punctuation and nothing else; cutting there
    // would title the row "e.g".
    expect(scheduledTitle("e.g. run the linter over the repo")).toBe("e.g. run the linter over the repo");
  });

  it("caps a long clause at 48 chars, on a word boundary", () => {
    const long =
      "Append one line with the current timestamp to the running notes file in the workspace";
    const out = scheduledTitle(long);
    expect(out.length).toBeLessThanOrEqual(SCHEDULED_TITLE_MAX + 1); // + the ellipsis
    expect(out.endsWith("…")).toBe(true);
    // The cut lands between words — never mid-word.
    expect(long.startsWith(out.slice(0, -1))).toBe(true);
    expect(long[out.length - 1]).toBe(" ");
    expect(out).toBe("Append one line with the current timestamp to…");
  });

  it("hard-cuts CJK, which has no word boundaries to find", () => {
    const cjk = "每小时把当前时间戳追加到笔记文件里并提交到仓库，然后把结果同步给团队，最后再检查一次构建是否仍然是绿的";
    const out = scheduledTitle(cjk);
    expect(out.endsWith("…")).toBe(true);
    expect(out.length).toBeLessThanOrEqual(SCHEDULED_TITLE_MAX + 1);
    expect(cjk.startsWith(out.slice(0, 6))).toBe(true);
  });

  it("splits CJK on its own full stop, which carries no trailing space", () => {
    expect(scheduledTitle("更新每周的状态简报。然后把它发到群里。")).toBe("更新每周的状态简报");
  });

  it("collapses the whitespace a pasted prompt drags in", () => {
    expect(scheduledTitle("  Watch   the\n  build  ")).toBe("Watch the build");
  });

  it("falls back to the id when there is no label at all", () => {
    expect(scheduledTitle("", "20260712-033455-cx3")).toBe("20260712-033455-cx3");
    expect(scheduledTitle(undefined, "r-loop")).toBe("r-loop");
    expect(scheduledTitle("   ", "r-loop")).toBe("r-loop");
  });

  it("never returns an empty title for a prompt that is pure punctuation", () => {
    expect(scheduledTitle("???", "r-x")).toBe("???");
  });
});
