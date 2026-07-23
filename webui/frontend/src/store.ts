import { createContext, createElement, useContext, type ReactNode } from "react";
import { useStore as useZustandStore, type UseBoundStore } from "zustand";
import { createStore, type StoreApi } from "zustand/vanilla";
import type { Health, LauncherApp, ProjectMeta, Run, Session } from "./types";
import { nextTheme, type Theme } from "./theme";
import type { CadenceSpec, RunPreset } from "./runPreset";
import {
  createProductionAppServices,
  productionAppServices,
  type AppServices,
} from "./app/appServices";

export type ModalKind =
  | { kind: "new"; message?: string; spec?: string; worker?: string; provider?: string; model?: string; effort?: string }
  // submit / drive launcher. `cadence` (SC-18) is the RHYTHM the caller already
  // showed the user — a Scheduled suggestion card's "Weekdays at 8:00 AM" — as
  // the driver-spec fields themselves. The launcher opens on it instead of the
  // preset's generic default, so what gets built is what was clicked.
  | {
      kind: "run";
      prompt?: string;
      preset?: RunPreset;
      cadence?: CadenceSpec;
      // Ephemeral UI reference only: lets pointer-launched suggestion dialogs
      // return focus to the card that opened them. Never persisted or sent.
      returnFocus?: HTMLElement;
    }
  | { kind: "fork"; sid: string }
  | { kind: "agent"; sid: string; provider?: string; model?: string; effort?: string }
  | { kind: "rename"; sid: string }
  | { kind: "trust" }
  | {
      kind: "confirm";
      title: string;
      body: string;
      confirmLabel: string;
      danger?: boolean;
      details?: Array<{ icon: "files" | "terminal" | "internet"; title: string; body: string }>;
      note?: string;
      onConfirm: () => void | Promise<void>;
      onClose?: () => void;
    }
  | { kind: "inspect"; data: unknown; status?: string }
  | { kind: "viewer"; title: string; body: string }
  | null;

// PromptState is the app-styled replacement for window.prompt (QA Round1
// F-C1: the native dialog synchronously freezes the renderer and clashes
// with the app's modal style). It lives in its own slot so it can stack on
// top of an open modal (e.g. the worktree path asked from "New session").
// onSubmit fires only on a non-empty submit.
export interface PromptState {
  title: string;
  label?: string;
  initial?: string;
  placeholder?: string;
  submitLabel?: string; // primary button text (default "OK"); e.g. "Commit"
  onSubmit: (value: string) => void;
}

interface ToastMsg {
  id: number;
  text: string;
  kind: "error" | "info";
  // Raw CLI/git stderr for an optional "Details" disclosure (G36 余项): the
  // toast sentence stays friendly, the scary blob is one tap away, on demand.
  details?: string;
}

// The full-window destinations reachable from the sidebar's primary nav
// (New session / Scheduled). "home" is the New-session landing; the Scheduled page
// routes to a matching hash (#scheduled).
export type Page = "home" | "scheduled";

export interface NewSessionProject {
  workspace: string;
  requestId: number;
}

export interface AppState {
  health: Health | null;
  sessions: Session[];
  // False until the first successful session-list response. An empty array
  // before then is "not loaded", not proof that the user has no sessions.
  sessionsReady: boolean;
  // True only while the first successful recent page is being extended with
  // older pages. The recent page is already usable at this point.
  sessionsLoadingOlder: boolean;
  runs: Run[];
  currentSid: string | null;
  currentRunId: string | null;
  currentPage: Page;
  scheduledDetailSid: string | null;
  // One-shot intent from a project-row New chat shortcut. The request id lets
  // an already-mounted Home composer react even when the same project is
  // chosen twice in a row, without remounting and losing its draft/settings.
  newSessionProject: NewSessionProject | null;
  newSessionForProject: (workspace: string) => void;
  consumeNewSessionProject: (requestId: number) => void;
  modal: ModalKind;
  prompt: PromptState | null;
  toasts: ToastMsg[];
  showSys: boolean;
  toggleSys: () => void;
  theme: Theme;
  cycleTheme: () => void;
  helpOpen: boolean; // keyboard-shortcuts reference overlay
  openHelp: () => void;
  closeHelp: () => void;
  archived: string[]; // session ids the user has archived (localStorage-backed)
  showArchived: boolean;
  toggleArchive: (id: string) => void;
  toggleShowArchived: () => void;
  pinned: string[]; // session ids pinned to the top of the sidebar (localStorage-backed)
  togglePin: (id: string) => void;
  renames: Record<string, string>; // session id -> custom title (localStorage-backed)
  setRename: (id: string, title: string) => void;
  sidebarCollapsed: boolean; // hide the sidebar for a full-width conversation (localStorage-backed)
  toggleSidebar: () => void;
  sidebarWidth: number; // desktop rail width in px (localStorage-backed)
  setSidebarWidth: (width: number) => void;
  unread: string[]; // sids with new activity you haven't opened (localStorage-backed)
  markUnread: (id: string) => void;
  markRead: (id: string) => void;

