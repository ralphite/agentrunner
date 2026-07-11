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

export interface ParsedFileDiff {
  // header badges distilled from git's metadata lines ("new file",
  // "deleted", "renamed", "binary", "mode changed") — the raw meta lines
  // themselves never render (Codex hides them too).
  badges: string[];
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
  return { badges, rows };
}

// splitPath separates the directory (rendered dim) from the basename
// (rendered strong) the way Codex file headers do.
export function splitPath(path: string): { dir: string; base: string } {
  const i = path.lastIndexOf("/");
  return i < 0 ? { dir: "", base: path } : { dir: path.slice(0, i + 1), base: path.slice(i + 1) };
}

export function splitDiff(diff: string): FileDiffSummary[] {
  if (!diff.trim()) return [];
  const files: FileDiffSummary[] = [];
  let cur: FileDiffSummary | null = null;
  for (const line of diff.split("\n")) {
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
export type HlClass = "kw" | "str" | "com" | "num" | "fn" | "punc";
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

const LANGS: Record<string, LangSpec> = {
  js: { line: ["//"], block: true, strings: ['"', "'", "`"], keywords: JS_KW },
  go: { line: ["//"], block: true, strings: ['"', "'", "`"], keywords: GO_KW },
  rust: { line: ["//"], block: true, strings: ['"'], keywords: RUST_KW },
  css: { line: [], block: true, strings: ['"', "'"], keywords: new Set() },
  py: { line: ["#"], block: false, strings: ['"', "'"], keywords: PY_KW },
  sh: { line: ["#"], block: false, strings: ['"', "'"], keywords: SH_KW },
  yaml: { line: ["#"], block: false, strings: ['"', "'"], keywords: kw("true false null yes no on off") },
  json: { line: [], block: false, strings: ['"'], keywords: kw("true false null") },
};

const EXT_LANG: Record<string, string> = {
  ts: "js", tsx: "js", js: "js", jsx: "js", mjs: "js", cjs: "js",
  go: "go", rs: "rust", py: "py", pyi: "py",
  sh: "sh", bash: "sh", zsh: "sh",
  yaml: "yaml", yml: "yaml", json: "json",
  css: "css", scss: "css", less: "css",
  md: "md", markdown: "md",
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
      out.push({ t: text.slice(i, stop), c: "str" });
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

export function shouldExpandDiffByDefault(diff: string): boolean {
  const files = splitDiff(diff || "");
  const changedLines = files.reduce((total, file) => total + file.add + file.del, 0);
  return files.length <= 10 && changedLines <= 500;
}
