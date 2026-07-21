import { create } from "zustand";
import { AR } from "./api";
import type { Health, LauncherApp, ProjectMeta, Run, Session } from "./types";
import { notifyRunChanges, notifySessionChanges } from "./notify";
import { loadTheme, nextTheme, saveTheme, type Theme } from "./theme";
import type { CadenceSpec, RunPreset } from "./runPreset";

export type ModalKind =
  | { kind: "new"; message?: string }
  // submit / drive launcher. `cadence` (SC-18) is the RHYTHM the caller already
  // showed the user — a Scheduled suggestion card's "Weekdays at 8:00 AM" — as
  // the driver-spec fields themselves. The launcher opens on it instead of the
  // preset's generic default, so what gets built is what was clicked.
  | { kind: "run"; prompt?: string; preset?: RunPreset; cadence?: CadenceSpec }
  | { kind: "fork"; sid: string }
  | { kind: "agent"; sid: string }
  | { kind: "rename"; sid: string }
  | { kind: "trust" }
  | { kind: "confirm"; title: string; body: string; confirmLabel: string; danger?: boolean; onConfirm: () => void | Promise<void> }
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

interface AppState {
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
  openModal: (m: ModalKind) => void;
  openPrompt: (p: PromptState | null) => void;
  toast: (text: string, kind?: "error" | "info", details?: string) => void;
  dismissToast: (id: number) => void;
}

let toastSeq = 0;

const ARCHIVE_KEY = "arwebui.archived";
function loadArchived(): string[] {
  try {
    return JSON.parse(localStorage.getItem(ARCHIVE_KEY) || "[]");
  } catch {
    return [];
  }
}

const PIN_KEY = "arwebui.pinned";
function loadPinned(): string[] {
  try {
    return JSON.parse(localStorage.getItem(PIN_KEY) || "[]");
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
function migrateRenames(rename: (id: string, title: string) => Promise<unknown>) {
  let stale: Record<string, string> = {};
  try {
    const v = JSON.parse(localStorage.getItem(RENAME_KEY) || "{}");
    if (v && typeof v === "object") stale = v;
  } catch {
    /* corrupt: drop below */
  }
  const entries = Object.entries(stale).filter(([, t]) => typeof t === "string" && t.trim());
  if (!entries.length) {
    try {
      localStorage.removeItem(RENAME_KEY);
    } catch {
      /* ignore */
    }
    return;
  }
  Promise.allSettled(entries.map(([id, t]) => rename(id, t.trim()))).then((results) => {
    if (results.every((r) => r.status === "fulfilled")) {
      try {
        localStorage.removeItem(RENAME_KEY);
      } catch {
        /* ignore */
      }
    }
  });
}

const SIDEBAR_KEY = "arwebui.sidebar";
function loadSidebarCollapsed(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_KEY) === "1";
  } catch {
    return false;
  }
}

export const SIDEBAR_MIN_WIDTH = 220;
export const SIDEBAR_MAX_WIDTH = 480;
export const SIDEBAR_DEFAULT_WIDTH = 260;
const SIDEBAR_WIDTH_KEY = "arwebui.sidebarWidth";

export function clampSidebarWidth(width: number): number {
  if (!Number.isFinite(width)) return SIDEBAR_DEFAULT_WIDTH;
  return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(width)));
}

function loadSidebarWidth(): number {
  try {
    const raw = Number(localStorage.getItem(SIDEBAR_WIDTH_KEY));
    return raw ? clampSidebarWidth(raw) : SIDEBAR_DEFAULT_WIDTH;
  } catch {
    return SIDEBAR_DEFAULT_WIDTH;
  }
}

const UNREAD_KEY = "arwebui.unread";
function loadUnread(): string[] {
  try {
    return JSON.parse(localStorage.getItem(UNREAD_KEY) || "[]");
  } catch {
    return [];
  }
}
function saveUnread(ids: string[]) {
  try {
    localStorage.setItem(UNREAD_KEY, JSON.stringify(ids));
  } catch {
    /* ignore quota */
  }
}

// seenTurns tracks the last turn-count we've "seen" per session, so a later
// increase (while you're not viewing it) can flag the session unread. It's
// in-memory: rebuilt from the first fetch each load, so a reload never
// retroactively marks history unread — only the persisted unread set restores.
const seenTurns: Record<string, number> = {};

const firstSessionPageSize = 40;
const olderSessionPageSize = 80;
let sessionsRefreshInFlight: Promise<void> | null = null;

// Put `head` first and append only unseen rows from `tail`. Periodic refreshes
// replace the recent page while retaining already hydrated history; history
// pages append without duplicating rows when activity shifts page boundaries.
export function mergeSessionRows(head: Session[], tail: Session[]): Session[] {
  const seen = new Set(head.map((session) => session.id));
  return [...head, ...tail.filter((session) => !seen.has(session.id))];
}

