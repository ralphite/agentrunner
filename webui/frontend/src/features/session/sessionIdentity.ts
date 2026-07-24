// Keep the browser-side session verdict aligned with webui/ar.go's id grammar.
const SESSION_ID_RE = /^[A-Za-z0-9._#-]+$/;

export function isValidSessionId(sid: string): boolean {
  return !!sid && sid.length <= 200 && SESSION_ID_RE.test(sid);
}

// Distinguish a permanent "this id does not exist" verdict from transient
// daemon/network failures. The narrow 400 check intentionally mirrors the
// server response for syntactically invalid ids.
export function isSessionNotFound(err: unknown): boolean {
  const e = err as { status?: unknown; code?: unknown; message?: unknown } | null | undefined;
  if (e && (e.status === 404 || e.code === "session_not_found")) return true;
  const msg = err instanceof Error ? err.message : typeof err === "string" ? err : "";
  if (e && e.status === 400 && /invalid session id/i.test(msg)) return true;
  return /no session matches/i.test(msg);
}
