import type { DiffResp } from "./types";

export interface FileDiffSummary {
  path: string;
  lines: string[];
  add: number;
  del: number;
  countsKnown: boolean;
}

export interface ChangesSummary {
  files: FileDiffSummary[];
  totalAdd: number;
  totalDel: number;
}

// One rendered row of a file diff. Hunk rows carry the @@ context line;
// content rows carry old/new line numbers for the gutter (absent on the side
// a line doesn't exist on).
export interface DiffRow {
  kind: "hunk" | "add" | "del" | "ctx";
  oldNo?: number;
  newNo?: number;
  text: string;
}

// The file's overall change status, distilled to a single Codex-style glyph
// (M/A/D/R/C). Absent any git status metadata a touched file is "modified".
export type FileStatus = "modified" | "added" | "deleted" | "renamed" | "copied";

export interface ParsedFileDiff {
  // header badges distilled from git's metadata lines ("new file",
  // "deleted", "renamed", "binary", "mode changed") — the raw meta lines
  // themselves never render (Codex hides them too).
  badges: string[];
  // leading status for the compact M/A/D/R glyph in the file header.
  status: FileStatus;
  rows: DiffRow[];
}

// parseFileDiff turns one file's raw unified-diff lines into gutter-numbered
// rows plus header badges. Pure; tolerant of truncated hunks.
export function parseFileDiff(lines: string[]): ParsedFileDiff {
  const badges: string[] = [];
  const badge = (b: string) => {
    if (!badges.includes(b)) badges.push(b);
  };
  const rows: DiffRow[] = [];
  let oldNo = 0;
  let newNo = 0;
  for (const line of lines) {
    if (line.startsWith("new file")) { badge("new file"); continue; }
    if (line.startsWith("deleted file")) { badge("deleted"); continue; }
    if (line.startsWith("rename from") || line.startsWith("rename to")) { badge("renamed"); continue; }
    if (line.startsWith("copy from") || line.startsWith("copy to")) { badge("copied"); continue; }
    if (line.startsWith("similarity index") || line.startsWith("dissimilarity index")) continue;
    if (line.startsWith("Binary files") || line.startsWith("GIT binary patch")) { badge("binary"); continue; }
    if (line.startsWith("old mode") || line.startsWith("new mode")) { badge("mode changed"); continue; }
    if (line.startsWith("index ") || line.startsWith("--- ") || line.startsWith("+++ ")) continue;
    const h = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@ ?(.*)$/.exec(line);
    if (h) {
      oldNo = parseInt(h[1], 10);
      newNo = parseInt(h[2], 10);
      rows.push({ kind: "hunk", text: h[3] });
      continue;
    }
    if (line.startsWith("+")) { rows.push({ kind: "add", newNo: newNo++, text: line.slice(1) }); continue; }
    if (line.startsWith("-")) { rows.push({ kind: "del", oldNo: oldNo++, text: line.slice(1) }); continue; }
    if (line.startsWith("\\")) { rows.push({ kind: "ctx", text: line }); continue; }
    rows.push({ kind: "ctx", oldNo: oldNo++, newNo: newNo++, text: line.startsWith(" ") ? line.slice(1) : line });
  }
  // Default to "modified": a plain content edit carries no new/deleted/renamed
  // metadata line, and Codex still stamps such files with an "M" glyph.
  const status: FileStatus = badges.includes("new file")
    ? "added"
    : badges.includes("deleted")
      ? "deleted"
      : badges.includes("renamed")
        ? "renamed"
        : badges.includes("copied")
          ? "copied"
          : "modified";
  return { badges, status, rows };
}

// A run of unmodified new-file lines hidden between (or before, or after) diff
// hunks — the region a "N unmodified lines" collapser reveals. `start`/`end` are
// 1-based inclusive new-file line numbers, so the client slices them straight out
// of the file blob.
//
// `end === null` means "runs to EOF, length unknown": a unified diff never states
// a file's total line count, so the trailing gap (last hunk → EOF) can only be
// bounded once the file blob is in hand. The renderer resolves a null end against
// the blob it fetches — it never invents a number.
export interface ContextGap {
  start: number;
  end: number | null;
}

