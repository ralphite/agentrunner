// Browser notifications for attention-worthy transitions — Codex's completion /
// permission / question notifications. Best-effort: silently no-ops without
// permission or Notification support.
import type { Run, Session } from "./types";

export function requestNotifyPermission() {
  try {
    if ("Notification" in window && Notification.permission === "default") {
      void Notification.requestPermission();
    }
  } catch {
    /* ignore */
  }
}

function notify(title: string, body: string) {
  try {
    if ("Notification" in window && Notification.permission === "granted") {
      new Notification(title, { body });
    }
  } catch {
    /* ignore */
  }
}

// notifySessionChanges fires Codex's "completion" notification: a session you
// aren't currently viewing finished its turn (transitioned out of running/busy).
// The daemon reports an approval-wait as "running", so completion is the signal
// the sessions list actually exposes.
const active = (s: string) => /run|busy/i.test(s);
export function notifySessionChanges(prev: Session[], next: Session[], currentSid: string | null) {
  const prevStatus = new Map(prev.map((s) => [s.id, s.status]));
  for (const s of next) {
    const p = prevStatus.get(s.id);
    if (p === undefined) continue; // brand-new session: don't announce it here
    if (active(p) && !active(s.status) && s.id !== currentSid) {
      notify("Session finished", `${s.title || s.id} · ${s.status}`);
    }
  }
}

// notifyRunChanges fires when a background run leaves the running state
// (finished / failed / stopped).
export function notifyRunChanges(prev: Run[], next: Run[]) {
  const prevStatus = new Map(prev.map((r) => [r.id, r.status]));
  for (const r of next) {
    const p = prevStatus.get(r.id);
    if (p === "running" && r.status !== "running") {
      notify("Background run finished", `${r.label || r.id} · ${r.status}`);
    }
  }
}
