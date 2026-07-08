import { create } from "zustand";
import { AR } from "./api";
import type { Health, Run, Session } from "./types";

export type ModalKind =
  | { kind: "new"; message?: string }
  | { kind: "run"; task?: string } // submit / drive launcher
  | { kind: "fork"; sid: string }
  | { kind: "agent"; sid: string }
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

  refreshHealth: async () => {
    try {
      set({ health: await AR.health() });
    } catch {
      set({ health: null });
    }
  },
  refreshSessions: async () => {
    try {
      set({ sessions: await AR.sessions() });
    } catch {
      /* health indicator carries the failure */
    }
  },
  refreshRuns: async () => {
    try {
      set({ runs: await AR.runs() });
    } catch {
      /* ignore */
    }
  },
  select: (sid) => {
    set({ currentSid: sid, currentRunId: null });
    if (sid) location.hash = sid;
    else location.hash = "";
  },
  selectRun: (rid) => set({ currentRunId: rid, currentSid: null }),
  openModal: (m) => set({ modal: m }),
  toast: (text, kind = "error") => {
    const id = ++toastSeq;
    set({ toasts: [...get().toasts, { id, text, kind }] });
    setTimeout(() => get().dismissToast(id), 7000);
  },
  dismissToast: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),
}));
