// matchesQuery is the shared "Search settings…" predicate: empty query matches
// everything; otherwise every whitespace-separated term must appear somewhere
// in the haystack (label + keywords). Kept in one place so the nav rail and
// each panel filter identically.
export function matchesQuery(query: string, haystack: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  const hay = haystack.toLowerCase();
  return q.split(/\s+/).every((term) => hay.includes(term));
}
