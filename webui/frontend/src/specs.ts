// ---------------------------------------------------------------------------
// Spec / driver construction for the Codex-style composer.
//
// The product has no `--model` / `--access` flags: the model lives in the agent
// spec's `model:` block and the approval posture lives in its `permissions:`
// block (plus the `--mode` runtime override). So the composer's model and
// access pickers work by *building the spec text* here, then handing it to
// `ar new` / `ar agent` / `ar drive` exactly as the YAML editor modal does.
// ---------------------------------------------------------------------------

export interface ModelChoice {
  provider: string;
  id: string;
  label: string; // short name shown on the pill (like Codex's "5.5")
  sub: string; // one-line description in the menu
}

// Curated, editable model list. Gemini is the primary provider (real creds);
// Anthropic is secondary (needs ANTHROPIC creds — offered, may error without).
export const MODELS: ModelChoice[] = [
  { provider: "gemini", id: "gemini-flash-latest", label: "Gemini Flash", sub: "Fast · default" },
  { provider: "gemini", id: "gemini-flash-lite-latest", label: "Gemini Flash Lite", sub: "Fastest, lightest" },
  { provider: "gemini", id: "gemini-3.5-flash", label: "Gemini 3.5 Flash", sub: "Newest flash" },
  { provider: "gemini", id: "gemini-pro-latest", label: "Gemini Pro", sub: "Most capable" },
  { provider: "anthropic", id: "claude-sonnet-5", label: "Claude Sonnet 5", sub: "Anthropic · needs creds" },
];

export const DEFAULT_MODEL = MODELS[0];

// Access levels = the composer's permission-mode pill. Each maps to a concrete,
// verifiable behavior via the spec's `permissions:` block and/or `--mode`:
//   full        → allow-all, nothing gated (matches today's DEFAULT_SPEC)
//   ask         → reads/search allowed, everything that mutates asks (approval
//                 cards surface in the UI)
//   acceptEdits → --mode acceptEdits: file edits auto-apply, other gated tools ask
//   plan        → --mode plan: read-only planning, no writes advertised
export type AccessId = "full" | "ask" | "acceptEdits" | "plan";

export interface AccessLevel {
  id: AccessId;
  label: string;
  desc: string;
  mode: string; // value for `ar new --mode` ("" = spec default)
  risk: "high" | "med" | "low";
}

export const ACCESS_LEVELS: AccessLevel[] = [
  { id: "full", label: "Full access", desc: "Nothing is gated — the agent acts freely", mode: "", risk: "high" },
  { id: "ask", label: "Ask to approve", desc: "Reads run freely; edits, shell & network ask first", mode: "", risk: "low" },
  { id: "acceptEdits", label: "Auto-accept edits", desc: "File edits apply automatically; other gated tools ask", mode: "acceptEdits", risk: "med" },
  { id: "plan", label: "Plan · read-only", desc: "No changes — the agent plans and proposes only", mode: "plan", risk: "low" },
];

// Ask-first by default (W15): a brand-new task should not silently run with
// nothing gated. Full access stays one click away and the composer remembers
// the user's last choice.
export const DEFAULT_ACCESS: AccessId = "ask";

export function accessById(id: string): AccessLevel {
  return ACCESS_LEVELS.find((a) => a.id === id) || ACCESS_LEVELS[0];
}

export function modelById(provider: string, id: string): ModelChoice | undefined {
  return MODELS.find((m) => m.provider === provider && m.id === id);
}

// ---- reasoning effort (Codex's model-pill "Effort" dimension) ---------------
//
// Codex's model pill reads "<model> <effort>" (e.g. "5.6 Sol Extra High") and its
// picker has a Light/Medium/High/Extra High "Effort" submenu. Our product models
// the same thing as the spec's `thinking:` block (ModelSpec.Thinking{Enabled,
// BudgetTokens}), which the Gemini provider maps to native extended thinking.
//
// Sizing rule (critical): Gemini counts thought tokens against max_output_tokens,
// so enabling thinking with a small cap starves the answer to empty — the
// "会话死亡" empty-message defect. We therefore size max_tokens = ANSWER_ROOM +
// budget, guaranteeing the answer always has ANSWER_ROOM left after thinking.
export type EffortId = "off" | "light" | "medium" | "high" | "xhigh";

