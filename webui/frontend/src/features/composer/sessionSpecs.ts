import { productionAppServices } from "../../app/appServices";

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
const MODELS_KEY = "arwebui.sessModels";

function loadMap(storage: Storage, key: string): Record<string, string> {
  try {
    const v = JSON.parse(storage.getItem(key) || "{}");
    return v && typeof v === "object" ? v : {};
  } catch {
    return {};
  }
}

function saveMap(storage: Storage, key: string, m: Record<string, string>) {
  try {
    storage.setItem(key, JSON.stringify(m));
  } catch {
    /* quota — stay best-effort */
  }
}

const specMaps = new WeakMap<Storage, Record<string, string>>();
const accessMaps = new WeakMap<Storage, Record<string, string>>();
const modelMaps = new WeakMap<Storage, Record<string, string>>();
const localDefault = () => productionAppServices.storage.local;
const sessionDefault = () => productionAppServices.storage.session;

function cachedMap(
  cache: WeakMap<Storage, Record<string, string>>,
  storage: Storage,
  key: string,
): Record<string, string> {
  const current = cache.get(storage);
  if (current) return current;
  const loaded = loadMap(storage, key);
  cache.set(storage, loaded);
  return loaded;
}

export const rememberSpec = (sid: string, spec: string, storage = localDefault()) => {
  if (!sid || !spec) return;
  const specs = cachedMap(specMaps, storage, SPECS_KEY);
  specs[sid] = spec;
  saveMap(storage, SPECS_KEY, specs);
};
export const recallSpec = (sid: string, storage = localDefault()): string | undefined =>
  cachedMap(specMaps, storage, SPECS_KEY)[sid];

export const rememberAccess = (sid: string, a: string, storage = localDefault()) => {
  if (!sid || !a) return;
  const access = cachedMap(accessMaps, storage, ACCESS_KEY);
  access[sid] = a;
  saveMap(storage, ACCESS_KEY, access);
};
export const recallAccess = (sid: string, storage = localDefault()): string | undefined =>
  cachedMap(accessMaps, storage, ACCESS_KEY)[sid];

export interface RememberedModel {
  provider: string;
  model: string;
  effort: string;
}

export const rememberModel = (
  sid: string,
  model: RememberedModel,
  storage = localDefault(),
) => {
  if (!sid || !model.provider || !model.model || !model.effort) return;
  const models = cachedMap(modelMaps, storage, MODELS_KEY);
  models[sid] = JSON.stringify(model);
  saveMap(storage, MODELS_KEY, models);
};

export const recallModel = (
  sid: string,
  storage = localDefault(),
): RememberedModel | undefined => {
  try {
    const models = cachedMap(modelMaps, storage, MODELS_KEY);
    const value = JSON.parse(models[sid] || "");
    if (value?.provider && value?.model && value?.effort) return value;
  } catch {
    /* absent/legacy */
  }
  return undefined;
};

// Per-session composer text drafts: switching sessions and reloading this tab
// keeps what you were typing (send/clear wipes it). sessionStorage is exactly
// the intended lifetime: reload-safe but tab-local, so two tabs editing the same
// session never overwrite each other's half-typed text. The in-memory map keeps
// input working when storage is unavailable or full.
const DRAFT_PREFIX = "arwebui.draft.";
const draftMaps = new WeakMap<Storage, Map<string, string>>();

function draftsFor(storage: Storage): Map<string, string> {
  const current = draftMaps.get(storage);
  if (current) return current;
  const created = new Map<string, string>();
  draftMaps.set(storage, created);
  return created;
}

function draftStorageKey(key: string): string {
  return `${DRAFT_PREFIX}${encodeURIComponent(key)}`;
}

export const rememberDraft = (key: string, text: string, storage = sessionDefault()) => {
  if (!key) return;
  const drafts = draftsFor(storage);
  if (text) {
    drafts.set(key, text);
    try {
      storage.setItem(draftStorageKey(key), text);
    } catch {
      /* quota/privacy mode — the current tab still has the in-memory copy */
    }
    return;
  }
  drafts.delete(key);
  try {
    storage.removeItem(draftStorageKey(key));
  } catch {
    /* unavailable storage */
  }
};
export const recallDraft = (key: string, storage = sessionDefault()): string => {
  if (!key) return "";
  const drafts = draftsFor(storage);
  const current = drafts.get(key);
  if (current !== undefined) return current;
  try {
    const stored = storage.getItem(draftStorageKey(key)) || "";
    if (stored) drafts.set(key, stored);
    return stored;
  } catch {
    return "";
  }
};