// hunkGaps maps a row index → the ContextGap hidden immediately before it. Each
// hunk-header row index keys the gap in front of that hunk: the leading gap (file
// start → first hunk) hangs off the first hunk's index; each later gap spans from
// the previous hunk's last shown new line to the next hunk's first shown new line.
//
// INC-41 RD-2: with `opts.trailing`, one more gap is emitted for the region after
// the last hunk, keyed by `rows.length` — an index no real row can occupy, so it
// never collides. Its `end` is null (see ContextGap): git tells us where the diff
// stops, not where the file does. It is suppressed when the diff itself proves it
// already reached EOF (a trailing "\ No newline at end of file" marker), and
// callers should pass `trailing: false` for added/deleted files, whose diffs show
// the whole new side (or have none at all).
//
// Only positive-length gaps are returned. Pure; needs no blob.
export function hunkGaps(rows: DiffRow[], opts: { trailing?: boolean } = {}): Map<number, ContextGap> {
  const gaps = new Map<number, ContextGap>();
  let prevLastNew = 0; // last new-file line already shown by an earlier hunk
  const n = rows.length;
  for (let idx = 0; idx < n; idx++) {
    if (rows[idx].kind !== "hunk") continue;
    let firstNew = 0;
    let lastNew = prevLastNew;
    for (let j = idx + 1; j < n && rows[j].kind !== "hunk"; j++) {
      const no = rows[j].newNo;
      if (no !== undefined) {
        if (firstNew === 0) firstNew = no;
        lastNew = no;
      }
    }
    if (firstNew > 0 && firstNew - 1 >= prevLastNew + 1) {
      gaps.set(idx, { start: prevLastNew + 1, end: firstNew - 1 });
    }
    if (lastNew > prevLastNew) prevLastNew = lastNew;
  }
  const last = n > 0 ? rows[n - 1] : undefined;
  const diffRanToEOF = !!last && last.kind === "ctx" && last.newNo === undefined && last.text.startsWith("\\");
  if (opts.trailing && prevLastNew > 0 && !diffRanToEOF) {
    gaps.set(n, { start: prevLastNew + 1, end: null });
  }
  return gaps;
}

// trailingGapKey: the key hunkGaps uses for the after-the-last-hunk gap.
export const trailingGapKey = (rows: DiffRow[]) => rows.length;

// splitPath separates the directory (rendered dim) from the basename
// (rendered strong) the way Codex file headers do.
export function splitPath(path: string): { dir: string; base: string } {
  const i = path.lastIndexOf("/");
  return i < 0 ? { dir: "", base: path } : { dir: path.slice(0, i + 1), base: path.slice(i + 1) };
}

