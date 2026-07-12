import type { DiffResp, DiffScope, Envelope, Health, LauncherApp, ProjectMeta, Run, Session, SpecFile, Task } from "./types";

// ApiError carries the HTTP status and the server's machine-readable `code`
// (e.g. 404 / "session_not_found") next to the human message, so callers branch
// on semantics instead of grepping the CLI prose inside `stderr` — that prose is
// a display detail and may be re-worded at any time (INC-41 L5). The message is
// unchanged from a plain Error, so every toast reads exactly as before.
export class ApiError extends Error {
  status: number;
  code?: string;
  details?: string; // raw stderr — kept for a "Details" disclosure, NOT the toast
  constructor(message: string, status: number, code?: string, details?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

// api wraps the arwebui JSON contract. A non-2xx carries {error, stderr, code?};
// we surface all of it so the cockpit shows the real ar failure (never swallow it).
async function api<T = any>(path: string, opts?: RequestInit): Promise<T> {
  const r = await fetch("/api" + path, opts);
  const text = await r.text();
  let body: any = {};
  try {
    body = text ? JSON.parse(text) : {};
  } catch {
    body = { error: text };
  }
  if (!r.ok) {
    // The toast shows the friendly `body.error` sentence ONLY — the raw
    // `stderr` (git/CLI prose, absolute paths, "exit status N") is kept on
    // `.details` for an optional disclosure, never dumped into the message
    // (phone report class: scary multi-line blobs for ordinary actions).
    throw new ApiError(
      body.error || r.statusText,
      r.status,
      typeof body.code === "string" && body.code ? body.code : undefined,
      typeof body.stderr === "string" && body.stderr ? body.stderr : undefined,
    );
  }
  return body as T;
}

const post = <T = any>(path: string, body?: any) =>
  api<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body || {}),
  });

export const diffPath = (sid: string, scope: DiffScope) =>
  `/sessions/${sid}/diff?scope=${scope}`;

