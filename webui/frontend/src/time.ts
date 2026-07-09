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