  visibleOrder: string[]; // sidebar's flat session order, for keyboard nav
  setVisibleOrder: (ids: string[]) => void;
  selectAdjacent: (delta: number) => void;

  // Project overlay (INC-53, HANDA #24): server-side, workspace-keyed cosmetic
  // preferences (custom name / folded / last_opened) layered on the
  // journal-derived project groups. Keyed by the project group's identity key.
  projects: Record<string, ProjectMeta>;
  refreshProjects: () => Promise<void>;
  setProjectName: (key: string, name: string) => Promise<void>;
  toggleProjectFolded: (key: string, folded: boolean) => Promise<void>;
  toggleProjectPinned: (key: string, pinned: boolean) => Promise<void>;
  setProjectRemoved: (key: string, removed: boolean) => Promise<void>;
  openProjectIn: (workspace: string, app: LauncherApp) => Promise<void>;

  // INC-41 TH-5 · pending "open the review AT this file". The thread's "Edited N
  // files" card names the files it touched; clicking one is a navigation, so it
  // hands the path off here and switches to the Changes panel. DiffView consumes
  // the path on its next render (expand that file + scroll it into view) and
  // clears it — it is a one-shot request, not a selection: it must not re-fire
  // when the panel is re-opened later, and no other surface reads it.
  diffFocusPath: string | null;
  focusDiffFile: (path: string) => void;
  clearDiffFocus: () => void;

  // QA-0719 · git-fact surfaces (the changes card, the rail's Environment
  // rows) re-read on session events — but Undo/commit/push/git-init are
  // UI-side mutations that emit no event, so the surfaces kept stating stale
  // facts (rail said "Changes · 1 file" after Undo emptied the tree). Every
  // UI action that mutates the workspace bumps this; consumers fold it into
  // their refreshKey.
  workspaceEpoch: number;
  bumpWorkspaceEpoch: () => void;

  refreshHealth: () => Promise<void>;
  refreshSessions: () => Promise<void>;
  refreshRuns: () => Promise<void>;
  select: (sid: string | null) => void;
  selectRun: (rid: string | null) => void;
  showPage: (page: Page) => void;
  showScheduledDetail: (sid: string | null) => void;
  openModal: (m: ModalKind) => void;
  openPrompt: (p: PromptState | null) => void;
  toast: (text: string, kind?: "error" | "info", details?: string) => void;
  dismissToast: (id: number) => void;
}

const ARCHIVE_KEY = "arwebui.archived";
function loadArchived(storage: Storage): string[] {
  try {
    return JSON.parse(storage.getItem(ARCHIVE_KEY) || "[]");
  } catch {
    return [];
  }
}

const PIN_KEY = "arwebui.pinned";
function loadPinned(storage: Storage): string[] {
  try {
    return JSON.parse(storage.getItem(PIN_KEY) || "[]");
  } catch {
    return [];
  }
}

