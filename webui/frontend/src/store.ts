import { create } from "zustand";
import { AR } from "./api";
import type { Health, Run, Session } from "./types";
import { notifyRunChanges, notifySessionChanges } from "./notify";
import { loadTheme, nextTheme, saveTheme, type Theme } from "./theme";

export type ModalKind =
  | { kind: "new"; message?: string }
  | { kind: "run"; task?: string } // submit / drive launcher
  | { kind: "fork"; sid: string }
  | { kind: "agent"; sid: string }
  | { kind: "rename"; sid: string }
  | { kind: "trust" }
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
  onSubmit: (value: string) => void;
}

interface ToastMsg {
  id: number;
  text: string;
  kind: "error" | "info";
}

interface AppState {
  health: Health | null;
  sessions: Session[];
  runs: Run[];
  currentSid: string | null;
  currentRunId: string | null;
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
  unread: string[]; // sids with new activity you haven't opened (localStorage-backed)
  markUnread: (id: string) => void;
  markRead: (id: string) => void;

  visibleOrder: string[]; // sidebar's flat session order, for keyboard nav
  setVisibleOrder: (ids: string[]) => void;
  selectAdjacent: (delta: number) => void;

  refreshHealth: () => Promise<void>;
  refreshSessions: () => Promise<void>;
  refreshRuns: () => Promise<void>;
  select: (sid: string | null) => void;
  selectRun: (rid: string | null) => void;
  openModal: (m: ModalKind) => void;
  openPrompt: (p: PromptState | null) => void;
  toast: (text: string, kind?: "error" | "info") => void;
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

const RENAME_KEY = "arwebui.renames";
function loadRenames(): Record<string, string> {
  try {
    const v = JSON.parse(localStorage.getItem(RENAME_KEY) || "{}");
    return v && typeof v === "object" ? v : {};
  } catch {
    return {};
  }
}

const SIDEBAR_KEY = "arwebui.sidebar";
function loadSidebarCollapsed(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_KEY) === "1";
  } catch {
    return false;
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

export const useStore = create<AppState>((set, get) => ({
  health: null,
  sessions: [],
  runs: [],
  currentSid: null,
  currentRunId: null,
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
  renames: loadRenames(),
  setRename: (id, title) => {
    const next = { ...get().renames };
    const t = title.trim();
    if (t) next[id] = t;
    else delete next[id]; // empty title clears the rename (revert to derived)
    try {
      localStorage.setItem(RENAME_KEY, JSON.stringify(next));
    } catch {
      /* ignore quota */
    }
    set({ renames: next });
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

  refreshHealth: async () => {
    try {
      set({ health: await AR.health() });
    } catch {
      set({ health: null });
    }
  },
  refreshSessions: async () => {
    try {
      const next = await AR.sessions();
      const prev = get().sessions;
      if (prev.length) notifySessionChanges(prev, next, get().currentSid);
      // Flag sessions that gained turns since we last saw them (and aren't the
      // one you're viewing). First sighting only records a baseline.
      const cur = get().currentSid;
      const unreadSet = new Set(get().unread);
      let changed = false;
      for (const s of next) {
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
      set({ sessions: next });
    } catch {
      /* health indicator carries the failure */
    }
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
    set({ currentSid: sid, currentRunId: null });
    if (sid) {
      location.hash = sid;
      get().markRead(sid); // opening a task clears its unread flag
    } else {
      location.hash = "";
    }
  },
  selectRun: (rid) => {
    set({ currentRunId: rid, currentSid: null });
    location.hash = rid ? "run:" + rid : "";
  },
  openModal: (m) => set({ modal: m }),
  openPrompt: (p) => set({ prompt: p }),
  toast: (text, kind = "error") => {
    const id = ++toastSeq;
    set({ toasts: [...get().toasts, { id, text, kind }] });
    setTimeout(() => get().dismissToast(id), 7000);
  },
  dismissToast: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),
}));
