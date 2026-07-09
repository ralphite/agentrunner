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

  refreshHealth: () => Promise<void>;
  refreshSessions: () => Promise<void>;
  refreshRuns: () => Promise<void>;
  select: (sid: string | null) => void;
  selectRun: (rid: string | null) => void;
  openModal: (m: ModalKind) => void;
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

export const useStore = create<AppState>((set, get) => ({
  health: null,
  sessions: [],
  runs: [],
  currentSid: null,
  currentRunId: null,
  modal: null,
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
  select: (sid) => {
    set({ currentSid: sid, currentRunId: null });
    if (sid) location.hash = sid;
    else location.hash = "";
  },
  selectRun: (rid) => {
    set({ currentRunId: rid, currentSid: null });
    location.hash = rid ? "run:" + rid : "";
  },
  openModal: (m) => set({ modal: m }),
  toast: (text, kind = "error") => {
    const id = ++toastSeq;
    set({ toasts: [...get().toasts, { id, text, kind }] });
    setTimeout(() => get().dismissToast(id), 7000);
  },
  dismissToast: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),
}));
