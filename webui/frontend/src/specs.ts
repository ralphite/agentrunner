// ---------------------------------------------------------------------------
// Spec / driver construction for the Codex-style composer.
//
// The product has no `--model` / `--access` flags: the model lives in the agent
// spec's `model:` block and the approval posture lives in its `permissions:`
// block (plus the `--mode` runtime override). So the composer's model and
// access pickers work by *building the spec text* here, then handing it to
// `ar new` / `ar agent` / `ar drive` exactly as the YAML editor modal does.
// ---------------------------------------------------------------------------

import auditorSpec from "./agents/auditor.yaml?raw";
import chatSpec from "./agents/chat.yaml?raw";
import devSpec from "./agents/dev.yaml?raw";
import leadSpec from "./agents/lead.yaml?raw";
import reviewerSpec from "./agents/reviewer.yaml?raw";
import workerSpec from "./agents/worker.yaml?raw";

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

// Ask-first by default (W15): a brand-new session should not silently run with
// nothing gated. Full access stays one click away and the composer remembers
// the user's last choice.
export const DEFAULT_ACCESS: AccessId = "ask";

export function accessById(id: string): AccessLevel {
  return ACCESS_LEVELS.find((a) => a.id === id) || ACCESS_LEVELS[0];
}

// runtimeModeTarget maps an access level to the `/mode` target the daemon
// accepts mid-session (INC-42's ValidTransition: default↔acceptEdits only),
// or null when the level can't be entered at runtime. "full" is a launch-time
// posture the runtime switch can't grant (it only sets the fold mode, not the
// spec's permission rules); "plan" exits through an exit_plan_mode approval,
// not this switch; bypass is start-time only. The session mode pill (INC-54)
// uses this to decide which rows are clickable vs. disabled-with-reason.
export function runtimeModeTarget(id: AccessId): "default" | "acceptEdits" | null {
  if (id === "ask") return "default";
  if (id === "acceptEdits") return "acceptEdits";
  return null;
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
export type EffortId = "light" | "medium" | "high" | "xhigh";

export interface EffortLevel {
  id: EffortId;
  label: string;
  desc: string;
  budget: number; // thinking budget tokens (0 ⇒ thinking off)
}

// No "off"/budget:0 level: gemini-flash-latest now rejects thinkingBudget:0 with
// INVALID_ARGUMENT (2026-07-21), so every session must think at least a little.
export const EFFORT_LEVELS: EffortLevel[] = [
  { id: "light", label: "Light", desc: "A little reasoning before answering", budget: 2048 },
  { id: "medium", label: "Medium", desc: "Balanced reasoning on most sessions", budget: 6144 },
  { id: "high", label: "High", desc: "Thorough reasoning on hard problems", budget: 12288 },
  { id: "xhigh", label: "Extra High", desc: "Maximum reasoning — slower, spends more tokens", budget: 24576 },
];

export const DEFAULT_EFFORT: EffortId = "medium";

// Answer capacity reserved on top of the thinking budget. Matches the historical
// 4096 max_tokens so "Off" behaves exactly as before.
const ANSWER_ROOM = 4096;

export function effortById(id: string): EffortLevel {
  return EFFORT_LEVELS.find((e) => e.id === id) || EFFORT_LEVELS.find((e) => e.id === DEFAULT_EFFORT) || EFFORT_LEVELS[0];
}

// modelBlock renders the spec's `model:` line for a provider/model/effort. When
// effort is off (and no override) it is byte-for-byte the old line (no thinking
// block, max 4096). `budgetOverride` (the model menu's Advanced → thinking
// budget override) wins over the effort preset when it is a positive number,
// letting power users dial an exact budget the presets don't cover.
function modelBlock(provider: string, model: string, effort: EffortId, budgetOverride?: number | null): string {
  let budget = budgetOverride != null && budgetOverride > 0 ? Math.floor(budgetOverride) : effortById(effort).budget;
  // Never emit a no-thinking block: gemini-flash-latest rejects thinkingBudget:0
  // (INVALID_ARGUMENT, 2026-07-21). Floor any non-positive budget to the default.
  if (budget <= 0) budget = effortById(DEFAULT_EFFORT).budget;
  const max = ANSWER_ROOM + budget;
  return `model: { provider: ${provider}, id: ${model}, max_tokens: ${max}, thinking: { enabled: true, budget_tokens: ${budget} } }`;
}

// effortFromSpec reads the effort back out of a spec so the pill shows the right
// level for a session we built. A spec with no thinking block reads as "off".
export function effortFromSpec(spec: string): EffortId {
  const m = spec.match(/budget_tokens:\s*(\d+)/);
  if (!m) return DEFAULT_EFFORT; // legacy no-thinking spec → show the default (no "off" level anymore)
  const b = Number(m[1]);
  const lvl = EFFORT_LEVELS.find((e) => e.budget === b);
  return lvl ? lvl.id : DEFAULT_EFFORT;
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
  spec: string; // full shipped YAML; model/access are the only picker overrides
}

export const PERSONAS: Persona[] = [
  { id: "dev", label: "Dev", desc: "Full tools + isolated worker sub-agents · default", withWorker: true, spec: devSpec },
  { id: "lead", label: "Team Lead", desc: "Drafts a team sharing one workspace · for collaboration", withWorker: false, spec: leadSpec },
  { id: "auditor", label: "Auditor", desc: "Read-only, answers in one stern sentence", withWorker: false, spec: auditorSpec },
  { id: "reviewer", label: "Reviewer", desc: "Read + inspect, structured findings, no writes", withWorker: false, spec: reviewerSpec },
  { id: "chat", label: "Chat", desc: "No tools — plain conversation", withWorker: false, spec: chatSpec },
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

// replaceTopLevelBlock edits one YAML top-level field without parsing and
// re-serializing the user's text. A block field owns its following indented
// lines; every other top-level line and comment remains byte-for-byte.
function replaceTopLevelBlock(spec: string, key: string, block: string): string {
  const lines = spec.replace(/\r\n/g, "\n").trimEnd().split("\n");
  const start = lines.findIndex((line) => line.startsWith(`${key}:`));
  if (start < 0) {
    const name = lines.findIndex((line) => line.startsWith("name:"));
    const at = name >= 0 ? name + 1 : 0;
    lines.splice(at, 0, ...block.split("\n"));
    return `${lines.join("\n")}\n`;
  }
  let end = start + 1;
  while (end < lines.length && (lines[end].trim() === "" || /^[ \t]/.test(lines[end]))) end += 1;
  lines.splice(start, end - start, ...block.split("\n"));
  return `${lines.join("\n")}\n`;
}

// buildSpec produces the main agent spec (base.yaml) for the chosen persona +
// model + access level + reasoning effort. Defaults mirror the old DEFAULT_SPEC
// so behavior is unchanged when nothing is picked (effort defaults to medium).
export function buildSpec(opts: { provider: string; model: string; access: AccessId; persona?: string; effort?: EffortId; budgetOverride?: number | null }): string {
  const persona = personaById(opts.persona || DEFAULT_PERSONA);
  const withModel = replaceModel(persona.spec, opts.provider, opts.model, opts.effort || DEFAULT_EFFORT, opts.budgetOverride);
  return replaceTopLevelBlock(withModel, "permissions", permissionsBlock(opts.access));
}

// replaceModel swaps the `model:` block of an existing spec (provider/model plus
// the effort-derived max_tokens + thinking block), preserving the rest
// (system_prompt, tools, permissions). Used for mid-session model / effort
// switches where we keep everything else the session was configured with.
export function replaceModel(spec: string, provider: string, model: string, effort: EffortId = DEFAULT_EFFORT, budgetOverride?: number | null): string {
  return replaceTopLevelBlock(spec, "model", modelBlock(provider, model, effort, budgetOverride));
}

// modelFromSpec reads back the provider/id currently declared in a spec so the
// pill can show the right label for a session we didn't build from a picker.
export function modelFromSpec(spec: string): { provider: string; id: string } | null {
  const m = spec.match(/model:\s*\{[^}]*provider:\s*([a-z0-9_-]+)[^}]*id:\s*([a-z0-9._-]+)/i);
  if (!m) return null;
  return { provider: m[1], id: m[2] };
}

// Dev's explicit sibling sub-agent spec. Its conservative 24-step limit is the
// INC-30/G25 guard against an isolated worker spinning on an unavailable file.
export const DEFAULT_WORKER = workerSpec;

// Legacy export kept for the advanced YAML modal (unchanged escape hatch).
export const DEFAULT_SPEC = buildSpec({ provider: "gemini", model: "gemini-flash-latest", access: "full" });

// ---- drivers (goal / loop) --------------------------------------------------
//
// /goal and /loop are IterationDriver schedules, launched via `ar drive`
// (POST /api/runs kind:drive). A driver.yaml references a child agent spec
// (agent.yaml sibling) and carries the loop's prompt / schedule / verifiers.

export const DEFAULT_DRIVER_AGENT = `name: worker
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 10240, thinking: { enabled: true, budget_tokens: 6144 } }
system_prompt: You work in an iteration loop; each round advance the goal one small step, self-check, and report concisely what you did this round.
tools: [read_file, write_file, bash]
permissions:
  - { action: allow }
`;

// buildGoalDriver: schedule "immediate" — re-iterate toward the goal until the
// verifier passes / budget spent / it stalls (Codex's Goal mode).
export function buildGoalDriver(opts: { prompt: string; maxIterations: number; verifier?: string; provider: string; model: string }): string {
  const verifiers = opts.verifier?.trim()
    ? `verifiers:\n  - { kind: command, command: ${JSON.stringify(opts.verifier.trim())} }`
    : `verifiers: []`;
  return `name: goal
schedule: immediate
agent_spec: agent.yaml
prompt: ${JSON.stringify(opts.prompt)}
max_iterations: ${opts.maxIterations}
${verifiers}
`;
}

// buildLoopDriver: schedule "interval" (fixed cadence) — Codex's Loop mode.
export function buildLoopDriver(opts: { prompt: string; interval: string; maxIterations: number; provider: string; model: string }): string {
  return `name: loop
schedule: interval
interval: ${JSON.stringify(opts.interval)}
agent_spec: agent.yaml
prompt: ${JSON.stringify(opts.prompt)}
max_iterations: ${opts.maxIterations}
verifiers: []
`;
}

// buildBestOfNDriver: schedule "parallel" — N attempts in isolated worktrees
// from one base snapshot, judged by the verifiers; the best verdict wins.
export function buildBestOfNDriver(opts: { prompt: string; n: number; verifier?: string; provider: string; model: string }): string {
  const verifiers = opts.verifier?.trim()
    ? `verifiers:\n  - { kind: command, command: ${JSON.stringify(opts.verifier.trim())} }`
    : `verifiers: []`;
  return `name: best-of-n
schedule: parallel
n: ${Math.max(2, opts.n)}
agent_spec: agent.yaml
prompt: ${JSON.stringify(opts.prompt)}
${verifiers}
`;
}

// buildDriverAgent produces the per-iteration child agent for the chosen model.
export function buildDriverAgent(opts: { provider: string; model: string }): string {
  return `name: worker
${modelBlock(opts.provider, opts.model, DEFAULT_EFFORT)}
system_prompt: You work in an iteration loop; each round advance the goal one small step, self-check, and report concisely what you did this round.
tools: [read_file, write_file, bash]
permissions:
  - { action: allow }
`;
}

// Kept for the advanced RunModal (drive) escape hatch.
export const DEFAULT_DRIVER = `name: progress-loop
agent_spec: agent.yaml
prompt: Append one line describing this round's progress to progress.txt in the workspace (use bash or write_file); do not repeat the previous line.
max_iterations: 3
verifiers:
  - { kind: command, command: "test -f progress.txt" }
`;