export interface EffortLevel {
  id: EffortId;
  label: string;
  desc: string;
  budget: number; // thinking budget tokens (0 ⇒ thinking off)
}

export const EFFORT_LEVELS: EffortLevel[] = [
  { id: "off", label: "Off", desc: "No extended thinking — fastest replies", budget: 0 },
  { id: "light", label: "Light", desc: "A little reasoning before answering", budget: 2048 },
  { id: "medium", label: "Medium", desc: "Balanced reasoning on most tasks", budget: 6144 },
  { id: "high", label: "High", desc: "Thorough reasoning on hard problems", budget: 12288 },
  { id: "xhigh", label: "Extra High", desc: "Maximum reasoning — slower, spends more tokens", budget: 24576 },
];

export const DEFAULT_EFFORT: EffortId = "off";

// Answer capacity reserved on top of the thinking budget. Matches the historical
// 4096 max_tokens so "Off" behaves exactly as before.
const ANSWER_ROOM = 4096;

export function effortById(id: string): EffortLevel {
  return EFFORT_LEVELS.find((e) => e.id === id) || EFFORT_LEVELS[0];
}

// modelBlock renders the spec's `model:` line for a provider/model/effort. When
// effort is off it is byte-for-byte the old line (no thinking block, max 4096).
function modelBlock(provider: string, model: string, effort: EffortId): string {
  const e = effortById(effort);
  if (e.budget <= 0) {
    return `model: { provider: ${provider}, id: ${model}, max_tokens: ${ANSWER_ROOM} }`;
  }
  const max = ANSWER_ROOM + e.budget;
  return `model: { provider: ${provider}, id: ${model}, max_tokens: ${max}, thinking: { enabled: true, budget_tokens: ${e.budget} } }`;
}

// effortFromSpec reads the effort back out of a spec so the pill shows the right
// level for a session we built. A spec with no thinking block reads as "off".
export function effortFromSpec(spec: string): EffortId {
  const m = spec.match(/budget_tokens:\s*(\d+)/);
  if (!m) return "off";
  const b = Number(m[1]);
  const lvl = EFFORT_LEVELS.find((e) => e.budget === b);
  return lvl ? lvl.id : "off";
}

// permissionsBlock returns the YAML `permissions:` list for an access level.
// `ask` allows the read-class tools by name and asks on everything else — the
// order matters (first matching rule wins), so the specific allows precede the
// catch-all ask.
function permissionsBlock(access: AccessId): string {
  if (access === "ask") {
    return [
      "permissions:",
      "  - { tool: read_file, action: allow }",
      "  - { tool: grep, action: allow }",
      "  - { tool: glob, action: allow }",
      "  - { tool: semantic_search, action: allow }",
      "  - { action: ask }",
    ].join("\n");
  }
  // full / acceptEdits / plan all keep an allow-all base; acceptEdits & plan
  // get their gating from `--mode`, so the rule list stays permissive.
  return "permissions:\n  - { action: allow }";
}

// ---- agent personas -------------------------------------------------------
//
// A persona is a curated main-agent shape (name / prompt / tool set); the
// composer's agent pill switches between them mid-session via `ar agent`
// (spec_changed at the next message). Model and access are orthogonal — they
// slot into whichever persona is active.
export interface Persona {
  id: string;
  label: string;
  desc: string;
  withWorker: boolean; // dev spawns worker sub-agents; the rest are solo
}

export const PERSONAS: Persona[] = [
  { id: "dev", label: "Dev", desc: "Full tools + worker sub-agents · default", withWorker: true },
  { id: "lead", label: "Team Lead", desc: "Drafts a team & coordinates it via messages", withWorker: false },
  { id: "auditor", label: "Auditor", desc: "Read-only, answers in one stern sentence", withWorker: false },
  { id: "reviewer", label: "Reviewer", desc: "Read + inspect, structured findings, no writes", withWorker: false },
  { id: "chat", label: "Chat", desc: "No tools — plain conversation", withWorker: false },
];

export const DEFAULT_PERSONA = "dev";

export function personaById(id: string): Persona {
  return PERSONAS.find((p) => p.id === id) || PERSONAS[0];
}