export const AR = {
  health: () => api<Health>("/health"),
  daemonStart: () => post("/daemon/start"),
  trust: (dir: string) => post("/trust", { dir }),

  sessions: (limit = 0, offset = 0) => {
    const q = new URLSearchParams();
    if (limit > 0) q.set("limit", String(limit));
    if (offset > 0) q.set("offset", String(offset));
    return api<Session[]>("/sessions" + (q.size ? `?${q}` : ""));
  },
  newSession: (b: {
    spec: string;
    extraSpecs: SpecFile[];
    workspace: string;
    message: string;
    mode: string;
  }) => post<{ sid: string }>("/sessions", b),
  makeWorkspace: () => post<{ path: string }>("/workspace"),
  makeWorktree: (repo: string, branch: string, ref = "") =>
    post<{ path: string; repo: string; branch: string }>("/worktree", { repo, branch, ref }),
  upload: async (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return api<{ path: string; name: string }>("/upload", { method: "POST", body: fd });
  },
  // Composer helpers (INC-56) — both go through `ar` on the server; the browser
  // never talks to a provider. optimize rewrites a draft prompt; dictate
  // transcribes an already-uploaded recording (pass an /api/upload path).
  optimize: (draft: string, context = "") => post<{ text: string }>("/optimize", { draft, context }),
  dictate: (path: string, context = "") => post<{ text: string }>("/dictate", { path, context }),

  events: (sid: string, after: number) =>
    api<Envelope[]>(`/sessions/${sid}/events?after=${after}`),
  state: (sid: string) => api<any>(`/sessions/${sid}/state`),
  inspect: (sid: string) => api<any>(`/sessions/${sid}/inspect`),
  // Raw text of one published artifact version (INC-40) — not JSON.
  artifact: async (sid: string, stream: string, version?: number) => {
    const q = new URLSearchParams({ stream });
    if (version) q.set("version", String(version));
    const r = await fetch(`/api/sessions/${sid}/artifact?` + q.toString());
    const text = await r.text();
    if (!r.ok) throw new Error(text || r.statusText);
    return text;
  },
  rawEvents: (sid: string) => api<Envelope[]>(`/sessions/${sid}/events`),
  ps: (sid: string) => api<Task[]>(`/sessions/${sid}/ps`),
  barriers: (sid: string) => api<string[]>(`/sessions/${sid}/barriers`),
  barrier: (sid: string) => post<{ barrier: string }>(`/sessions/${sid}/barrier`),
  files: (sid: string, q: string) =>
    api<{ workspace: string; known: boolean; files: string[] }>(
      `/sessions/${sid}/files?q=${encodeURIComponent(q)}`,
    ),
  fileURL: (sid: string, path: string) =>
    `/api/sessions/${encodeURIComponent(sid)}/file?path=${encodeURIComponent(path)}`,
  diff: (sid: string, scope: DiffScope = "working-tree") => api<DiffResp>(diffPath(sid, scope)),
  // One file's current working-tree text, so the diff view can reveal the
  // unmodified lines hidden between hunks ("N unmodified lines" collapsers).
  blob: (sid: string, path: string) =>
    api<{ lines: string[] }>(`/sessions/${sid}/blob?path=${encodeURIComponent(path)}`),
  commit: (sid: string, message: string) =>
    post<{ status: string }>(`/sessions/${sid}/commit`, { message }),
  // Push the workspace's current branch to its upstream/origin. Failures carry a
  // structured `kind` (no-remote / no-upstream / rejected / detached / auth) next
  // to the human message + git stderr; ApiError.push surfaces it to the caller.
  push: (sid: string) => post<{ status: string; branch: string }>(`/sessions/${sid}/push`),
  // Discard the workspace's changes back to HEAD (whole batch, or one file when
  // `path` is given) — destructive; the UI confirms first.
  revert: (sid: string, path?: string) =>
    post<{ status: string }>(`/sessions/${sid}/revert`, path ? { path } : {}),
  gitInit: (sid: string) => post<{ status: string }>(`/sessions/${sid}/git-init`),
  // Worktree lifecycle (INC-49): apply the worktree's changes back onto its main
  // checkout (clean-or-nothing git apply), and remove the worktree when done
  // (force skips the dirty-worktree guard after the user confirms).
  applyWorktree: (sid: string) => post<{ status: string; mainRepo: string; applied: string }>(`/sessions/${sid}/apply`),
  removeWorktree: (sid: string, force = false) =>
    post<{ status: string; mainRepo: string }>(`/sessions/${sid}/worktree/remove`, { force }),

  // delivery (INC-43): "steer" folds the message into the running turn at its
  // next safe boundary; "queue"/undefined queues it for the next turn.
  send: (sid: string, text: string, images: string[], files: string[] = [], delivery?: "steer" | "queue") =>
    post(`/sessions/${sid}/send`, { text, images, files, ...(delivery ? { delivery } : {}) }),
  interrupt: (sid: string) => post(`/sessions/${sid}/interrupt`),
  resume: (sid: string) => post(`/sessions/${sid}/resume`),
  retry: (sid: string) => post(`/sessions/${sid}/retry`),
  // Structured ask (INC-47.2): specs are 1-based "<q>:<n>" the form builds.
  answer: (sid: string, specs: string[]) => post(`/sessions/${sid}/answer`, { specs }),
  skipAnswer: (sid: string) => post(`/sessions/${sid}/answer`, { skip: true }),
  // Queued-message management (INC-46/47.2).
  queue: (sid: string) => api<{ command_id: string; text: string; revoked: boolean }[]>(`/sessions/${sid}/queue`),
  unqueue: (sid: string, commandId: string) => post(`/sessions/${sid}/unqueue`, { commandId }),
  closeSession: (sid: string) => post(`/sessions/${sid}/close`),
  stopSession: (sid: string) => post(`/sessions/${sid}/stop`),
  compact: (sid: string) => post(`/sessions/${sid}/compact`),
  clear: (sid: string) => post(`/sessions/${sid}/clear`),
  mode: (sid: string, mode: "default" | "acceptEdits") => post(`/sessions/${sid}/mode`, { mode }),
  goal: (sid: string, b: { action: "attach" | "update" | "pause" | "resume" | "cancel"; goal?: string; verifier?: string; maxChecks?: number }) =>
    post(`/sessions/${sid}/goal`, b),
  kill: (sid: string, handle: string) => post(`/sessions/${sid}/kill`, { handle }),
  approve: (sid: string, approvalId: string, decision: "approve" | "deny", reason: string, always = false) =>
    post(`/sessions/${sid}/approve`, { approvalId, decision, reason, always }),
  switchAgent: (sid: string, spec: string, extraSpecs: SpecFile[]) =>
    post(`/sessions/${sid}/agent`, { spec, extraSpecs }),
  fork: (sid: string, barrier: string, workspace: string) =>
    post<{ sid: string }>(`/sessions/${sid}/fork`, { barrier, workspace }),

  gitBranches: (dir: string) =>
    api<{ isRepo: boolean; current: string; branches: string[]; dirty: number; hasCommits?: boolean }>(
      `/git/branches?dir=${encodeURIComponent(dir)}`,
    ),
  gitCheckout: (dir: string, branch: string, create: boolean) =>
    post<{ status: string; branch: string }>(`/git/checkout`, { dir, branch, create }),

  // Project overlay + system launcher (INC-53, HANDA #24). projects returns the
  // workspace-keyed cosmetic overlay; updateProject patches display name/fold;
  // openIn launches a whitelisted system app on a known workspace directory.
  projects: () => api<Record<string, ProjectMeta>>("/projects"),
  updateProject: (workspace: string, patch: { displayName?: string; folded?: boolean }) =>
    post<Record<string, ProjectMeta>>("/projects", { workspace, ...patch }),
  openIn: (workspace: string, app: LauncherApp) =>
    post<{ status: string }>("/open", { workspace, app }),

  runs: () => api<Run[]>("/runs"),
  startRun: (b: {
    kind: "submit" | "drive";
    spec: string;
    extraSpecs: SpecFile[];
    task: string;
    workspace: string;
    mode: string;
    idem: string;
  }) => post<{ runId: string }>("/runs", b),
  stopRun: (rid: string) => post(`/runs/${rid}/stop`),
};

// uploadURL maps an upload's server path to its preview URL — a file we just
// uploaded is previewable from the local uploads dir.
//
// RT-6: an image that is already an API URL (a durable session blob, see
// sessionImageURL) passes through untouched, so every image viewer downstream —
// thumbnails AND the Lightbox — takes one kind of handle: "something the server
// will serve me", not "a path under the uploads dir".
export const uploadURL = (path: string) =>
  path.startsWith("/api/")
    ? path
    : "/api/uploads/" + encodeURIComponent(path.split("/").pop() || "");

// sessionImageURL is the durable preview of an attachment the user sent: the
// journal records only a CAS ref, and the bytes live in the session's artifact
// store, so this — unlike uploadURL — still resolves after a reload, in another
// tab, or on another machine reading the same store (RT-6).
export const sessionImageURL = (sid: string, ref: string) =>
  `/api/sessions/${encodeURIComponent(sid)}/image/${encodeURIComponent(ref)}`;