// Manual renames are a JOURNAL fact since PLAN 5.6 (`ar title` →
// SessionTitled{manual}); the store keeps only a transient optimistic
// overlay while the server round-trip is in flight. RENAME_KEY is the
// retired localStorage layer — migrateRenames pushes any leftover entries
// to the server once, then removes the key.
const RENAME_KEY = "arwebui.renames";
function migrateRenames(storage: Storage, rename: (id: string, title: string) => Promise<unknown>) {
  let stale: Record<string, string> = {};
  try {
    const v = JSON.parse(storage.getItem(RENAME_KEY) || "{}");
    if (v && typeof v === "object") stale = v;
  } catch {
    /* corrupt: drop below */
  }
  const entries = Object.entries(stale).filter(([, t]) => typeof t === "string" && t.trim());
  if (!entries.length) {
    try {
      storage.removeItem(RENAME_KEY);
    } catch {
      /* ignore */
    }
    return;
  }
  Promise.allSettled(entries.map(([id, t]) => rename(id, t.trim()))).then((results) => {
    if (results.every((r) => r.status === "fulfilled")) {
      try {
        storage.removeItem(RENAME_KEY);
      } catch {
        /* ignore */
      }
    }
  });
}

const SIDEBAR_KEY = "arwebui.sidebar";
function loadSidebarCollapsed(storage: Storage): boolean {
  try {
    return storage.getItem(SIDEBAR_KEY) === "1";
  } catch {
    return false;
  }
}

export const SIDEBAR_MIN_WIDTH = 220;
export const SIDEBAR_MAX_WIDTH = 480;
export const SIDEBAR_DEFAULT_WIDTH = 320;
const SIDEBAR_WIDTH_KEY = "arwebui.sidebarWidth";

export function clampSidebarWidth(width: number): number {
  if (!Number.isFinite(width)) return SIDEBAR_DEFAULT_WIDTH;
  return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(width)));
}

function loadSidebarWidth(storage: Storage): number {
  try {
    const raw = Number(storage.getItem(SIDEBAR_WIDTH_KEY));
    return raw ? clampSidebarWidth(raw) : SIDEBAR_DEFAULT_WIDTH;
  } catch {
    return SIDEBAR_DEFAULT_WIDTH;
  }
}

const UNREAD_KEY = "arwebui.unread";
function loadUnread(storage: Storage): string[] {
  try {
    return JSON.parse(storage.getItem(UNREAD_KEY) || "[]");
  } catch {
    return [];
  }
}
function saveUnread(storage: Storage, ids: string[]) {
  try {
    storage.setItem(UNREAD_KEY, JSON.stringify(ids));
  } catch {
    /* ignore quota */
  }
}

// seenTurns tracks the last turn-count we've "seen" per session, so a later
// increase (while you're not viewing it) can flag the session unread. It's
// in-memory: rebuilt from the first fetch each load, so a reload never
// retroactively marks history unread — only the persisted unread set restores.
const firstSessionPageSize = 40;
const olderSessionPageSize = 80;

// Put `head` first and append only unseen rows from `tail`. Periodic refreshes
// replace the recent page while retaining already hydrated history; history
// pages append without duplicating rows when activity shifts page boundaries.
export function mergeSessionRows(head: Session[], tail: Session[]): Session[] {
  const seen = new Set(head.map((session) => session.id));
  return [...head, ...tail.filter((session) => !seen.has(session.id))];
}

export type AppStore = StoreApi<AppState>;

export interface CreateAppStoreOptions {
  migrateLegacyRenames?: boolean;
}

