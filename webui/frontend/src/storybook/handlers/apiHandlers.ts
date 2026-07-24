import { HttpResponse, http, type RequestHandler } from "msw";
import type {
  AgentCatalogEntry,
  BackgroundWork,
  DiffResp,
  Envelope,
  Health,
  ProjectMeta,
  Run,
  ScheduleDetail,
  Session,
} from "../../types";
import {
  buildBackgroundWork,
  buildDiff,
  buildHealth,
  buildInputReceived,
  buildInspect,
  buildProjects,
  buildRun,
  buildScheduleDetail,
  buildSession,
  buildTimeline,
  cloneFixture,
  fixtureDefaults,
  type StoryInspectFixture,
} from "../fixtures";

export interface StoryQueuedMessage {
  command_id: string;
  text: string;
  revoked: boolean;
}

export interface StoryFileSearch {
  workspace: string;
  known: boolean;
  files: string[];
}

export interface StoryApiSeed {
  health?: Health;
  agents?: AgentCatalogEntry[];
  sessions?: Session[];
  runs?: Run[];
  projects?: Record<string, ProjectMeta>;
  events?: Record<string, Envelope[]>;
  inspect?: Record<string, StoryInspectFixture>;
  backgroundWork?: Record<string, BackgroundWork[]>;
  queue?: Record<string, StoryQueuedMessage[]>;
  diffs?: Record<string, DiffResp>;
  schedules?: Record<string, ScheduleDetail>;
  blobs?: Record<string, string[]>;
  files?: Record<string, StoryFileSearch>;
}

export interface StoryApiState {
  health: Health;
  agents: AgentCatalogEntry[];
  sessions: Session[];
  runs: Run[];
  projects: Record<string, ProjectMeta>;
  events: Record<string, Envelope[]>;
  inspect: Record<string, StoryInspectFixture>;
  backgroundWork: Record<string, BackgroundWork[]>;
  queue: Record<string, StoryQueuedMessage[]>;
  diffs: Record<string, DiffResp>;
  schedules: Record<string, ScheduleDetail>;
  blobs: Record<string, string[]>;
  files: Record<string, StoryFileSearch>;
}

export interface StoryApiHandlerGroups {
  runtime: RequestHandler[];
  sessions: RequestHandler[];
  projects: RequestHandler[];
  runs: RequestHandler[];
  workspace: RequestHandler[];
}

export interface StoryApiHarness {
  /** Flat list for `parameters.msw.handlers`. No catch-all is included. */
  handlers: RequestHandler[];
  /** Named subsets for stories that want to compose only the surfaces they use. */
  groups: StoryApiHandlerGroups;
  snapshot(): StoryApiState;
  appendEvents(sid: string, events: readonly Envelope[]): void;
  updateSession(sid: string, patch: Partial<Session>): void;
  setInspect(sid: string, inspect: StoryInspectFixture): void;
  reset(): void;
}

interface NewSessionBody {
  workspace?: string;
  message?: string;
}

interface UpdateProjectBody {
  workspace?: string;
  displayName?: string;
  folded?: boolean;
  pinned?: boolean;
  removed?: boolean;
}

interface StartRunBody {
  kind?: "submit" | "drive";
  prompt?: string;
  workspace?: string;
}

function initialState(seed: StoryApiSeed): StoryApiState {
  const defaultSession = buildSession();
  const defaultRun = buildRun({ sessionId: defaultSession.id });
  return {
    health: cloneFixture(seed.health ?? buildHealth()),
    agents: cloneFixture(seed.agents ?? [
      {
        name: "dev",
        description: "Build and change code",
        source: "shipped",
        yaml: "name: dev\nsystem_prompt: Build and change code.\ntools: []\n",
      },
      {
        name: "reviewer",
        description: "Review implementation quality",
        source: "shipped",
        yaml: "name: reviewer\nsystem_prompt: Review implementation quality.\ntools: []\n",
      },
    ]),
    sessions: cloneFixture(seed.sessions ?? [defaultSession]),
    runs: cloneFixture(seed.runs ?? [defaultRun]),
    projects: cloneFixture(seed.projects ?? buildProjects()),
    events: cloneFixture(seed.events ?? {
      [defaultSession.id]: buildTimeline(),
    }),
    inspect: cloneFixture(seed.inspect ?? {
      [defaultSession.id]: buildInspect(),
    }),
    backgroundWork: cloneFixture(seed.backgroundWork ?? {
      [defaultSession.id]: [buildBackgroundWork()],
    }),
    queue: cloneFixture(seed.queue ?? {
      [defaultSession.id]: [],
    }),
    diffs: cloneFixture(seed.diffs ?? {
      [defaultSession.id]: buildDiff(),
    }),
    schedules: cloneFixture(seed.schedules ?? {}),
    blobs: cloneFixture(seed.blobs ?? {
      "src/Card.tsx": ['export const label = "After";'],
    }),
    files: cloneFixture(seed.files ?? {
      [defaultSession.id]: {
        workspace: defaultSession.workspace ?? fixtureDefaults.workspace,
        known: true,
        files: ["src/Card.tsx", "src/Card.stories.tsx"],
      },
    }),
  };
}

