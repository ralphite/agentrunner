// Journal envelope as emitted by `ar events --json`. Payload shapes vary by
// type; we keep it loose and narrow per-case in the timeline builder.
export interface Envelope {
  seq: number;
  type: string;
  payload?: any;
  ts?: string; // RFC3339 event time, recorded by the daemon
}

export interface Session {
  id: string;
  status: string;
  turns: number;
  title?: string;
  workspace?: string;
}

export interface DiffResp {
  workspace: string;
  known: boolean;
  isRepo: boolean;
  // The workspace sits INSIDE another repository (repoRoot) instead of being
  // a repo of its own — git would diff the parent there, so no diff is shown.
  nested?: boolean;
  repoRoot?: string;
  diff: string;
  numstat: string;
  untracked: string[];
}

export interface Health {
  version: string;
  daemonUp: boolean;
  daemonManaged: boolean;
  daemonExternal: boolean;
  manageRequested: boolean;
  daemonLogPath: string;
  runtimeDir: string;
}

export interface Task {
  handle: string;
  tool: string;
  detail: string;
}

export interface Run {
  id: string;
  kind: "submit" | "drive";
  label: string;
  workspace: string;
  status: "running" | "done" | "failed" | "stopped";
  startedAt: string;
}

export interface SpecFile {
  name: string;
  content: string;
}