export const useStore = create<AppState>((set, get) => ({
  health: null,
  sessions: [],
  sessionsReady: false,
  sessionsLoadingOlder: false,
  runs: [],
  currentSid: null,
  currentRunId: null,
  currentPage: "home",
  modal: null,
  prompt: null,
  toasts: [],
  showSys: false,
  toggleSys: () => set({ showSys: !get().showSys }),
  theme: loadTheme(),
  cycleTheme: () => {
    const t = nextTheme(get().theme);
    saveTheme(t);
    set({ theme: t });
  },
  helpOpen: false,
  openHelp: () => set({ helpOpen: true }),
  closeHelp: () => set({ helpOpen: false }),
  archived: loadArchived(),
  showArchived: false,
  toggleArchive: (id) => {
    const cur = get().archived;
    const next = cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id];
    try {
      localStorage.setItem(ARCHIVE_KEY, JSON.stringify(next));
    } catch {
      /* ignore quota */
    }
    set({ archived: next });
  },
  toggleShowArchived: () => set({ showArchived: !get().showArchived }),
  pinned: loadPinned(),
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
    AR.rename(id, t)
      .then(() => get().refreshSessions())
      .catch(() => {
        const next = { ...get().renames };
        delete next[id];
        set({ renames: next });
      });
  },
  sidebarCollapsed: loadSidebarCollapsed(),
  toggleSidebar: () => {
    const next = !get().sidebarCollapsed;
    try {
      localStorage.setItem(SIDEBAR_KEY, next ? "1" : "0");
    } catch {
      /* ignore quota */
    }
    set({ sidebarCollapsed: next });
  },
  sidebarWidth: loadSidebarWidth(),
  setSidebarWidth: (width) => {
    const next = clampSidebarWidth(width);
    try {
      localStorage.setItem(SIDEBAR_WIDTH_KEY, String(next));
    } catch {
      /* private mode / quota */
    }
    set({ sidebarWidth: next });
  },
  unread: loadUnread(),
  markUnread: (id) => {
    if (get().unread.includes(id)) return;
    const next = [...get().unread, id];
    saveUnread(next);
    set({ unread: next });
  },
  markRead: (id) => {
    // Record the current turn count as seen so it won't re-flag, and drop the dot.
    const s = get().sessions.find((x) => x.id === id);
    if (s) seenTurns[id] = s.turns;
    if (!get().unread.includes(id)) return;
    const next = get().unread.filter((x) => x !== id);
    saveUnread(next);
    set({ unread: next });
  },

  projects: {},
  refreshProjects: async () => {
    try {
      set({ projects: await AR.projects() });
    } catch {
      /* overlay is cosmetic; a failed fetch leaves the last-known map */
    }
  },
  setProjectName: async (key, name) => {
    try {
      set({ projects: await AR.updateProject(key, { displayName: name }) });
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
      set({ projects: await AR.updateProject(key, { folded }) });
    } catch {
      get().refreshProjects();
    }
  },
  toggleProjectPinned: async (key, pinned) => {
    const prev = get().projects;
    set({ projects: { ...prev, [key]: { ...prev[key], pinned } } });
    try {
      set({ projects: await AR.updateProject(key, { pinned }) });
    } catch (error: any) {
      set({ projects: prev });
      get().toast(error.message, "error", error.details);
    }
  },
  setProjectRemoved: async (key, removed) => {
    const prev = get().projects;
    set({ projects: { ...prev, [key]: { ...prev[key], removed } } });
    try {
      set({ projects: await AR.updateProject(key, { removed }) });
    } catch (error: any) {
      set({ projects: prev });
      get().toast(error.message, "error", error.details);
    }
  },
  openProjectIn: async (workspace, app) => {
    try {
      await AR.openIn(workspace, app);
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
      set({ health: await AR.health() });
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
      if (prev.length) notifySessionChanges(prev, next, get().currentSid);
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
        saveUnread(arr);
        set({ unread: arr });
      }
      set({ sessions: next, sessionsReady: true });
    };
    const session = (async () => {
      try {
        const recent = await AR.sessions(firstSessionPageSize, 0);
        applyPage(recent, false);
        if (!initialHydration || recent.length < firstSessionPageSize) return;

        set({ sessionsLoadingOlder: true });
        let offset = recent.length;
        for (;;) {
          const page = await AR.sessions(olderSessionPageSize, offset);
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
      const next = await AR.runs();
      const prev = get().runs;
      if (prev.length) notifyRunChanges(prev, next);
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
    set({ currentSid: sid, currentRunId: null, currentPage: "home" });
    if (sid) {
      location.hash = sid;
      get().markRead(sid); // opening a session clears its unread flag
    } else {
      location.hash = "";
    }
  },
  selectRun: (rid) => {
    set({ currentRunId: rid, currentSid: null, currentPage: "scheduled" });
    location.hash = rid ? "run:" + rid : "";
  },
  showPage: (page) => {
    set({ currentSid: null, currentRunId: null, currentPage: page });
    // "home" is the bare route (no hash); Scheduled routes to a hash that
    // matches its key so deep links + back/forward work (#scheduled).
    location.hash = page === "home" ? "" : page;
  },
  openModal: (m) => set({ modal: m }),
  openPrompt: (p) => set({ prompt: p }),
  toast: (text, kind = "error", details) => {
    const id = ++toastSeq;
    set({ toasts: [...get().toasts, { id, text, kind, details }] });
    // Errors persist until tapped — a long message on a phone must not vanish
    // before it can be read (phone report). Info toasts still auto-dismiss.
    if (kind !== "error") setTimeout(() => get().dismissToast(id), 5000);
  },
  dismissToast: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),
}));

// One-shot migration of the retired localStorage rename layer (PLAN 5.6).
migrateRenames((id, t) => AR.rename(id, t));
