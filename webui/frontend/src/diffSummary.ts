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