function jsonError(error: string, status: number, code?: string) {
  return HttpResponse.json({ error, ...(code ? { code } : {}) }, { status });
}

function sessionExists(state: StoryApiState, sid: string): boolean {
  return state.sessions.some((session) => session.id === sid);
}

function sessionOr404(state: StoryApiState, sid: string) {
  return sessionExists(state, sid)
    ? null
    : jsonError("No session matches that id.", 404, "session_not_found");
}

function fixtureImage() {
  return HttpResponse.text(
    [
      '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="48"',
      ' viewBox="0 0 64 48" role="img" aria-label="Storybook fixture">',
      '<rect width="64" height="48" rx="6" fill="#dbeafe"/>',
      '<path d="M12 34 25 21l8 8 7-7 12 12" fill="none"',
      ' stroke="#2563eb" stroke-width="3"/>',
      "</svg>",
    ].join(""),
    { headers: { "Content-Type": "image/svg+xml" } },
  );
}

/**
 * Stateful MSW v2 handlers for Storybook. State is private to each factory
 * call, and `reset` restores a fresh clone of the seed. Deliberately no
 * `/api/*` wildcard lives here: preview's strict unhandled-request policy keeps
 * detecting request-contract drift.
 */
export function createStoryApiHandlers(
  seed: StoryApiSeed = {},
): StoryApiHarness {
  const pristine = initialState(seed);
  let state = cloneFixture(pristine);
  let sessionSequence = 0;
  let runSequence = 0;
  let barrierSequence = 0;
  let commandSequence = 0;

  const runtime: RequestHandler[] = [
    http.get("/api/health", () => HttpResponse.json(cloneFixture(state.health))),
    http.get("/api/agents", () => HttpResponse.json(cloneFixture(state.agents))),
    http.post("/api/daemon/start", () => {
      state.health.daemonUp = true;
      return HttpResponse.json({ status: "started" });
    }),
    http.post("/api/trust", () => HttpResponse.json({ status: "trusted" })),
  ];

  const sessions: RequestHandler[] = [
    http.get("/api/sessions", ({ request }) => {
      const url = new URL(request.url);
      const limit = Math.max(0, Number(url.searchParams.get("limit") ?? 0));
      const offset = Math.max(0, Number(url.searchParams.get("offset") ?? 0));
      const rows = limit > 0
        ? state.sessions.slice(offset, offset + limit)
        : state.sessions.slice(offset);
      return HttpResponse.json(cloneFixture(rows));
    }),
    http.post("/api/sessions", async ({ request }) => {
      const body = await request.json() as NewSessionBody;
      sessionSequence += 1;
      const id = `story-session-${sessionSequence}`;
      const session = buildSession({
        id,
        title: body.message?.trim() || "New Storybook session",
        workspace: body.workspace?.trim() || fixtureDefaults.workspace,
        turns: 0,
      });
      state.sessions.unshift(session);
      state.events[id] = body.message?.trim()
        ? [
            buildInputReceived({
              seq: 1,
              command_id: "story-command-1",
              payload: {
                source: "user",
                text: body.message.trim(),
                item_id: "story-user-1",
                turn_id: "story-turn-1",
              },
            }),
          ]
        : [];
      state.inspect[id] = buildInspect();
      state.backgroundWork[id] = [];
      state.queue[id] = [];
      state.diffs[id] = buildDiff({ workspace: session.workspace });
      state.files[id] = {
        workspace: session.workspace ?? fixtureDefaults.workspace,
        known: true,
        files: [],
      };
      return HttpResponse.json({ sid: id });
    }),
    http.get("/api/sessions/:sid/events", ({ params, request }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      const after = Number(new URL(request.url).searchParams.get("after") ?? 0);
      return HttpResponse.json(
        cloneFixture((state.events[sid] ?? []).filter((event) => event.seq > after)),
      );
    }),
    http.get("/api/sessions/:sid/state", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      return HttpResponse.json({
        status: state.sessions.find((session) => session.id === sid)?.status ?? "unknown",
      });
    }),
    http.get("/api/sessions/:sid/inspect", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      return HttpResponse.json(cloneFixture(state.inspect[sid] ?? buildInspect()));
    }),
    http.get("/api/sessions/:sid/ps", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      return HttpResponse.json(cloneFixture(state.backgroundWork[sid] ?? []));
    }),
    http.get("/api/sessions/:sid/queue", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      return HttpResponse.json(cloneFixture(state.queue[sid] ?? []));
    }),
    http.get("/api/sessions/:sid/barriers", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      return HttpResponse.json([]);
    }),
    http.post("/api/sessions/:sid/barrier", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      barrierSequence += 1;
      return HttpResponse.json({ barrier: `story-barrier-${barrierSequence}` });
    }),
    http.get("/api/sessions/:sid/files", ({ params, request }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      const source = state.files[sid] ?? {
        workspace: fixtureDefaults.workspace,
        known: true,
        files: [],
      };
      const query = new URL(request.url).searchParams.get("q")?.toLowerCase() ?? "";
      return HttpResponse.json({
        ...cloneFixture(source),
        files: source.files.filter((file) => file.toLowerCase().includes(query)),
      });
    }),
    http.get("/api/sessions/:sid/diff", ({ params, request }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      const scope = new URL(request.url).searchParams.get("scope");
      const diff = cloneFixture(state.diffs[sid] ?? buildDiff());
      if (scope === "working-tree" || scope === "last-turn") diff.scope = scope;
      return HttpResponse.json(diff);
    }),
    http.get("/api/sessions/:sid/blob", ({ params, request }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      const path = new URL(request.url).searchParams.get("path") ?? "";
      const lines = state.blobs[path];
      return lines
        ? HttpResponse.json({ lines: cloneFixture(lines) })
        : jsonError("file is unavailable", 404, "blob_not_found");
    }),
    http.get("/api/sessions/:sid/schedule", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      return HttpResponse.json(
        cloneFixture(state.schedules[sid] ?? buildScheduleDetail({ sessionId: sid })),
      );
    }),
    http.get("/api/sessions/:sid/artifact", ({ params, request }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      const stream = new URL(request.url).searchParams.get("stream") ?? "artifact";
      return HttpResponse.text(`# ${stream}\n\nStorybook fixture artifact.\n`);
    }),
    http.get("/api/sessions/:sid/file", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      return missing ?? HttpResponse.text("Storybook fixture file.\n");
    }),
    http.get("/api/sessions/:sid/image/:ref", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      return missing ?? fixtureImage();
    }),
    http.post("/api/sessions/:sid/send", async ({ params, request }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      const body = await request.json() as { text?: string; delivery?: string };
      commandSequence += 1;
      const events = state.events[sid] ?? (state.events[sid] = []);
      events.push(buildInputReceived({
        seq: (events[events.length - 1]?.seq ?? 0) + 1,
        ts: new Date(Date.parse(fixtureDefaults.time) + commandSequence * 1000).toISOString(),
        command_id: `story-command-${commandSequence}`,
        payload: {
          source: "user",
          text: body.text ?? "",
        },
      }));
      const session = state.sessions.find((item) => item.id === sid);
      if (session) session.turns += 1;
      return HttpResponse.json({ status: "queued" });
    }),
    http.post("/api/sessions/:sid/rename", async ({ params, request }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      const body = await request.json() as { title?: string };
      const session = state.sessions.find((item) => item.id === sid);
      if (session && body.title?.trim()) session.title = body.title.trim();
      return HttpResponse.json({ status: "renamed" });
    }),
    http.post("/api/sessions/:sid/unqueue", async ({ params, request }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      const body = await request.json() as { commandId?: string };
      const message = (state.queue[sid] ?? []).find(
        (item) => item.command_id === body.commandId,
      );
      if (message) message.revoked = true;
      return HttpResponse.json({ status: "revoked" });
    }),
    http.post("/api/sessions/:sid/schedule", async ({ params, request }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      const body = await request.json() as {
        action?: "pause" | "resume" | "update";
        expectedRevision?: number;
        prompt?: string;
        schedule?: "interval" | "cron";
        interval?: string;
        cron?: string;
        overlap?: "skip" | "coalesce";
      };
      const detail = state.schedules[sid] ??
        (state.schedules[sid] = buildScheduleDetail({ sessionId: sid }));
      if (body.action === "pause") detail.status = "paused";
      if (body.action === "resume") detail.status = "running";
      if (body.action === "update") {
        if (
          body.expectedRevision !== undefined &&
          body.expectedRevision !== detail.revision
        ) {
          return jsonError("schedule revision changed", 409, "stale_revision");
        }
        if (body.prompt !== undefined) detail.prompt = body.prompt;
        if (body.schedule !== undefined) detail.schedule = body.schedule;
        if (body.interval !== undefined) detail.interval = body.interval;
        if (body.cron !== undefined) detail.cron = body.cron;
        if (body.overlap !== undefined) detail.overlap = body.overlap;
        detail.revision += 1;
      }
      return HttpResponse.json({ status: body.action ?? "updated" });
    }),
    http.post("/api/sessions/:sid/push", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      return missing ?? HttpResponse.json({
        status: "pushed",
        branch: "storybook/demo",
      });
    }),
    http.post("/api/sessions/:sid/apply", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      return missing ?? HttpResponse.json({
        status: "applied",
        mainRepo: fixtureDefaults.workspace,
        applied: "storybook/demo",
      });
    }),
    http.post("/api/sessions/:sid/fork", async ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      if (missing) return missing;
      sessionSequence += 1;
      const childID = `story-session-${sessionSequence}`;
      const parent = state.sessions.find((session) => session.id === sid);
      state.sessions.unshift(buildSession({
        id: childID,
        title: `Continuation of ${parent?.title ?? sid}`,
        workspace: parent?.workspace ?? fixtureDefaults.workspace,
        turns: 0,
      }));
      state.events[childID] = [];
      return HttpResponse.json({ sid: childID });
    }),
    http.post(
      "/api/sessions/:sid/continue-from-message",
      async ({ params, request }) => {
        const sid = String(params.sid);
        const missing = sessionOr404(state, sid);
        if (missing) return missing;
        const body = await request.json() as {
          item_id?: string;
          request_id?: string;
        };
        sessionSequence += 1;
        const childID = `story-session-${sessionSequence}`;
        const sourceItemID = body.item_id ?? "story-item";
        const parent = state.sessions.find((session) => session.id === sid);
        state.sessions.unshift(buildSession({
          id: childID,
          title: `Continuation of ${parent?.title ?? sid}`,
          workspace: parent?.workspace ?? fixtureDefaults.workspace,
          turns: 0,
        }));
        state.events[childID] = [];
        return HttpResponse.json({
          session_id: childID,
          source_item_id: sourceItemID,
          source_side: "after_assistant",
          draft: {
            draft_id: body.request_id ?? "story-draft",
            text: "",
            content: [],
          },
        });
      },
    ),
    ...([
      "commit",
      "revert",
      "git-init",
      "interrupt",
      "resume",
      "retry",
      "answer",
      "stop",
      "approve",
      "agent",
      "compact",
      "clear",
      "mode",
      "promote",
      "goal",
    ] as const).map((action) =>
      http.post(`/api/sessions/:sid/${action}`, ({ params }) => {
        const sid = String(params.sid);
        const missing = sessionOr404(state, sid);
        if (missing) return missing;
        const session = state.sessions.find((item) => item.id === sid);
        if (session) {
          if (action === "stop" || action === "interrupt") {
            session.status = "stopped";
          }
          if (action === "resume" || action === "retry") {
            session.status = "running";
          }
        }
        const status = action === "approve"
          ? "resolved"
          : action === "commit"
            ? "committed"
            : action === "revert"
              ? "reverted"
              : action === "promote"
                ? "promoted"
                : "ok";
        return HttpResponse.json({ status });
      })
    ),
    http.post("/api/sessions/:sid/worktree/remove", ({ params }) => {
      const sid = String(params.sid);
      const missing = sessionOr404(state, sid);
      return missing ?? HttpResponse.json({
        status: "removed",
        mainRepo: fixtureDefaults.workspace,
      });
    }),
  ];

  const projects: RequestHandler[] = [
    http.get("/api/projects", () => HttpResponse.json(cloneFixture(state.projects))),
    http.post("/api/projects", async ({ request }) => {
      const body = await request.json() as UpdateProjectBody;
      const workspace = body.workspace?.trim();
      if (!workspace) return jsonError("workspace is required", 400);
      const current = state.projects[workspace] ?? {};
      const { displayName, folded, pinned, removed } = body;
      state.projects[workspace] = {
        ...current,
        ...(displayName !== undefined ? { displayName } : {}),
        ...(folded !== undefined ? { folded } : {}),
        ...(pinned !== undefined ? { pinned } : {}),
        ...(removed !== undefined ? { removed } : {}),
      };
      return HttpResponse.json(cloneFixture(state.projects));
    }),
    http.post("/api/open", async ({ request }) => {
      const body = await request.json() as { workspace?: string };
      const workspace = body.workspace?.trim();
      if (workspace) {
        state.projects[workspace] = {
          ...(state.projects[workspace] ?? {}),
          lastOpened: Date.parse(fixtureDefaults.time) + 1000,
        };
      }
      return HttpResponse.json({ status: "opened" });
    }),
  ];

  const runs: RequestHandler[] = [
    http.get("/api/runs", () => HttpResponse.json(cloneFixture(state.runs))),
    http.post("/api/runs", async ({ request }) => {
      const body = await request.json() as StartRunBody;
      runSequence += 1;
      const id = `story-run-${runSequence}`;
      state.runs.unshift(buildRun({
        id,
        kind: body.kind ?? "submit",
        label: body.prompt?.trim() || "New Storybook run",
        workspace: body.workspace?.trim() || fixtureDefaults.workspace,
        sessionId: undefined,
      }));
      return HttpResponse.json({ runId: id });
    }),
    http.post("/api/runs/:rid/stop", ({ params }) => {
      const rid = String(params.rid);
      const run = state.runs.find((item) => item.id === rid);
      if (!run) return jsonError("run not found", 404, "run_not_found");
      run.status = "stopped";
      return HttpResponse.json({ status: "stopped" });
    }),
  ];

  const workspace: RequestHandler[] = [
    http.post("/api/workspace", () =>
      HttpResponse.json({ path: fixtureDefaults.workspace })),
    http.post("/api/worktree", async ({ request }) => {
      const body = await request.json() as { repo?: string; branch?: string };
      const branch = body.branch?.trim() || "storybook/demo";
      return HttpResponse.json({
        path: `${fixtureDefaults.workspace}-worktree`,
        repo: body.repo?.trim() || fixtureDefaults.workspace,
        branch,
      });
    }),
    http.get("/api/git/branches", ({ request }) => {
      const dir = new URL(request.url).searchParams.get("dir") ??
        fixtureDefaults.workspace;
      return HttpResponse.json({
        isRepo: true,
        current: "storybook/demo",
        branches: ["main", "storybook/demo"],
        dirty: 1,
        hasCommits: true,
        dir,
      });
    }),
    http.post("/api/git/checkout", async ({ request }) => {
      const body = await request.json() as { branch?: string };
      return HttpResponse.json({
        status: "checked-out",
        branch: body.branch?.trim() || "storybook/demo",
      });
    }),
    http.post("/api/optimize", async ({ request }) => {
      const body = await request.json() as { draft?: string };
      return HttpResponse.json({
        text: body.draft?.trim()
          ? `${body.draft.trim()} — clarified for the Storybook demo.`
          : "Describe the Storybook scenario.",
      });
    }),
    http.post("/api/dictate", () =>
      HttpResponse.json({ text: "Fixture-only dictated prompt." })),
    http.post("/api/upload", () =>
      HttpResponse.json({
        path: "/runtime/storybook/upload.png",
        name: "storybook-fixture.png",
      })),
    http.get("/api/uploads/:name", () => fixtureImage()),
  ];

  const groups: StoryApiHandlerGroups = {
    runtime,
    sessions,
    projects,
    runs,
    workspace,
  };

  return {
    groups,
    handlers: Object.values(groups).flat(),
    snapshot: () => cloneFixture(state),
    appendEvents: (sid, events) => {
      if (!sessionExists(state, sid)) {
        throw new Error(`Cannot append events for unknown session: ${sid}`);
      }
      state.events[sid] = [
        ...(state.events[sid] ?? []),
        ...cloneFixture([...events]),
      ];
    },
    updateSession: (sid, patch) => {
      const index = state.sessions.findIndex((session) => session.id === sid);
      if (index < 0) {
        throw new Error(`Cannot update unknown session: ${sid}`);
      }
      state.sessions[index] = {
        ...state.sessions[index],
        ...cloneFixture(patch),
      };
    },
    setInspect: (sid, inspect) => {
      if (!sessionExists(state, sid)) {
        throw new Error(`Cannot set inspect for unknown session: ${sid}`);
      }
      state.inspect[sid] = cloneFixture(inspect);
    },
    reset: () => {
      state = cloneFixture(pristine);
      sessionSequence = 0;
      runSequence = 0;
      barrierSequence = 0;
      commandSequence = 0;
    },
  };
}
