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
  // True only for the canonical merged-stream repeating series whose daemon
  // implements durable pause/resume. Legacy driver journals omit it.
  scheduleControl?: boolean;
  // True for any canonical merged series, including terminal history. It
  // gates the safe typed detail endpoint; legacy driver streams omit it.
  scheduleDetail?: boolean;
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
  // Full multi-root boundary (INC-105), primary first; absent = single-root.
  workspaceRoots?: string[];
  kind?: "session" | "driver";
}

// Safe, typed read model for a scheduled series. The backend intentionally
// excludes the raw driver/agent spec (system prompt, permissions and tool
// configuration); this is the complete browser-visible contract.
export interface ScheduleDetail extends Cadence {
  kind: "series" | "session";
  sessionId: string;
  name?: string;
  status: string;
  prompt?: string;
  workspace?: string;
  agent?: string;
  provider?: string;
  model?: string;
  thinkingEnabled?: boolean;
  thinkingBudgetTokens?: number;
  interval?: string;
  cron?: string;
  paceMin?: string;
  paceMax?: string;
  overlap?: string;
  iterations: number;
  maxIterations?: number;
  scheduleEdit?: boolean;
  revision: number;
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
  // Multi-root sessions (INC-105): one probe per workspace root, primary
  // first. The top-level fields above remain the primary's — old consumers
  // keep working; roots is the full picture.
  roots?: DiffRootResp[];
}

// DiffRootResp is one root's working-tree probe within a multi-root session
// (INC-105) — the same shape the top level carries for the primary.
export interface DiffRootResp {
  root: string;
  isRepo: boolean;
  nested?: boolean;
  repoRoot?: string;
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

// Effective runtime Agent catalog. `yaml` is the editable behavior definition;
// model selection is intentionally absent and travels separately on requests.
export interface AgentCatalogEntry {
  name: string;
  description?: string;
  source: "shipped" | "user";
  yaml: string;
}

// ProjectDef is one entry of the explicit project registry (INC-104): a
// user-declared name plus one or more source folders the sidebar merges into a
// single group. Membership stays derived — a session belongs to the project
// iff its workspace exactly matches one of the folders; sessions never carry a
// project id.
export interface ProjectDef {
  id: string;
  name: string;
  folders: string[]; // folders[0] is the primary (New chat target)
  order?: number; // manual sort rank; absent/0 = unranked
  createdAt?: number; // unix millis
  missing?: string[]; // folders currently absent from disk (computed server-side)
}

// ProjectsPayload is the combined GET/POST /api/projects* response: the
// cosmetic overlay map plus the explicit registry, refreshed by one poll.
export interface ProjectsPayload {
  overlays: Record<string, ProjectMeta>;
  projects: ProjectDef[];
}

// ProjectMeta is the server-side cosmetic overlay (INC-53, HANDA #24). Keys
// are derived-group workspace paths, or "project:<id>" for an explicit
// project's presentation state (INC-104) — a custom display name, a folded
// (collapsed) state, and when the project was last opened in a system app via
// the launcher. Decorative only; it never decides which group a session
// belongs to.
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
