import type { DiffResp, Envelope, Health, Run, Session, SpecFile, Task } from "./types";

// api wraps the arwebui JSON contract. A non-2xx carries {error, stderr};
// we surface both so the cockpit shows the real ar failure (never swallow it).
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
    throw new Error((body.error || r.statusText) + (body.stderr ? "\n" + body.stderr : ""));
  }
  return body as T;
}

const post = <T = any>(path: string, body?: any) =>
  api<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body || {}),
  });

export const AR = {
  health: () => api<Health>("/health"),
  daemonStart: () => post("/daemon/start"),
  trust: (dir: string) => post("/trust", { dir }),

  sessions: () => api<Session[]>("/sessions"),
  newSession: (b: {
    spec: string;
    extraSpecs: SpecFile[];
    workspace: string;
    message: string;
    mode: string;
  }) => post<{ sid: string }>("/sessions", b),
  makeWorkspace: () => post<{ path: string }>("/workspace"),
  makeWorktree: (repo: string, branch: string, ref = "") =>
    post<{ path: string; repo: string }>("/worktree", { repo, branch, ref }),
  upload: async (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return api<{ path: string; name: string }>("/upload", { method: "POST", body: fd });
  },

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
  diff: (sid: string) => api<DiffResp>(`/sessions/${sid}/diff`),
  commit: (sid: string, message: string) =>
    post<{ status: string }>(`/sessions/${sid}/commit`, { message }),
  gitInit: (sid: string) => post<{ status: string }>(`/sessions/${sid}/git-init`),

  // delivery (INC-43): "steer" folds the message into the running turn at its
  // next safe boundary; "queue"/undefined queues it for the next turn.
  send: (sid: string, text: string, images: string[], files: string[] = [], delivery?: "steer" | "queue") =>
    post(`/sessions/${sid}/send`, { text, images, files, ...(delivery ? { delivery } : {}) }),
  interrupt: (sid: string) => post(`/sessions/${sid}/interrupt`),
  resume: (sid: string) => post(`/sessions/${sid}/resume`),
  retry: (sid: string) => post(`/sessions/${sid}/retry`),
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
    api<{ isRepo: boolean; current: string; branches: string[]; dirty: number }>(
      `/git/branches?dir=${encodeURIComponent(dir)}`,
    ),
  gitCheckout: (dir: string, branch: string, create: boolean) =>
    post<{ status: string; branch: string }>(`/git/checkout`, { dir, branch, create }),

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

// uploadURL maps an upload's server path to its preview URL — the journal
// keeps only a CAS ref, so thumbnails come from the local uploads dir.
export const uploadURL = (path: string) =>
  "/api/uploads/" + encodeURIComponent(path.split("/").pop() || "");
