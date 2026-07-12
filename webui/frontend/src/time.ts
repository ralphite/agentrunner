// Codex shows a compact relative time on every list row (1w, 2w, 1mo). We
// derive it from the session id's date prefix (YYYYMMDD-HHMMSS-…) or a run's
// RFC3339 startedAt.
export function sessionDate(id: string): Date | null {
  const m = id.match(/^(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})/);
  if (!m) return null;
  const [, y, mo, d, h, mi, s] = m;
  // Session ids are stamped in UTC (ar uses now.UTC()); parse as UTC so the
  // relative time is correct in every timezone.
  return new Date(Date.UTC(+y, +mo - 1, +d, +h, +mi, +s));
}

// bucketOf groups a session/run into a Codex-style recency section for the
// sidebar. Returns a stable label + a sort rank (lower = more recent).
export function bucketOf(when: Date | null): { label: string; rank: number } {
  if (!when || isNaN(when.getTime())) return { label: "Undated", rank: 9 };
  const day = (Date.now() - when.getTime()) / 86400000;
  if (day < 1) return { label: "Today", rank: 0 };
  if (day < 2) return { label: "Yesterday", rank: 1 };
  if (day < 7) return { label: "Previous 7 days", rank: 2 };
  if (day < 30) return { label: "Previous 30 days", rank: 3 };
  return { label: "Older", rank: 4 };
}

export function relTime(when: Date | null): string {
  if (!when || isNaN(when.getTime())) return "";
  const sec = Math.max(0, (Date.now() - when.getTime()) / 1000);
  if (sec < 60) return "just now";
  const min = sec / 60;
  if (min < 60) return `${Math.floor(min)}m`;
  const hr = min / 60;
  if (hr < 24) return `${Math.floor(hr)}h`;
  const day = hr / 24;
  if (day < 7) return `${Math.floor(day)}d`;
  const wk = day / 7;
  if (wk < 5) return `${Math.floor(wk)}w`;
  const mon = day / 30;
  if (mon < 12) return `${Math.floor(mon)}mo`;
  return `${Math.floor(day / 365)}y`;
}

// Turn the compact stamp into a complete phrase. The newest bucket is already
// prose, so appending "ago" would produce the visible nonsense "just now ago".
export function relTimeAgo(when: Date | null): string {
  const rel = relTime(when);
  if (!rel) return "";
  return rel === "just now" ? rel : `${rel} ago`;
}
