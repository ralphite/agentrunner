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
