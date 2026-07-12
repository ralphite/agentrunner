import { describe, expect, it } from "vitest";
import {
  parseFileDiff,
  defaultOpenByPath,
  shouldExpandFileByDefault,
  isGeneratedPath,
  longestContentLine,
  MAX_INLINE_LINE_WIDTH,
  hunkGaps,
  trailingGapKey,
  splitDiff,
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

describe("generated / minified files stay folded (INC-41 DF-2)", () => {
  // Field case: session 20260711-011831 deleted a built bundle. 176 lines — under
  // MAX_INLINE_FILE_LINES — so the old rule expanded it and buried the two lines
  // of index.html the reviewer came for under 4,000px of minified React.
  const minified = (n: number) =>
    Array.from({ length: n }, () => "-" + "!function(e,t){...}(this,function(){return 42});".repeat(40));

  it("folds a deleted dist bundle even though it is short", () => {
    const open = defaultOpenByPath([
      { path: "webui/frontend/dist/assets/index-BLupS6ef.js", add: 0, del: 176, lines: minified(176) },
      { path: "webui/frontend/dist/index.html", add: 2, del: 2, lines: ["+<script src=a>", "-<script src=b>"] },
      { path: "docs/DESIGN.md", add: 8, del: 4, lines: ["+prose", "-prose"] },
    ]);
    expect(open.get("webui/frontend/dist/assets/index-BLupS6ef.js")).toBe(false);
    // a 4-line asset-hash bump is what the reviewer came for — it still opens
    expect(open.get("webui/frontend/dist/index.html")).toBe(true);
    expect(open.get("docs/DESIGN.md")).toBe(true);
  });

  it("folds a generated file that is merely long, even without minified lines", () => {
    const lines = Array.from({ length: 60 }, (_, i) => `+  "line ${i}": "narrow but generated"`);
    expect(shouldExpandFileByDefault({ path: "webui/frontend/dist/manifest.json", add: 60, del: 0, lines })).toBe(false);
  });

  it("still expands an ordinary 176-line source file (no width regression)", () => {
    const lines = Array.from({ length: 176 }, (_, i) => `+  const value${i} = compute(i); // a perfectly normal line of code`);
    expect(shouldExpandFileByDefault({ path: "src/components/DiffView.tsx", add: 176, del: 0, lines })).toBe(true);
  });

  it("keeps folding package-lock.json (unchanged verdict)", () => {
    expect(shouldExpandFileByDefault({ path: "package-lock.json", add: 1284, del: 0 })).toBe(false);
    // a 200-line lockfile churn is noise too — generated files get a 20-line budget
    expect(shouldExpandFileByDefault({ path: "package-lock.json", add: 200, del: 0 })).toBe(false);
  });

  it("folds a short file whose single line is minified-wide", () => {
    const oneLongLine = ["+" + "a".repeat(MAX_INLINE_LINE_WIDTH + 1)];
    expect(shouldExpandFileByDefault({ path: "src/generated/schema.ts", add: 1, del: 0, lines: oneLongLine })).toBe(false);
    // exactly at the threshold is still readable-ish → expand
    expect(shouldExpandFileByDefault({ path: "src/generated/schema.ts", add: 1, del: 0, lines: ["+" + "a".repeat(MAX_INLINE_LINE_WIDTH)] })).toBe(true);
  });

  it("isGeneratedPath: build output, minified assets and lockfiles; not source", () => {
    for (const p of [
      "webui/frontend/dist/assets/index-DZa2Gr9X.js",
      "build/main.css",
      "out/index.js",
      "vendor/github.com/x/y.go",
      "node_modules/react/index.js",
      "static/app.min.js",
      "pnpm-lock.yaml",
      "go.sum",
    ]) expect(isGeneratedPath(p)).toBe(true);
    for (const p of ["docs/DESIGN.md", "src/diffSummary.ts", "webui/main.go", "distribution/notes.md", "package.json"])
      expect(isGeneratedPath(p)).toBe(false);
  });

  it("does not count diff headers as wide content", () => {
    const headers = ["diff --git a/x b/x", "index 111..222 100644", "--- a/x", "+++ b/x", "@@ -1 +1 @@ " + "z".repeat(900)];
    expect(longestContentLine(headers)).toBe(0);
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

describe("splitDiff (INC-41 RVW-PHANTOM, no ghost row from the payload's newline)", () => {
  // Field case: `A rd-d-untracked-probe.txt +1 −0` rendered *two* body rows, the
  // second one blank, though the hunk is `@@ -0,0 +1,1 @@` with a single + line.
  // The payload's trailing "\n" split into a "" element that landed in the last
  // file's lines and came back out of the row builder as a blank ctx row.
  const added = ["diff --git a/probe.txt b/probe.txt", "new file mode 100644", "index 000..111", "--- /dev/null", "+++ b/probe.txt", "@@ -0,0 +1,1 @@", "+probe"];
  const modified = ["diff --git a/x.ts b/x.ts", "index 000..111 100644", "--- a/x.ts", "+++ b/x.ts", "@@ -10,3 +10,3 @@", " keep", "-old", "+new", " tail"];

  it("drops the newline terminator: the last file gains no phantom blank row", () => {
    const files = splitDiff(added.join("\n") + "\n");
    expect(files).toHaveLength(1);
    expect(files[0].lines).not.toContain("");
    const { rows } = parseFileDiff(files[0].lines);
    expect(rows).toHaveLength(2); // one @@ header + one "+probe" — nothing else
    expect(rows[1]).toMatchObject({ kind: "add", newNo: 1, text: "probe" });
    expect(files[0]).toMatchObject({ add: 1, del: 0 });
  });

  it("behaves identically when the payload has no trailing newline", () => {
    expect(splitDiff(added.join("\n"))).toEqual(splitDiff(added.join("\n") + "\n"));
  });

  it("only the last file was ever at risk — and now no file carries the ghost", () => {
    const files = splitDiff([...modified, ...added].join("\n") + "\n");
    expect(files.map((f) => f.path)).toEqual(["x.ts", "probe.txt"]);
    for (const f of files) expect(f.lines).not.toContain("");
    expect(parseFileDiff(files[1].lines).rows).toHaveLength(2);
  });

  it("keeps blank lines that are real diff content (' ' / '+' / '-', never '')", () => {
    // an empty context line, an empty addition and an empty deletion, mid-body
    const blanks = ["diff --git a/y.ts b/y.ts", "@@ -1,3 +1,3 @@", " ", "-", "+", " done"];
    const [file] = splitDiff(blanks.join("\n") + "\n");
    const { rows } = parseFileDiff(file.lines);
    expect(rows).toHaveLength(5); // hunk + ctx"" + del"" + add"" + ctx"done"
    expect(rows[1]).toMatchObject({ kind: "ctx", oldNo: 1, newNo: 1, text: "" });
    expect(rows[2]).toMatchObject({ kind: "del", oldNo: 2, text: "" });
    expect(rows[3]).toMatchObject({ kind: "add", newNo: 2, text: "" });
    expect(file).toMatchObject({ add: 1, del: 1 });
  });

  it("the trailing 'N unmodified lines' band no longer starts one line late", () => {
    const [file] = splitDiff(modified.join("\n") + "\n");
    const { rows } = parseFileDiff(file.lines);
    // last shown new line is 12 (" tail"), so the hidden run resumes at 13 — the
    // phantom ctx row used to consume newNo 13 and push the band to 14.
    expect(rows[rows.length - 1]).toMatchObject({ kind: "ctx", newNo: 12, text: "tail" });
    expect(hunkGaps(rows, { trailing: true }).get(trailingGapKey(rows))).toEqual({ start: 13, end: null });
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
