// Journal envelope as emitted by `ar events --json`. Payload shapes vary by
// type; we keep it loose and narrow per-case in the timeline builder.
export interface Envelope {
  seq: number;
  type: string;
  payload?: any;
  ts?: string; // RFC3339 event time, recorded by the daemon
  // Durable-command receipt (journal `command_id`). The timeline uses the
  // retry lineage encoded here ("retry:<orig-id>") to collapse a failed,
  // retried block into one row (INC-84 UX).
  command_id?: string;
}

// The cadence contract every scheduled thing carries (CX-3): what rhythm it
// runs on, and when it fires next. Both are derived server-side from the driver
// spec. Absent = not knowable (a one-shot goal, a finished series, a spec we
// could not read) — render the absence honestly, never a guessed time.
export interface Cadence {
  // immediate | interval | cron | self_paced | parallel
  schedule?: string;
  // Human phrase: "Every 30m", "Saturdays at 4:00 AM", "Best of 4", "Runs once".
  cadence?: string;
  // RFC3339 instant of the next tick; only present for a LIVE interval/cron series.
  nextRunAt?: string;
}

export interface Session extends Cadence {
  id: string;
  status: string;
  turns: number;
  attention?: {
    approvals?: number;
    answers?: number;
  };
  // RFC3339 journal mtime: the durable source for sidebar activity recency.
  // Legacy/older backends may omit it; clients fall back to the id stamp.
  updatedAt?: string;
  title?: string;
  workspace?: string;
  kind?: "session" | "driver";
}

export interface DiffResp {
  scope?: "working-tree" | "last-turn";
  // Last turn is a durable capability, not a guessed empty diff. Historical
  // sessions without a usable barrier return available:false + reason.
  available?: boolean;
  reason?: string;
  input_seq?: number;
  barrier_seq?: number;
  barrier_id?: string;
  workspace: string;
  known: boolean;
  isRepo: boolean;
  // The workspace sits INSIDE another repository (repoRoot) instead of being
  // a repo of its own — git would diff the parent there, so no diff is shown.
  nested?: boolean;
  repoRoot?: string;
  // The workspace is a LINKED git worktree of mainRepo, checked out on `branch`
  // ("" when detached) — enables the Apply-back / Remove controls (INC-49).
  worktree?: boolean;
  mainRepo?: string;
  branch?: string;
  diff: string;
  numstat: string;
  untracked: string[];
  untrackedReasons?: Record<string, "binary" | "large" | "unavailable">;
  hiddenUntracked?: number;
  conflicts?: string[];
}

export type DiffScope = "working-tree" | "last-turn";

export interface Health {
  version: string;
  daemonUp: boolean;
  daemonManaged: boolean;
  daemonExternal: boolean;
  manageRequested: boolean;
  daemonLogPath: string;
  runtimeDir: string;
  sandboxBackend?: string;
  sandboxDetected?: boolean;
}

export interface BackgroundWork {
  handle: string;
  tool: string;
  detail: string;
}

export interface Run extends Cadence {
  id: string;
  kind: "submit" | "drive";
  label: string;
  workspace: string;
  // sessionId is the daemon-assigned session the run created (once known).
  // A drive run's SESSION is the canonical user-facing object (INC-80.3) —
  // surfaces prefer it over the transient run row.
  sessionId?: string;
  status: "running" | "done" | "failed" | "stopped";
  startedAt: string;
}

export interface SpecFile {
  name: string;
  content: string;
}

// ProjectMeta is the server-side, workspace-keyed overlay (INC-53, HANDA #24):
// a user's cosmetic preferences layered on top of the journal-derived project
// groups — a custom display name, a folded (collapsed) state, and when the
// project was last opened in a system app via the launcher. Decorative only;
// it never decides which group a session belongs to.
export interface ProjectMeta {
  displayName?: string;
  folded?: boolean;
  pinned?: boolean;
  // Sidebar-only removal preference. Sessions/journals/workspace remain intact
  // and continue to be reachable from search; the rail exposes Restore.
  removed?: boolean;
  lastOpened?: number; // unix millis; absent = never opened via the launcher
}

// LauncherApp is the whitelisted set of system apps /api/open can launch. The
// backend maps each token to a fixed argv per OS — never the raw string.
export type LauncherApp = "vscode" | "finder" | "terminal";
