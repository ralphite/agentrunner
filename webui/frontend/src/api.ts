import type { Envelope, Health, Run, Session, SpecFile, Task } from "./types";

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
  upload: async (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    return api<{ path: string; name: string }>("/upload", { method: "POST", body: fd });
  },

  events: (sid: string, after: number) =>
    api<Envelope[]>(`/sessions/${sid}/events?after=${after}`),
  state: (sid: string) => api<any>(`/sessions/${sid}/state`),
  inspect: (sid: string) => api<any>(`/sessions/${sid}/inspect`),
  rawEvents: (sid: string) => api<Envelope[]>(`/sessions/${sid}/events`),
  ps: (sid: string) => api<Task[]>(`/sessions/${sid}/ps`),
  barriers: (sid: string) => api<string[]>(`/sessions/${sid}/barriers`),

  send: (sid: string, text: string, images: string[]) =>
    post(`/sessions/${sid}/send`, { text, images }),
  interrupt: (sid: string) => post(`/sessions/${sid}/interrupt`),
  close: (sid: string) => post(`/sessions/${sid}/close`),
  resume: (sid: string) => post(`/sessions/${sid}/resume`),
  kill: (sid: string, handle: string) => post(`/sessions/${sid}/kill`, { handle }),
  approve: (sid: string, approvalId: string, decision: "approve" | "deny", reason: string) =>
    post(`/sessions/${sid}/approve`, { approvalId, decision, reason }),
  switchAgent: (sid: string, spec: string, extraSpecs: SpecFile[]) =>
    post(`/sessions/${sid}/agent`, { spec, extraSpecs }),
  fork: (sid: string, barrier: string, workspace: string) =>
    post<{ sid: string }>(`/sessions/${sid}/fork`, { barrier, workspace }),

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