export function createAppStore(
  services: AppServices = createProductionAppServices(),
  options: CreateAppStoreOptions = {},
): AppStore {
  const localStorage = services.storage.local;
  const seenTurns: Record<string, number> = {};
  let sessionsRefreshInFlight: Promise<void> | null = null;

  const store = createStore<AppState>()((set, get) => ({
  health: null,
  sessions: [],
  sessionsReady: false,
  sessionsLoadingOlder: false,
  runs: [],
  currentSid: null,
  currentRunId: null,
  currentPage: "home",
  scheduledDetailSid: null,
  newSessionProject: null,
  newSessionForProject: (workspace) => {
    const normalized = workspace.trim().replace(/\/+$/, "");
    if (!normalized) return;
    set({
      currentSid: null,
      currentRunId: null,
      currentPage: "home",
      scheduledDetailSid: null,
      newSessionProject: {
        workspace: normalized,
        requestId: services.ids.next("new-session-project"),
      },
    });
    services.navigation.setHash("");
  },
  consumeNewSessionProject: (requestId) => {
    if (get().newSessionProject?.requestId === requestId) set({ newSessionProject: null });
  },
  modal: null,
  prompt: null,
  toasts: [],
  showSys: false,
  toggleSys: () => set({ showSys: !get().showSys }),
  theme: services.theme.load(),
  cycleTheme: () => {
    const t = nextTheme(get().theme);
    services.theme.save(t);
    set({ theme: t });
  },
  helpOpen: false,
  openHelp: () => set({ helpOpen: true }),
  closeHelp: () => set({ helpOpen: false }),
  archived: loadArchived(localStorage),
  showArchived: false,
  toggleArchive: (id) => {
    const cur = get().archived;
    const restoring = cur.includes(id);
    const next = restoring ? cur.filter((x) => x !== id) : [...cur, id];
    try {
      localStorage.setItem(ARCHIVE_KEY, JSON.stringify(next));
    } catch {
      /* ignore quota */
    }
    set({ archived: next });
    // A schedule detail route is still the archived session's current
    // destination even though currentSid is deliberately null. Close only the
    // detail pane and stay on the Scheduled hub; Back must not resurrect a
    // route whose row is now hidden.
    if (!restoring && get().scheduledDetailSid === id) {
      set({
        currentSid: null,
        currentRunId: null,
        currentPage: "scheduled",
        scheduledDetailSid: null,
        toasts: [],
      });
      services.navigation.replaceHash("scheduled");
      return;
    }
    // Replace archive-current history so Back cannot reopen the hidden session.
    if (!restoring && get().currentSid === id) {
      set({ currentSid: null, currentRunId: null, currentPage: "home", toasts: [] });
      services.navigation.replaceHash("");
    }
  },
  toggleShowArchived: () => set({ showArchived: !get().showArchived }),
  pinned: loadPinned(localStorage),
  togglePin: (id) => {
    const cur = get().pinned;
    const next = cur.includes(id) ? cur.filter((x) => x !== id) : [id, ...cur];
    try {
      localStorage.setItem(PIN_KEY, JSON.stringify(next));
    } catch {
      /* ignore quota */
    }
    set({ pinned: next });
  },
  renames: {},
  setRename: (id, title) => {
    const t = title.trim();
    if (!t) return; // journal titles don't "clear"; blank input is a no-op
    // Optimistic overlay now; the journal fact arrives with the next
    // sessions refresh (s.title), at which point the overlay is redundant.
    set({ renames: { ...get().renames, [id]: t } });
    services.api
      .rename(id, t)
      .then(() => get().refreshSessions())
      .catch(() => {
        const next = { ...get().renames };
        delete next[id];
        set({ renames: next });
      });
  },
  sidebarCollapsed: loadSidebarCollapsed(localStorage),
  toggleSidebar: () => {
    const next = !get().sidebarCollapsed;
    try {
      localStorage.setItem(SIDEBAR_KEY, next ? "1" : "0");
    } catch {
      /* ignore quota */
    }
    set({ sidebarCollapsed: next });
  },
  sidebarWidth: loadSidebarWidth(localStorage),
  setSidebarWidth: (width) => {
    const next = clampSidebarWidth(width);
    try {
      localStorage.setItem(SIDEBAR_WIDTH_KEY, String(next));
    } catch {
      /* private mode / quota */
    }
    set({ sidebarWidth: next });
  },
  unread: loadUnread(localStorage),
  markUnread: (id) => {
    if (get().unread.includes(id)) return;
    const next = [...get().unread, id];
    saveUnread(localStorage, next);
    set({ unread: next });
  },
  markRead: (id) => {
    // Record the current turn count as seen so it won't re-flag, and drop the dot.
    const s = get().sessions.find((x) => x.id === id);
    if (s) seenTurns[id] = s.turns;
    if (!get().unread.includes(id)) return;
    const next = get().unread.filter((x) => x !== id);
    saveUnread(localStorage, next);
    set({ unread: next });
  },

  projects: {},
  refreshProjects: async () => {
    try {
      set({ projects: await services.api.projects() });
    } catch {
      /* overlay is cosmetic; a failed fetch leaves the last-known map */
    }
  },
  setProjectName: async (key, name) => {
    try {
      set({ projects: await services.api.updateProject(key, { displayName: name }) });
    } catch (error: any) {
      get().toast(error.message, "error", error.details);
    }
  },
  toggleProjectFolded: async (key, folded) => {
    // Optimistic: fold/unfold must feel instant. Reconcile with the server's
    // authoritative map on success; roll back to a refetch on failure.
    const prev = get().projects;
    set({ projects: { ...prev, [key]: { ...prev[key], folded } } });
    try {
      set({ projects: await services.api.updateProject(key, { folded }) });
    } catch {
      get().refreshProjects();
    }
  },
  toggleProjectPinned: async (key, pinned) => {
    const prev = get().projects;
    set({ projects: { ...prev, [key]: { ...prev[key], pinned } } });
    try {
      set({ projects: await services.api.updateProject(key, { pinned }) });
    } catch (error: any) {
      set({ projects: prev });
      get().toast(error.message, "error", error.details);
    }
  },
  setProjectRemoved: async (key, removed) => {
    const prev = get().projects;
    set({ projects: { ...prev, [key]: { ...prev[key], removed } } });
    try {
      set({ projects: await services.api.updateProject(key, { removed }) });
    } catch (error: any) {
      set({ projects: prev });
      get().toast(error.message, "error", error.details);
    }
  },
  openProjectIn: async (workspace, app) => {
    try {
      await services.api.openIn(workspace, app);
      get().refreshProjects(); // pick up the new last_opened
    } catch (error: any) {
      get().toast(error.message, "error", error.details);
    }
  },

  diffFocusPath: null,
  focusDiffFile: (path) => set({ diffFocusPath: path }),
  clearDiffFocus: () => {
    if (get().diffFocusPath !== null) set({ diffFocusPath: null });
  },

  workspaceEpoch: 0,
  bumpWorkspaceEpoch: () => set((s) => ({ workspaceEpoch: s.workspaceEpoch + 1 })),

  refreshHealth: async () => {
    try {
      set({ health: await services.api.health() });
    } catch {
      set({ health: null });
    }
  },
  refreshSessions: () => {
    if (sessionsRefreshInFlight) return sessionsRefreshInFlight;
    const initialHydration = !get().sessionsReady;
    const applyPage = (page: Session[], append: boolean) => {
      const prev = get().sessions;
      const next = append ? mergeSessionRows(prev, page) : mergeSessionRows(page, prev);
      if (prev.length) services.notifications.sessions(prev, next, get().currentSid);
      // Flag sessions that gained turns since we last saw them (and aren't the
      // one you're viewing). First sighting only records a baseline.
      const cur = get().currentSid;
      const unreadSet = new Set(get().unread);
      let changed = false;
      for (const s of page) {
        const seen = seenTurns[s.id];
        if (s.id === cur) {
          if (unreadSet.delete(s.id)) changed = true;
        } else if (seen !== undefined && s.turns > seen && !unreadSet.has(s.id)) {
          unreadSet.add(s.id);
          changed = true;
        }
        seenTurns[s.id] = s.turns;
      }
      if (changed) {
        const arr = [...unreadSet];
        saveUnread(localStorage, arr);
        set({ unread: arr });
      }
      set({ sessions: next, sessionsReady: true });
    };
    const session = (async () => {
      try {
        const recent = await services.api.sessions(firstSessionPageSize, 0);
        applyPage(recent, false);
        if (!initialHydration || recent.length < firstSessionPageSize) return;

        set({ sessionsLoadingOlder: true });
        let offset = recent.length;
        for (;;) {
          const page = await services.api.sessions(olderSessionPageSize, offset);
          if (page.length === 0) break;
          applyPage(page, true);
          offset += page.length;
          if (page.length < olderSessionPageSize) break;
        }
      } catch {
        /* health indicator carries a first-page failure; hydrated rows stay */
      } finally {
        set({ sessionsLoadingOlder: false });
      }
    })();
    sessionsRefreshInFlight = session.finally(() => {
      sessionsRefreshInFlight = null;
    });
    return sessionsRefreshInFlight;
  },
  refreshRuns: async () => {
    try {
      const next = await services.api.runs();
      const prev = get().runs;
      if (prev.length) services.notifications.runs(prev, next);
      set({ runs: next });
    } catch {
      /* ignore */
    }
  },
  visibleOrder: [],
  setVisibleOrder: (ids) => {
    const cur = get().visibleOrder;
    if (cur.length === ids.length && cur.every((x, i) => x === ids[i])) return;
    set({ visibleOrder: ids });
  },
  selectAdjacent: (delta) => {
    const order = get().visibleOrder;
    if (!order.length) return;
    const cur = get().currentSid;
    const idx = cur ? order.indexOf(cur) : -1;
    if (idx === -1) {
      get().select(order[0]);
      return;
    }
    const next = (idx + delta + order.length) % order.length;
    get().select(order[next]);
  },
  select: (sid) => {
    set({ currentSid: sid, currentRunId: null, currentPage: "home", scheduledDetailSid: null, toasts: [] });
    if (sid) {
      services.navigation.setHash(sid);
      get().markRead(sid); // opening a session clears its unread flag
    } else {
      services.navigation.setHash("");
    }
  },
  selectRun: (rid) => {
    set({ currentRunId: rid, currentSid: null, currentPage: "scheduled", scheduledDetailSid: null, toasts: [] });
    services.navigation.setHash(rid ? "run:" + rid : "");
  },
  showPage: (page) => {
    set({ currentSid: null, currentRunId: null, currentPage: page, scheduledDetailSid: null, toasts: [] });
    // "home" is the bare route (no hash); Scheduled routes to a hash that
    // matches its key so deep links + back/forward work (#scheduled).
    services.navigation.setHash(page === "home" ? "" : page);
  },
  showScheduledDetail: (sid) => {
    set({
      currentSid: null,
      currentRunId: null,
      currentPage: "scheduled",
      scheduledDetailSid: sid,
      toasts: [],
    });
    services.navigation.setHash(sid ? `scheduled:${sid}` : "scheduled");
  },
  openModal: (m) => set({ modal: m }),
  openPrompt: (p) => set({ prompt: p }),
  toast: (text, kind = "error", details) => {
    const id = services.ids.next("toast");
    set({ toasts: [...get().toasts, { id, text, kind, details }] });
    // Errors persist until tapped — a long message on a phone must not vanish
    // before it can be read (phone report). Info toasts still auto-dismiss.
    if (kind !== "error") services.clock.setTimeout(() => get().dismissToast(id), 5000);
  },
  dismissToast: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),
  }));

  if (options.migrateLegacyRenames) {
    migrateRenames(localStorage, (id, title) => services.api.rename(id, title));
  }

  return store;
}

