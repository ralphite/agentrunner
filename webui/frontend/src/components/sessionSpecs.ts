// A localStorage-backed record of what this UI configured each session with,
// so a mid-session model switch can edit *that* spec (keeping its
// system_prompt / tools / permissions) and the session composer can show the
// right approval posture. Persisted so a reload or a second tab agrees with
// the tab that created the session (QA Round1 F-C3: the in-memory version
// made a freshly-loaded tab misreport the approval mode). Best-effort: only
// populated for sessions this UI created or switched; sessions born in the
// CLI have no entry — callers must show an honest "unknown", not a guess.
const SPECS_KEY = "arwebui.sessSpecs";
const ACCESS_KEY = "arwebui.sessAccess";

function loadMap(key: string): Record<string, string> {
  try {
    const v = JSON.parse(localStorage.getItem(key) || "{}");
    return v && typeof v === "object" ? v : {};
  } catch {
    return {};
  }
}

function saveMap(key: string, m: Record<string, string>) {
  try {
    localStorage.setItem(key, JSON.stringify(m));
  } catch {
    /* quota — stay best-effort */
  }
}

const specs = loadMap(SPECS_KEY);
const access = loadMap(ACCESS_KEY);

export const rememberSpec = (sid: string, spec: string) => {
  if (!sid || !spec) return;
  specs[sid] = spec;
  saveMap(SPECS_KEY, specs);
};
export const recallSpec = (sid: string): string | undefined => specs[sid];

export const rememberAccess = (sid: string, a: string) => {
  if (!sid || !a) return;
  access[sid] = a;
  saveMap(ACCESS_KEY, access);
};
export const recallAccess = (sid: string): string | undefined => access[sid];

// Per-session composer drafts: switching tasks keeps what you were typing and
// restores it when you come back (send/clear wipes it). Keyed by sid; the
// landing composer uses its own sentinel key. Drafts stay in-memory: losing
// one on reload is fine, syncing half-typed text across tabs is not.
const drafts = new Map<string, string>();

export const rememberDraft = (key: string, text: string) => {
  if (!key) return;
  if (text) drafts.set(key, text);
  else drafts.delete(key);
};
export const recallDraft = (key: string): string => drafts.get(key) || "";
