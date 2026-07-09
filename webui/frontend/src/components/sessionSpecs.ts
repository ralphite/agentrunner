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