// One-shot migration of the retired localStorage rename layer (PLAN 5.6).
export const appStore = createAppStore(productionAppServices, {
  migrateLegacyRenames: true,
});

const AppStoreContext = createContext<AppStore | null>(null);

export interface AppStoreProviderProps {
  store: AppStore;
  children: ReactNode;
}

export function AppStoreProvider({ store, children }: AppStoreProviderProps) {
  return createElement(AppStoreContext.Provider, { value: store }, children);
}

export function useAppStoreApi(): AppStore {
  return useContext(AppStoreContext) ?? appStore;
}

function useStoreFromContext(): AppState;
function useStoreFromContext<T>(selector: (state: AppState) => T): T;
function useStoreFromContext<T>(selector?: (state: AppState) => T): T | AppState {
  const store = useAppStoreApi();
  if (selector) return useZustandStore(store, selector);
  return useZustandStore(store);
}

// Keep the existing production/test surface (`useStore(...)`,
// `useStore.getState()`, `useStore.setState()`) while allowing Storybook and
// Demo harnesses to provide fully isolated stores through AppStoreProvider.
export const useStore = Object.assign(useStoreFromContext, {
  setState: appStore.setState,
  getState: appStore.getState,
  getInitialState: appStore.getInitialState,
  subscribe: appStore.subscribe,
}) as UseBoundStore<AppStore>;
