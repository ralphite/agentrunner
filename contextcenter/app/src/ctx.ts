import { createContext, useContext } from "react";
import type { DocStatus, TaskStatus } from "./app-data";

export interface Statuses {
  "task-43": TaskStatus;
  "bug-142": TaskStatus;
  "session-recovery": DocStatus;
  "impl-plan": DocStatus;
}

export interface AppCtxValue {
  doc: string;
  openDoc: (id: string) => void;
  statuses: Statuses;
  setStatus: (key: keyof Statuses, value: TaskStatus | DocStatus) => void;
  agent: string;
  setAgent: (a: string) => void;
}

export const AppCtx = createContext<AppCtxValue>(null as unknown as AppCtxValue);
export const useApp = () => useContext(AppCtx);

export const taskPill: Record<TaskStatus, { cls: string; label: string }> = {
  Todo: { cls: "todo", label: "Todo" },
  "In progress": { cls: "prog", label: "In progress" },
  Done: { cls: "done", label: "Done" },
  Blocked: { cls: "failed", label: "Blocked" },
};