// personaFromSpec reads the spec's `name:` back into a persona id so the pill
// reflects a session we configured earlier (unknown names fall through).
export function personaFromSpec(spec: string): string | null {
  const m = spec.match(/^name:\s*([a-zA-Z0-9_-]+)/m);
  if (!m) return null;
  return PERSONAS.some((p) => p.id === m[1]) ? m[1] : null;
}

function personaBody(persona: string): string {
  switch (persona) {
    case "lead":
      // Team Lead (INC-12): drafts team members inline with spawn_agent{role:…}
      // and coordinates them with send_message. The prompt enforces a
      // spawn-then-kickoff-broadcast protocol so messages actually flow in all
      // three directions (lead→member, member↔member, member→parent) from a
      // one-line goal — models otherwise front-load everything at spawn time
      // and never message mid-flight. agents_dynamic opens the inline-role
      // face; agent_workspace: shared lets members collaborate on one tree.
      return `system_prompt: |
  你是工程团队 lead,带一支动态团队完成用户目标。**核心纪律:让消息真正
  在成员间流动,不要把全部指令都塞进 spawn 的 task 里。** 严格按此协议:

  1. 规划:据目标定 2-4 个角色(如 PM / 架构师 / SWE / Reviewer)。
  2. 先建人:对每个角色调 spawn_agent{role:{name,description,instructions}}
     (动态起草,不要用预定义 agent 名)。此时 instructions 只写"你的职责
     + 先待命,等 lead 的开工消息",task 写一句占位即可。记下每个 spawn
     返回的 child_session id。
  3. 开工广播(关键):所有成员建好后,用 send_message 给**每个**成员发一条
     开工消息,内含 ①它的详细任务;②全体队友的"名字→session_id"花名册;
     ③明确要求"要对接队友就 send_message(to=<队友的 session_id>) 直接联系,
     完成或有产出就 send_message(to='parent') 向我汇报"。
  4. 推进:成员汇报会以消息进入你的对话。评审环节让成员互发消息(例:SWE
     交付后你 send_message 通知 Reviewer 去评审、Reviewer 把结论 send_message
     发回 SWE);要某个已静止成员再做一轮时,直接 send_message 给它即可唤醒续做。
  5. 收尾:全部完成后向用户简洁汇总各成员产出与协作过程。
tools: [read_file, write_file, edit_file, bash, spawn_agent, kill]
agents_dynamic: true
agent_workspace: shared`;
    case "auditor":
      return `system_prompt: You are an auditor. Always open with "[AUDITOR]" and answer in one stern sentence.
tools: [read_file, bash]`;
    case "reviewer":
      return `system_prompt: |
  You are a code reviewer. Read the code, then produce structured findings
  (severity / file:line / failure scenario). Never modify the workspace.
tools: [read_file, bash]`;
    case "chat":
      return `system_prompt: You are a concise, helpful assistant.
tools: []`;
    default: // dev
      return `system_prompt: |
  You are a rigorous coding assistant. Follow the user's instructions
  exactly; when asked to start sub-agents, use the spawn_agent tool with
  the exact count and division of labor requested; use kill to cancel.
tools: [read_file, write_file, edit_file, bash, spawn_agent, kill, exit_plan_mode]
agents: [worker]`;
  }
}

// buildSpec produces the main agent spec (base.yaml) for the chosen persona +
// model + access level + reasoning effort. Defaults mirror the old DEFAULT_SPEC
// so behavior is unchanged when nothing is picked (effort "off").
export function buildSpec(opts: { provider: string; model: string; access: AccessId; persona?: string; effort?: EffortId }): string {
  const persona = opts.persona && PERSONAS.some((p) => p.id === opts.persona) ? opts.persona : DEFAULT_PERSONA;
  return `name: ${persona}
${modelBlock(opts.provider, opts.model, opts.effort || DEFAULT_EFFORT)}
${personaBody(persona)}
${permissionsBlock(opts.access)}
`;
}

// replaceModel swaps the `model:` block of an existing spec (provider/model plus
// the effort-derived max_tokens + thinking block), preserving the rest
// (system_prompt, tools, permissions). Used for mid-session model / effort
// switches where we keep everything else the session was configured with.
export function replaceModel(spec: string, provider: string, model: string, effort: EffortId = DEFAULT_EFFORT): string {
  const block = modelBlock(provider, model, effort);
  if (/^model:\s*\{[^}]*\}/m.test(spec)) {
    return spec.replace(/^model:\s*\{[^}]*\}[^\n]*$/m, block);
  }
  // No model line to replace (unusual) — prepend one after the name.
  return spec.replace(/^(name:.*)$/m, `$1\n${block}`);
}

