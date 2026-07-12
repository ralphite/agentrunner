import { describe, expect, it } from "vitest";
import {
  parseFileDiff,
  defaultOpenByPath,
  shouldExpandFileByDefault,
  hunkGaps,
  trailingGapKey,
  splitPath,
  splitRows,
  highlightLine,
  langFromPath,
  type DiffRow,
} from "./diffSummary";
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

  it("highlights markup — html/xml/svg were unmapped, so index.html rendered black (RD-3)", () => {
    expect(langFromPath("dist/index.html")).toBe("html");
    expect(langFromPath("page.htm")).toBe("html");
    expect(langFromPath("feed.xml")).toBe("html");
    expect(langFromPath("icon.svg")).toBe("html");

    const line = '<script src="/assets/index-a1b2.js"></script>';
    const toks = highlightLine(line, "html");
    expect(join(toks)).toBe(line); // still byte-exact
    expect(toks.find((t) => t.t === "script")?.c).toBe("kw");
    expect(toks.some((t) => t.c === "str" && t.t === '"/assets/index-a1b2.js"')).toBe(true);
    // a `//` inside a URL must not swallow the rest of the line as a comment
    const url = '<a href="https://x.dev/p">go</a>';
    expect(join(highlightLine(url, "html"))).toBe(url);
    expect(highlightLine(url, "html").some((t) => t.c === "com")).toBe(false);
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

describe("per-file diff disclosure (INC-41 RD-1)", () => {
  it("collapses only the file that is too big, never its small neighbours", () => {
    expect(shouldExpandFileByDefault({ add: 20, del: 0 })).toBe(true);
    expect(shouldExpandFileByDefault({ add: 500, del: 0 })).toBe(true);
    expect(shouldExpandFileByDefault({ add: 400, del: 101 })).toBe(false);
  });

  it("regression: a 1,284-line package-lock.json no longer folds package.json/server.js", () => {
    const open = defaultOpenByPath([
      { path: "package-lock.json", add: 1284, del: 0 },
      { path: "package.json", add: 12, del: 0 },
      { path: "server.js", add: 110, del: 0 },
    ]);
    expect(open.get("package-lock.json")).toBe(false);
    expect(open.get("package.json")).toBe(true);
    expect(open.get("server.js")).toBe(true);
  });

  it("spends a first-paint budget across a huge review instead of folding it whole", () => {
    const files = Array.from({ length: 40 }, (_, i) => ({ path: `f${i}.ts`, add: 200, del: 0 }));
    const open = defaultOpenByPath(files);
    // 5000-line budget → the first files open with code; the tail waits for a click.
    expect(open.get("f0.ts")).toBe(true);
    expect(open.get("f24.ts")).toBe(true);
    expect(open.get("f25.ts")).toBe(false);
    expect(open.get("f39.ts")).toBe(false);
  });
});

describe("hunkGaps (INC-41 RD-2, trailing unmodified band)", () => {
  // one hunk starting at new line 10 → 9 hidden lines before it, and an unknown
  // run after it (a unified diff never states the file's total length).
  const rows = (): DiffRow[] => [
    { kind: "hunk", text: "" },
    { kind: "ctx", oldNo: 10, newNo: 10, text: "keep" },
    { kind: "del", oldNo: 11, text: "old" },
    { kind: "add", newNo: 11, text: "new" },
    { kind: "ctx", oldNo: 12, newNo: 12, text: "tail" },
  ];

  it("emits a to-EOF gap keyed past the last row, with an unknown (null) end", () => {
    const gaps = hunkGaps(rows(), { trailing: true });
    const r = rows();
    expect(gaps.get(0)).toEqual({ start: 1, end: 9 }); // leading gap, exact
    expect(gaps.get(trailingGapKey(r))).toEqual({ start: 13, end: null });
    expect(trailingGapKey(r)).toBe(r.length); // never collides with a hunk row index
  });

  it("omits the trailing gap when not asked for it (added/deleted files)", () => {
    const gaps = hunkGaps(rows());
    expect(gaps.has(trailingGapKey(rows()))).toBe(false);
    expect(gaps.get(0)).toEqual({ start: 1, end: 9 }); // interior/leading behaviour unchanged
  });

  it("omits the trailing gap when the diff proves it already reached EOF", () => {
    const atEOF: DiffRow[] = [...rows(), { kind: "ctx", text: "\\ No newline at end of file" }];
    expect(hunkGaps(atEOF, { trailing: true }).has(trailingGapKey(atEOF))).toBe(false);
  });

  it("still keys interior gaps by their hunk row and returns exact lengths", () => {
    const two: DiffRow[] = [
      { kind: "hunk", text: "" },
      { kind: "ctx", oldNo: 1, newNo: 1, text: "a" },
      { kind: "hunk", text: "" },
      { kind: "ctx", oldNo: 20, newNo: 20, text: "b" },
    ];
    const gaps = hunkGaps(two, { trailing: true });
    expect(gaps.has(0)).toBe(false); // first hunk starts at line 1 — nothing hidden
    expect(gaps.get(2)).toEqual({ start: 2, end: 19 });
    expect(gaps.get(trailingGapKey(two))).toEqual({ start: 21, end: null });
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
