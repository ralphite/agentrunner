import type {
  BackgroundWork,
  DiffResp,
  Envelope,
  Health,
  ProjectMeta,
  Run,
  ScheduleDetail,
  Session,
} from "../../types";

export type FixtureOverrides<T> = {
  [K in keyof T]?: T[K] extends readonly (infer Item)[]
    ? FixtureOverrides<Item>[]
    : T[K] extends object
      ? FixtureOverrides<T[K]>
      : T[K];
};

type JsonRecord = Record<string, unknown>;

function isRecord(value: unknown): value is JsonRecord {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

/**
 * Fixtures are JSON-shaped API values. Recursive cloning here is intentional:
 * two stories rendered on the same docs page must never share nested arrays or
 * objects through a module-level fixture constant.
 */
export function cloneFixture<T>(value: T): T {
  if (Array.isArray(value)) {
    return value.map((item) => cloneFixture(item)) as T;
  }
  if (isRecord(value)) {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, cloneFixture(item)]),
    ) as T;
  }
  return value;
}

function mergeFixture<T>(base: T, overrides?: FixtureOverrides<T>): T {
  if (!overrides) return cloneFixture(base);
  if (!isRecord(base) || !isRecord(overrides)) {
    return cloneFixture(overrides as T);
  }

  const result: JsonRecord = cloneFixture(base) as JsonRecord;
  for (const [key, override] of Object.entries(overrides)) {
    if (override === undefined) continue;
    const prior = result[key];
    result[key] = isRecord(prior) && isRecord(override)
      ? mergeFixture(prior, override)
      : cloneFixture(override);
  }
  return result as T;
}

const FIXTURE_TIME = "2026-01-15T12:00:00Z";
const FIXTURE_WORKSPACE = "/workspace/storybook-demo";

export function buildHealth(overrides?: FixtureOverrides<Health>): Health {
  return mergeFixture<Health>(
    {
      version: "storybook",
      daemonUp: true,
      daemonManaged: true,
      daemonExternal: false,
      manageRequested: false,
      daemonLogPath: "/runtime/storybook/agentrunner.log",
      runtimeDir: "/runtime/storybook",
      sandboxBackend: "storybook",
      sandboxDetected: true,
    },
    overrides,
  );
}

export function buildSession(overrides?: FixtureOverrides<Session>): Session {
  return mergeFixture<Session>(
    {
      id: "story-session",
      status: "running",
      turns: 2,
      attention: { approvals: 0, answers: 0 },
      updatedAt: FIXTURE_TIME,
      title: "Prepare the component demo",
      workspace: FIXTURE_WORKSPACE,
      kind: "session",
    },
    overrides,
  );
}

export function buildRun(overrides?: FixtureOverrides<Run>): Run {
  return mergeFixture<Run>(
    {
      id: "story-run",
      kind: "submit",
      label: "Validate the Storybook scenario",
      workspace: FIXTURE_WORKSPACE,
      sessionId: "story-session",
      status: "running",
      startedAt: FIXTURE_TIME,
    },
    overrides,
  );
}

export function buildProjectMeta(
  overrides?: FixtureOverrides<ProjectMeta>,
): ProjectMeta {
  return mergeFixture<ProjectMeta>(
    {
      displayName: "Storybook Demo",
      folded: false,
      pinned: false,
      removed: false,
      lastOpened: Date.parse(FIXTURE_TIME),
    },
    overrides,
  );
}

export function buildProjects(
  overrides: Record<string, FixtureOverrides<ProjectMeta>> = {},
): Record<string, ProjectMeta> {
  const projects: Record<string, ProjectMeta> = {
    [FIXTURE_WORKSPACE]: buildProjectMeta(),
  };
  for (const [workspace, project] of Object.entries(overrides)) {
    projects[workspace] = buildProjectMeta(project);
  }
  return projects;
}

export function buildEnvelope(
  overrides?: FixtureOverrides<Envelope>,
): Envelope {
  return mergeFixture<Envelope>(
    {
      seq: 1,
      type: "input_received",
      ts: FIXTURE_TIME,
      command_id: "story-command-1",
      payload: {
        source: "user",
        text: "Show the reusable component states.",
      },
    },
    overrides,
  );
}

export type StoryEnvelope<Type extends string, Payload> =
  Omit<Envelope, "type" | "payload"> & {
    type: Type;
    payload: Payload;
  };

export interface StoryInputPayload {
  source: "user" | "cli" | "tty" | "program" | "agent";
  text: string;
  item_id?: string;
  turn_id?: string;
  images?: unknown[];
  files?: unknown[];
}

export function buildInputReceived(
  overrides?: FixtureOverrides<
    StoryEnvelope<"input_received", StoryInputPayload>
  >,
): StoryEnvelope<"input_received", StoryInputPayload> {
  return mergeFixture(
    {
      seq: 2,
      type: "input_received" as const,
      ts: FIXTURE_TIME,
      command_id: "story-command-1",
      payload: {
        source: "user" as const,
        text: "Show the reusable component states.",
        item_id: "story-user-1",
        turn_id: "story-turn-1",
      },
    },
    overrides,
  );
}

export interface StoryAssistantPayload {
  item_id?: string;
  turn_id?: string;
  message: {
    parts: Array<{
      text?: string;
      tool_name?: string;
      call_id?: string;
      args?: unknown;
    }>;
  };
}

export function buildAssistantMessage(
  overrides?: FixtureOverrides<
    StoryEnvelope<"assistant_message", StoryAssistantPayload>
  >,
): StoryEnvelope<"assistant_message", StoryAssistantPayload> {
  return mergeFixture(
    {
      seq: 4,
      type: "assistant_message" as const,
      ts: "2026-01-15T12:00:04Z",
      command_id: "story-command-1",
      payload: {
        item_id: "story-assistant-1",
        turn_id: "story-turn-1",
        message: {
          parts: [
            { text: "The component states are ready for review." },
          ],
        },
      },
    },
    overrides,
  );
}