// modelFromSpec reads back the provider/id currently declared in a spec so the
// pill can show the right label for a session we didn't build from a picker.
export function modelFromSpec(spec: string): { provider: string; id: string } | null {
  const m = spec.match(/model:\s*\{[^}]*provider:\s*([a-z0-9_-]+)[^}]*id:\s*([a-z0-9._-]+)/i);
  if (!m) return null;
  return { provider: m[1], id: m[2] };
}

// ---- worker sub-agent (unchanged sibling) ----
export const DEFAULT_WORKER = `name: worker
description: carries out investigation/edit tasks assigned by the parent and reports back
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: When the task is done, report your conclusions as concise bullet points.
tools: [read_file, bash]
`;

// Legacy export kept for the advanced YAML modal (unchanged escape hatch).
export const DEFAULT_SPEC = buildSpec({ provider: "gemini", model: "gemini-flash-latest", access: "full" });

// ---- drivers (goal / loop) --------------------------------------------------
//
// /goal and /loop are IterationDriver schedules, launched via `ar drive`
// (POST /api/runs kind:drive). A driver.yaml references a child agent spec
// (agent.yaml sibling) and carries the loop's task / schedule / verifiers.

export const DEFAULT_DRIVER_AGENT = `name: worker
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: You work in an iteration loop; each round advance the task one small step, self-check, and report concisely what you did this round.
tools: [read_file, write_file, bash]
permissions:
  - { action: allow }
`;

// buildGoalDriver: schedule "immediate" — re-iterate toward the goal until the
// verifier passes / budget spent / it stalls (Codex's Goal mode).
export function buildGoalDriver(opts: { task: string; maxIterations: number; verifier?: string; provider: string; model: string }): string {
  const verifiers = opts.verifier?.trim()
    ? `verifiers:\n  - { kind: command, command: ${JSON.stringify(opts.verifier.trim())} }`
    : `verifiers: []`;
  return `name: goal
schedule: immediate
agent_spec: agent.yaml
task: ${JSON.stringify(opts.task)}
max_iterations: ${opts.maxIterations}
${verifiers}
`;
}

// buildLoopDriver: schedule "interval" (fixed cadence) — Codex's Loop mode.
export function buildLoopDriver(opts: { task: string; interval: string; maxIterations: number; provider: string; model: string }): string {
  return `name: loop
schedule: interval
interval: ${JSON.stringify(opts.interval)}
agent_spec: agent.yaml
task: ${JSON.stringify(opts.task)}
max_iterations: ${opts.maxIterations}
verifiers: []
`;
}

// buildBestOfNDriver: schedule "parallel" — N attempts in isolated worktrees
// from one base snapshot, judged by the verifiers; the best verdict wins.
export function buildBestOfNDriver(opts: { task: string; n: number; verifier?: string; provider: string; model: string }): string {
  const verifiers = opts.verifier?.trim()
    ? `verifiers:\n  - { kind: command, command: ${JSON.stringify(opts.verifier.trim())} }`
    : `verifiers: []`;
  return `name: best-of-n
schedule: parallel
n: ${Math.max(2, opts.n)}
agent_spec: agent.yaml
task: ${JSON.stringify(opts.task)}
${verifiers}
`;
}

// buildDriverAgent produces the per-iteration child agent for the chosen model.
export function buildDriverAgent(opts: { provider: string; model: string }): string {
  return `name: worker
model: { provider: ${opts.provider}, id: ${opts.model}, max_tokens: 2048 }
system_prompt: You work in an iteration loop; each round advance the task one small step, self-check, and report concisely what you did this round.
tools: [read_file, write_file, bash]
permissions:
  - { action: allow }
`;
}

// Kept for the advanced RunModal (drive) escape hatch.
export const DEFAULT_DRIVER = `name: progress-loop
agent_spec: agent.yaml
task: Append one line describing this round's progress to progress.txt in the workspace (use bash or write_file); do not repeat the previous line.
max_iterations: 3
verifiers:
  - { kind: command, command: "test -f progress.txt" }
`;
