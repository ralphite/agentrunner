// displayTitle resolves the label shown for a session: a user rename wins over
// the auto-derived title, which wins over the raw id. Renames are a local
// preference (localStorage), mirroring pinned/archived — the server keeps its
// own derived title untouched.
export function displayTitle(
  renames: Record<string, string>,
  sid: string,
  rawTitle?: string,
): string {
  const custom = renames[sid];
  if (custom && custom.trim()) return custom.trim();
  return rawTitle || sid;
}