// INC-41 RVW-PHANTOM: a diff payload ends with a newline terminator, and
// `split("\n")` turns that terminator into a phantom "" element. It landed in
// the *last* file's lines, where parseFileDiff's fallback branch rendered it as
// a blank context row — every review's last file grew one ghost line — and, worse,
// it consumed an oldNo/newNo, pushing the trailing "N unmodified lines" band's
// start one line past the truth.
//
// Only *trailing* empties are dropped. Nothing legitimate inside a diff body is
// ever "": a blank context line is " ", a blank addition "+", a blank deletion
// "-". A bare "" can only come from the terminator.
function diffLines(diff: string): string[] {
  const lines = diff.split("\n");
  while (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
  return lines;
}

export function splitDiff(diff: string): FileDiffSummary[] {
  if (!diff.trim()) return [];
  const files: FileDiffSummary[] = [];
  let cur: FileDiffSummary | null = null;
  for (const line of diffLines(diff)) {
    if (line.startsWith("diff --git")) {
      const match = line.match(/ b\/(.+)$/);
      cur = { path: match ? match[1] : line, lines: [], add: 0, del: 0, countsKnown: true };
      files.push(cur);
    } else if (cur) {
      cur.lines.push(line);
      if (line.startsWith("+") && !line.startsWith("+++")) cur.add++;
      else if (line.startsWith("-") && !line.startsWith("---")) cur.del++;
    }
  }
  return files;
}

// ---- INC-41 D4: side-by-side (split) rows ----
// One visual row of a split diff. A hunk spans both columns; otherwise `left`
// carries the old side (del/ctx) and `right` the new side (add/ctx). Runs of
// deletions and additions between context lines are paired by index so a pure
// replacement lines its old/new halves up the way GitHub's split view does.
export interface SplitRow {
  hunk?: string;
  left?: DiffRow;
  right?: DiffRow;
}

export function splitRows(rows: DiffRow[]): SplitRow[] {
  const out: SplitRow[] = [];
  let dels: DiffRow[] = [];
  let adds: DiffRow[] = [];
  const flush = () => {
    const n = Math.max(dels.length, adds.length);
    for (let i = 0; i < n; i++) out.push({ left: dels[i], right: adds[i] });
    dels = [];
    adds = [];
  };
  for (const r of rows) {
    if (r.kind === "hunk") {
      flush();
      out.push({ hunk: r.text });
    } else if (r.kind === "del") {
      dels.push(r);
    } else if (r.kind === "add") {
      adds.push(r);
    } else {
      // context (and no-newline markers) reset the pairing and show on both sides
      flush();
      out.push({ left: r, right: r });
    }
  }
  flush();
  return out;
}

// ---- INC-41 D3: dependency-free diff syntax highlighting ----
// A highlighted token: `t` is the literal source text (never dropped/reordered,
// so `white-space: pre` layout stays byte-exact); `c` is a CSS class suffix
// (.hl-<c>) or undefined for plain text.
export type HlClass = "kw" | "str" | "com" | "num" | "fn" | "punc" | "code";
export interface HlToken {
  t: string;
  c?: HlClass;
}

interface LangSpec {
  line: string[]; // line-comment starters
  block: boolean; // support /* … */ C-style block comments (single line)
  strings: string[]; // string delimiters
  keywords: Set<string>;
}

const kw = (s: string) => new Set(s.trim().split(/\s+/));

const JS_KW = kw(`
  const let var function return if else for while do switch case break continue
  new class extends super import export from default async await yield typeof
  instanceof in of try catch finally throw void this null undefined true false
  as interface type enum namespace public private protected readonly static get
  set implements declare abstract satisfies keyof infer never unknown any
`);
const GO_KW = kw(`
  func var const type struct interface map chan go defer return if else for range
  switch case break continue package import nil true false iota select fallthrough
  goto make new append len cap panic recover string int int8 int16 int32 int64
  uint uint8 uint16 uint32 uint64 float32 float64 bool byte rune error
`);
const PY_KW = kw(`
  def class return if elif else for while import from as pass break continue with
  try except finally raise lambda yield global nonlocal None True False and or not
  in is del assert async await match case self
`);
const RUST_KW = kw(`
  fn let mut const static struct enum impl trait pub use mod match if else for
  while loop return break continue Some None Ok Err self Self where async await
  move ref dyn as in crate super type unsafe
`);
const SH_KW = kw(`
  if then fi elif else for do done while until case esac function in select time
  echo cd export local return set unset source read shift exit trap
`);
// INC-41 RD-3 — markup (html/xml/svg). Tag names are the "keywords"; quoted
// attribute values fall out of the shared string rule and `< > / =` out of the
// punctuation rule, which is exactly the shape Codex gives markup. Deliberately
// no line- or block-comment starters: `//` inside a URL must not swallow the rest
// of the line, and `<!-- -->` is not C-style (it degrades to punctuation, which
// is harmless).
const HTML_KW = kw(`
  html head body title meta link script style base noscript template slot
  div span p a img picture source video audio canvas iframe embed object
  ul ol li dl dt dd table thead tbody tfoot tr td th caption colgroup col
  form input button label select option optgroup textarea fieldset legend
  header footer main nav section article aside figure figcaption details summary
  h1 h2 h3 h4 h5 h6 br hr pre code em strong b i u small sub sup blockquote
  svg path g rect circle ellipse line polyline polygon text defs use symbol
  xml DOCTYPE doctype
`);

const LANGS: Record<string, LangSpec> = {
  js: { line: ["//"], block: true, strings: ['"', "'", "`"], keywords: JS_KW },
  go: { line: ["//"], block: true, strings: ['"', "'", "`"], keywords: GO_KW },
  rust: { line: ["//"], block: true, strings: ['"'], keywords: RUST_KW },
  css: { line: [], block: true, strings: ['"', "'"], keywords: new Set() },
  py: { line: ["#"], block: false, strings: ['"', "'"], keywords: PY_KW },
  sh: { line: ["#"], block: false, strings: ['"', "'"], keywords: SH_KW },
  yaml: { line: ["#"], block: false, strings: ['"', "'"], keywords: kw("true false null yes no on off") },
  json: { line: [], block: false, strings: ['"'], keywords: kw("true false null") },
  html: { line: [], block: false, strings: ['"', "'"], keywords: HTML_KW },
};

const EXT_LANG: Record<string, string> = {
  ts: "js", tsx: "js", js: "js", jsx: "js", mjs: "js", cjs: "js",
  go: "go", rs: "rust", py: "py", pyi: "py",
  sh: "sh", bash: "sh", zsh: "sh",
  yaml: "yaml", yml: "yaml", json: "json",
  css: "css", scss: "css", less: "css",
  md: "md", markdown: "md",
  // RD-3: markup was missing entirely, so dist/index.html rendered as a slab of
  // undifferentiated black next to fully-colored .js siblings.
  html: "html", htm: "html", xhtml: "html", xml: "html", svg: "html",
};

export function langFromPath(path: string): string {
  const m = /\.([A-Za-z0-9]+)$/.exec(path);
  return m ? EXT_LANG[m[1].toLowerCase()] || "" : "";
}

const isWord = (ch: string) => /[A-Za-z0-9_$]/.test(ch);
const isIdentStart = (ch: string) => /[A-Za-z_$]/.test(ch);

// highlightLine tokenizes one diff content line (prefix already stripped). It
// is total over its input — concatenating token texts always rebuilds the
// original string — and returns a single plain token for unknown languages.
export function highlightLine(text: string, lang: string): HlToken[] {
  if (lang === "md") return highlightMd(text);
  const spec = LANGS[lang];
  if (!spec || !text) return [{ t: text }];
  const out: HlToken[] = [];
  let plain = "";
  const flush = () => {
    if (plain) {
      out.push({ t: plain });
      plain = "";
    }
  };
  const push = (t: string, c?: HlClass) => {
    flush();
    out.push({ t, c });
  };
  let i = 0;
  const n = text.length;
  while (i < n) {
    const ch = text[i];
    // line comments
    let matchedLine = false;
    for (const lc of spec.line) {
      if (text.startsWith(lc, i)) {
        push(text.slice(i), "com");
        i = n;
        matchedLine = true;
        break;
      }
    }
    if (matchedLine) break;
    // block comments (single-line slice; run to close or EOL)
    if (spec.block && ch === "/" && text[i + 1] === "*") {
      const end = text.indexOf("*/", i + 2);
      const stop = end < 0 ? n : end + 2;
      push(text.slice(i, stop), "com");
      i = stop;
      continue;
    }
    // strings
    if (spec.strings.includes(ch)) {
      let j = i + 1;
      while (j < n) {
        if (text[j] === "\\") { j += 2; continue; }
        if (text[j] === ch) { j++; break; }
        j++;
      }
      push(text.slice(i, j), "str");
      i = j;
      continue;
    }
    // numbers
    if (/[0-9]/.test(ch) && !(i > 0 && isWord(text[i - 1]))) {
      let j = i;
      while (j < n && /[0-9a-fA-FxX._eE+-]/.test(text[j])) {
        // stop a stray +/- that isn't part of an exponent
        if ((text[j] === "+" || text[j] === "-") && !/[eE]/.test(text[j - 1])) break;
        j++;
      }
      push(text.slice(i, j), "num");
      i = j;
      continue;
    }
    // identifiers / keywords / call names
    if (isIdentStart(ch)) {
      let j = i + 1;
      while (j < n && isWord(text[j])) j++;
      const word = text.slice(i, j);
      let k = j;
      while (k < n && text[k] === " ") k++;
      if (spec.keywords.has(word)) push(word, "kw");
      else if (text[k] === "(") push(word, "fn");
      else plain += word;
      i = j;
      continue;
    }
    // punctuation clusters read a touch stronger than identifiers
    if (/[{}()[\];,.:<>=!&|+\-*/%?]/.test(ch)) {
      push(ch, "punc");
      i++;
      continue;
    }
    plain += ch;
    i++;
  }
  flush();
  return out;
}

// Markdown gets a lighter, structure-first pass: heading markers, inline code
// spans, emphasis markers and bare list bullets — enough to keep prose diffs
// readable without a full CommonMark parser.
function highlightMd(text: string): HlToken[] {
  if (!text) return [{ t: text }];
  const out: HlToken[] = [];
  const lead = /^(\s*)(#{1,6}\s|[-*+]\s|\d+\.\s|>\s)?/.exec(text);
  let i = 0;
  if (lead && lead[0]) {
    if (lead[1]) out.push({ t: lead[1] });
    if (lead[2]) out.push({ t: lead[2], c: lead[2].trim().startsWith("#") ? "kw" : "punc" });
    i = lead[0].length;
  }
  let plain = "";
  const flush = () => { if (plain) { out.push({ t: plain }); plain = ""; } };
  const n = text.length;
  while (i < n) {
    const ch = text[i];
    if (ch === "`") {
      const end = text.indexOf("`", i + 1);
      const stop = end < 0 ? n : end + 1;
      flush();
      // Inline code spans get their own teal class (.hl-code) rather than the
      // orange string color, so backticked identifiers read as code, not prose.
      out.push({ t: text.slice(i, stop), c: "code" });
      i = stop;
      continue;
    }
    if ((ch === "*" || ch === "_") && (text[i + 1] === ch)) {
      flush();
      out.push({ t: ch + ch, c: "punc" });
      i += 2;
      continue;
    }
    plain += ch;
    i++;
  }
  flush();
  return out;
}

export function summarizeChanges(data: DiffResp): ChangesSummary {
  const files = splitDiff(data.diff || "");
  const seen = new Set(files.map((file) => file.path));
  for (const path of data.untracked || []) {
    if (!seen.has(path)) files.push({ path, lines: [], add: 0, del: 0, countsKnown: false });
  }
  return {
    files,
    totalAdd: files.reduce((total, file) => total + file.add, 0),
    totalDel: files.reduce((total, file) => total + file.del, 0),
  };
}

// ---- INC-41 RD-1: per-FILE disclosure ----
// f2f1932's performance intent is kept — never paint a monstrous file inline on
// first open — but the judgement moves from the REVIEW to the FILE. The old
// global rule collapsed *every* file as soon as *one* was big: a 3-file review
// with a 1,284-line package-lock.json also folded package.json (+12) and
// server.js (+110), so "Open Changes" showed three bare file headers and zero
// lines of code. Codex always shows code and controls volume with the unmodified
// collapser bands instead.
//
// MAX_INLINE_FILE_LINES caps one file; MAX_INLINE_REVIEW_LINES is the whole
// review's first-paint budget — files open in order until it runs out, so a
// 200-file dump still opens code at the top instead of everything at once. The
// "Expand all" / "Collapse all" control overrides either way.
export const MAX_INLINE_FILE_LINES = 500;
export const MAX_INLINE_REVIEW_LINES = 5000;

// ---- INC-41 DF-2: build artefacts / minified files never open by default ----
// The line-count rule alone measured the wrong axis. A deleted bundle
// (dist/assets/index-BLupS6ef.js, −176 lines) is *short*, so it passed
// MAX_INLINE_FILE_LINES and painted 176 lines of minified React — one of them
// 1.9M pixels wide — burying the two lines of index.html the reviewer actually
// wanted. Meanwhile a 1,284-line package-lock.json was correctly folded. Same
// noise, opposite verdicts, purely because one happened to be wide instead of
// tall. Codex's review pane is a readable stream of *source*; generated files
// keep their header and ±counts (and still open on a click) but never claim the
// first screen.
export const MAX_INLINE_LINE_WIDTH = 500;
// A generated file gets a much smaller inline budget than source: a two-line
// asset-hash bump in dist/index.html is exactly what the reviewer came to see,
// a 176-line bundle is not.
export const MAX_INLINE_GENERATED_LINES = 20;

// isGeneratedPath: build output, vendored trees, minified assets, lockfiles.
// Path-shaped judgement only — no content needed — so it also covers untracked
// entries and binary-ish files whose lines we never parsed.
export function isGeneratedPath(path: string): boolean {
  return (
    /(^|\/)(dist|build|out|vendor|node_modules|__pycache__|\.?venv|site-packages|\.tox|\.eggs)\//.test(path) ||
    /\.(dist-info|egg-info)\//.test(path) ||
    /\.(pyc|pyo)$/.test(path) ||
    /\.min\.(js|css)$/.test(path) ||
    /-lock\.(json|yaml|yml)$/.test(path) ||
    /(^|\/)(package-lock\.json|yarn\.lock|pnpm-lock\.yaml|go\.sum|Cargo\.lock)$/.test(path) ||
    /\/assets\/index-[A-Za-z0-9_-]+\.(js|css)$/.test(path)
  );
}

// dropGeneratedFiles: the changes CARD names what the agent meaningfully
// edited — compiled artifacts (__pycache__/*.pyc rows on the user's phone,
// QA-0719 review #7) are noise there, the same judgement DiffView already
// applies to its inline budget. Returns null when nothing but generated files
// changed: the card must not claim reviewable edits it wouldn't show.
export function dropGeneratedFiles(summary: ChangesSummary | null): ChangesSummary | null {
  if (!summary) return null;
  const kept = summary.files.filter((file) => !isGeneratedPath(file.path));
  if (kept.length === summary.files.length) return summary;
  if (!kept.length) return null;
  return {
    files: kept,
    totalAdd: kept.reduce((total, file) => total + file.add, 0),
    totalDel: kept.reduce((total, file) => total + file.del, 0),
  };
}

// longestContentLine: widest diff body line (the leading +/-/space marker does
// not count). Cheap — the caller already holds the split lines.
export function longestContentLine(lines: string[]): number {
  let max = 0;
  for (const line of lines) {
    if (/^(diff |index |\+\+\+ |--- |@@ |new file|deleted file|similarity |rename |Binary )/.test(line)) continue;
    const width = line.length > 0 ? line.length - 1 : 0;
    if (width > max) max = width;
  }
  return max;
}

export function shouldExpandFileByDefault(file: { path?: string; add: number; del: number; lines?: string[] }): boolean {
  // Minified content is unreadable at any length — one 1.9M-pixel-wide line is
  // worse than a thousand narrow ones.
  if (file.lines && longestContentLine(file.lines) > MAX_INLINE_LINE_WIDTH) return false;
  const budget = file.path && isGeneratedPath(file.path) ? MAX_INLINE_GENERATED_LINES : MAX_INLINE_FILE_LINES;
  return file.add + file.del <= budget;
}

// defaultOpenByPath decides, per file path, whether its diff starts expanded.
export function defaultOpenByPath(files: { path: string; add: number; del: number; lines?: string[] }[]): Map<string, boolean> {
  const out = new Map<string, boolean>();
  let budget = MAX_INLINE_REVIEW_LINES;
  for (const file of files) {
    const open = shouldExpandFileByDefault(file) && budget > 0;
    if (open) budget -= file.add + file.del;
    out.set(file.path, open);
  }
  return out;
}
