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

export const DEFAULT_ACCESS: AccessId = "full";

export function accessById(id: string): AccessLevel {
  return ACCESS_LEVELS.find((a) => a.id === id) || ACCESS_LEVELS[0];
}

export function modelById(provider: string, id: string): ModelChoice | undefined {
  return MODELS.find((m) => m.provider === provider && m.id === id);
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

// buildSpec produces the main agent spec (base.yaml) for the chosen model +
// access level. Mirrors the old DEFAULT_SPEC so behavior is unchanged when the
// defaults are picked.
export function buildSpec(opts: { provider: string; model: string; maxTokens?: number; access: AccessId }): string {
  const max = opts.maxTokens && opts.maxTokens > 0 ? opts.maxTokens : 4096;
  return `name: dev
model: { provider: ${opts.provider}, id: ${opts.model}, max_tokens: ${max} }
system_prompt: |
  You are a rigorous coding assistant. Follow the user's instructions
  exactly; when asked to start sub-agents, use the spawn_agent tool with
  the exact count and division of labor requested; use kill to cancel.
tools: [read_file, write_file, edit_file, bash, spawn_agent, kill]
agents: [worker]
${permissionsBlock(opts.access)}
`;
}

// replaceModel swaps only the `model:` line of an existing spec, preserving the
// rest (system_prompt, tools, permissions). Used for mid-session model switches
// where we want to keep everything else the session was configured with.
export function replaceModel(spec: string, provider: string, model: string, maxTokens?: number): string {
  const max = maxTokens && maxTokens > 0 ? maxTokens : undefined;
  const line = (m?: number) =>
    `model: { provider: ${provider}, id: ${model}${m ? `, max_tokens: ${m}` : ""} }`;
  if (/^model:\s*\{[^}]*\}/m.test(spec)) {
    // Preserve an existing max_tokens if present and none was supplied.
    return spec.replace(/^model:\s*\{[^}]*\}[^\n]*$/m, (prev) => {
      const mt = max ?? (prev.match(/max_tokens:\s*(\d+)/)?.[1] ? Number(prev.match(/max_tokens:\s*(\d+)/)![1]) : 4096);
      return line(mt);
    });
  }
  // No model line to replace (unusual) — prepend one after the name.
  return spec.replace(/^(name:.*)$/m, `$1\n${line(max ?? 4096)}`);
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
