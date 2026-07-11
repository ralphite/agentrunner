import { describe, expect, it } from "vitest";
import { parseFileDiff, shouldExpandDiffByDefault, splitPath, splitRows, highlightLine, langFromPath, type DiffRow } from "./diffSummary";
import { mixHex } from "./theme";

describe("parseFileDiff (Codex-style review rows, W5)", () => {
  it("numbers context/add/del rows from hunk headers and hides meta lines", () => {
    const { badges, rows } = parseFileDiff([
      "index 000..111 100644",
      "--- a/x.ts",
      "+++ b/x.ts",
      "@@ -10,3 +10,4 @@ function x()",
      " keep",
      "-old",
      "+new",
      "+more",
      " tail",
    ]);
    expect(badges).toEqual([]);
    expect(rows[0]).toMatchObject({ kind: "hunk", text: "function x()" });
    expect(rows[1]).toMatchObject({ kind: "ctx", oldNo: 10, newNo: 10, text: "keep" });
    expect(rows[2]).toMatchObject({ kind: "del", oldNo: 11, text: "old" });
    expect(rows[3]).toMatchObject({ kind: "add", newNo: 11, text: "new" });
    expect(rows[4]).toMatchObject({ kind: "add", newNo: 12, text: "more" });
    expect(rows[5]).toMatchObject({ kind: "ctx", oldNo: 12, newNo: 13, text: "tail" });
    expect(rows.some((r) => r.text.startsWith("+++") || r.text.startsWith("---") || r.text.startsWith("index"))).toBe(false);
  });

  it("distills git metadata into header badges", () => {
    expect(parseFileDiff(["new file mode 100644", "@@ -0,0 +1,1 @@", "+hi"]).badges).toEqual(["new file"]);
    expect(parseFileDiff(["deleted file mode 100644"]).badges).toEqual(["deleted"]);
    expect(parseFileDiff(["similarity index 90%", "rename from a", "rename to b"]).badges).toEqual(["renamed"]);
    expect(parseFileDiff(["Binary files a/x.png and b/x.png differ"]).badges).toEqual(["binary"]);
    expect(parseFileDiff(["old mode 100644", "new mode 100755"]).badges).toEqual(["mode changed"]);
  });

  it("keeps no-newline markers as unnumbered context", () => {
    const { rows } = parseFileDiff(["@@ -1 +1 @@", "-a", "+b", "\\ No newline at end of file"]);
    expect(rows[3]).toMatchObject({ kind: "ctx", text: "\\ No newline at end of file" });
    expect(rows[3].oldNo).toBeUndefined();
  });

  it("splits paths into dim dir + strong base", () => {
    expect(splitPath("docs/DESIGN.md")).toEqual({ dir: "docs/", base: "DESIGN.md" });
    expect(splitPath("README.md")).toEqual({ dir: "", base: "README.md" });
  });
});

describe("highlightLine (INC-41 D3, dependency-free syntax highlight)", () => {
  const join = (t: { t: string }[]) => t.map((x) => x.t).join("");

  it("is total: concatenating tokens rebuilds the input byte-for-byte", () => {
    const samples = [
      ["const x = 1 // hi", "js"],
      ["  return err // wrapped", "go"],
      ['name = "value"  # comment', "py"],
      ["let mut v = vec![]; // rust", "rust"],
      ["## Heading with `code` and **bold**", "md"],
      ["\tif [ -f x ]; then echo hi; fi", "sh"],
      ["", "js"],
      ["plain text no lang", ""],
    ] as const;
    for (const [line, lang] of samples) {
      expect(join(highlightLine(line, lang))).toBe(line);
    }
  });

  it("classifies keywords, strings, comments and call names", () => {
    const toks = highlightLine('const y = fn("s") // note', "js");
    const cls = (word: string) => toks.find((t) => t.t === word)?.c;
    expect(cls("const")).toBe("kw");
    expect(cls("fn")).toBe("fn");
    expect(toks.some((t) => t.c === "str" && t.t === '"s"')).toBe(true);
    expect(toks.some((t) => t.c === "com" && t.t.startsWith("//"))).toBe(true);
  });

  it("only highlights known languages; unknown falls back to one plain token", () => {
    expect(highlightLine("func main()", "")).toEqual([{ t: "func main()" }]);
  });

  it("treats the whole tail after a comment marker as a comment", () => {
    const toks = highlightLine("# just a comment", "py");
    expect(toks).toEqual([{ t: "# just a comment", c: "com" }]);
  });

  it("maps file extensions to a highlighter", () => {
    expect(langFromPath("a/b.ts")).toBe("js");
    expect(langFromPath("main.go")).toBe("go");
    expect(langFromPath("lib.rs")).toBe("rust");
    expect(langFromPath("README.md")).toBe("md");
    expect(langFromPath("data.json")).toBe("json");
    expect(langFromPath("Makefile")).toBe("");
  });
});

describe("splitRows (INC-41 D4, side-by-side pairing)", () => {
  const rows = (): DiffRow[] => [
    { kind: "hunk", text: "ctx" },
    { kind: "ctx", oldNo: 1, newNo: 1, text: "keep" },
    { kind: "del", oldNo: 2, text: "old-a" },
    { kind: "del", oldNo: 3, text: "old-b" },
    { kind: "add", newNo: 2, text: "new-a" },
    { kind: "ctx", oldNo: 4, newNo: 3, text: "tail" },
    { kind: "add", newNo: 4, text: "extra" },
  ];

  it("pairs del/add runs by index and mirrors context on both sides", () => {
    const out = splitRows(rows());
    expect(out[0]).toEqual({ hunk: "ctx" });
    expect(out[1].left?.text).toBe("keep");
    expect(out[1].right?.text).toBe("keep");
    // two dels, one add → row a pairs, row b is delete-only (no right)
    expect(out[2].left?.text).toBe("old-a");
    expect(out[2].right?.text).toBe("new-a");
    expect(out[3].left?.text).toBe("old-b");
    expect(out[3].right).toBeUndefined();
    // trailing add after context → add-only (no left)
    const last = out[out.length - 1];
    expect(last.left).toBeUndefined();
    expect(last.right?.text).toBe("extra");
  });
});

describe("large diff disclosure", () => {
  const diff = (lines: number) => [
    "diff --git a/x.txt b/x.txt",
    "--- a/x.txt",
    "+++ b/x.txt",
    `@@ -0,0 +1,${lines} @@`,
    ...Array.from({ length: lines }, () => "+x"),
  ].join("\n");

  it("opens normal reviews but collapses very large files by default", () => {
    expect(shouldExpandDiffByDefault(diff(20))).toBe(true);
    expect(shouldExpandDiffByDefault(diff(501))).toBe(false);
  });
});

describe("mixHex (INC-41 H2, contrast blending)", () => {
  it("interpolates between two hex colors", () => {
    expect(mixHex("#000000", "#ffffff", 0)).toBe("#000000");
    expect(mixHex("#000000", "#ffffff", 1)).toBe("#ffffff");
    expect(mixHex("#000000", "#ffffff", 0.5)).toBe("#808080");
  });
  it("returns the first color unchanged when either side isn't a hex triple", () => {
    expect(mixHex("var(--dim)", "#ffffff", 0.5)).toBe("var(--dim)");
    expect(mixHex("#123456", "rgb(0,0,0)", 0.5)).toBe("#123456");
  });
});