export interface StoryApprovalPayload {
  approval_id: string;
  call_id: string;
  gate_results: Array<{
    gate: string;
    allowed: boolean;
    reason?: string;
  }>;
}

export function buildApprovalRequested(
  overrides?: FixtureOverrides<
    StoryEnvelope<"approval_requested", StoryApprovalPayload>
  >,
): StoryEnvelope<"approval_requested", StoryApprovalPayload> {
  return mergeFixture(
    {
      seq: 4,
      type: "approval_requested" as const,
      ts: "2026-01-15T12:00:04Z",
      command_id: "story-command-1",
      payload: {
        approval_id: "story-approval-1",
        call_id: "story-tool-1",
        gate_results: [
          {
            gate: "workspace",
            allowed: false,
            reason: "Review this fixture-only write.",
          },
        ],
      },
    },
    overrides,
  );
}

export function buildTimeline(): Envelope[] {
  return [
    buildEnvelope({
      seq: 1,
      type: "session_started",
      payload: { spec_name: "storybook", model: "fixture-model" },
    }),
    buildEnvelope({
      seq: 2,
      type: "checkpoint_barrier",
      command_id: "story-command-1",
      payload: {
        message_anchor: {
          side: "before_user",
          item_id: "story-user-1",
          turn_id: "story-turn-1",
        },
      },
    }),
    buildInputReceived({ seq: 3 }),
    buildEnvelope({
      seq: 4,
      type: "generation_started",
      command_id: "story-command-1",
      payload: { gen_step: 1 },
    }),
    buildAssistantMessage({ seq: 5 }),
    buildEnvelope({
      seq: 6,
      type: "waiting_entered",
      command_id: "story-command-1",
      payload: { kind: "input" },
    }),
    buildEnvelope({
      seq: 7,
      type: "checkpoint_barrier",
      command_id: "story-command-1",
      payload: {
        message_anchor: {
          side: "after_assistant",
          item_id: "story-assistant-1",
          turn_id: "story-turn-1",
        },
      },
    }),
  ];
}

export function buildDiff(overrides?: FixtureOverrides<DiffResp>): DiffResp {
  return mergeFixture<DiffResp>(
    {
      scope: "working-tree",
      available: true,
      workspace: FIXTURE_WORKSPACE,
      known: true,
      isRepo: true,
      branch: "storybook/demo",
      diff: [
        "diff --git a/src/Card.tsx b/src/Card.tsx",
        "index 1111111..2222222 100644",
        "--- a/src/Card.tsx",
        "+++ b/src/Card.tsx",
        "@@ -1 +1 @@",
        "-export const label = \"Before\";",
        "+export const label = \"After\";",
      ].join("\n"),
      numstat: "1\t1\tsrc/Card.tsx\n",
      untracked: ["src/Card.stories.tsx"],
      untrackedReasons: {},
      hiddenUntracked: 0,
      conflicts: [],
    },
    overrides,
  );
}

export function buildScheduleDetail(
  overrides?: FixtureOverrides<ScheduleDetail>,
): ScheduleDetail {
  return mergeFixture<ScheduleDetail>(
    {
      kind: "series",
      sessionId: "story-schedule",
      name: "Storybook quality pass",
      status: "running",
      prompt: "Review the demo states.",
      workspace: FIXTURE_WORKSPACE,
      agent: "reviewer",
      provider: "fixture",
      model: "fixture-model",
      schedule: "interval",
      cadence: "Every 30m",
      nextRunAt: "2026-01-15T12:30:00Z",
      scheduleControl: true,
      scheduleDetail: true,
      interval: "30m",
      overlap: "skip",
      iterations: 1,
      maxIterations: 4,
      scheduleEdit: true,
      revision: 1,
    },
    overrides,
  );
}

export function buildBackgroundWork(
  overrides?: FixtureOverrides<BackgroundWork>,
): BackgroundWork {
  return mergeFixture<BackgroundWork>(
    {
      handle: "story-work-1",
      tool: "storybook",
      detail: "Rendering isolated component states",
    },
    overrides,
  );
}

export interface StoryInspectFixture {
  mode: string;
  gen_steps: number;
  usage: {
    input_tokens: number;
    output_tokens: number;
    cache_read: number;
    cache_write: number;
    billed: number;
  };
  children: unknown[];
  delegations: unknown[];
  artifacts: Array<{
    stream: string;
    version: number;
  }>;
  progress: Array<{
    id: string;
    title: string;
    status: "pending" | "running" | "done" | "failed";
  }>;
  waiting?: unknown;
  goal?: {
    goal: string;
    checks: number;
    max_checks?: number;
    paused?: boolean;
  };
}

export function buildInspect(
  overrides?: FixtureOverrides<StoryInspectFixture>,
): StoryInspectFixture {
  return mergeFixture<StoryInspectFixture>(
    {
      mode: "default",
      gen_steps: 3,
      usage: {
        input_tokens: 900,
        output_tokens: 300,
        cache_read: 0,
        cache_write: 0,
        billed: 1200,
      },
      children: [],
      delegations: [],
      artifacts: [],
      progress: [
        {
          id: "stories",
          title: "Build isolated component state",
          status: "done",
        },
        {
          id: "interaction",
          title: "Verify the interaction",
          status: "running",
        },
      ],
    },
    overrides,
  );
}

export const fixtureDefaults = {
  time: FIXTURE_TIME,
  workspace: FIXTURE_WORKSPACE,
} as const;
