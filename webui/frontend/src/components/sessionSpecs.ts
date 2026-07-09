// A tiny in-memory record of what this UI configured each session with, so a
// mid-session model switch can edit *that* spec (keeping its system_prompt /
// tools / permissions) and the session composer can show the right approval
// posture. Best-effort: only populated for sessions this UI created or switched;
// sessions born in the CLI fall back to defaults.
const specs = new Map<string, string>();
const access = new Map<string, string>();

export const rememberSpec = (sid: string, spec: string) => {
  if (sid && spec) specs.set(sid, spec);
};
export const recallSpec = (sid: string): string | undefined => specs.get(sid);

export const rememberAccess = (sid: string, a: string) => {
  if (sid && a) access.set(sid, a);
};
export const recallAccess = (sid: string): string | undefined => access.get(sid);

// Per-session composer drafts: switching tasks keeps what you were typing and
// restores it when you come back (send/clear wipes it). Keyed by sid; the
// landing composer uses its own sentinel key.
const drafts = new Map<string, string>();

export const rememberDraft = (key: string, text: string) => {
  if (!key) return;
  if (text) drafts.set(key, text);
  else drafts.delete(key);
};
export const recallDraft = (key: string): string => drafts.get(key) || "";
