// Composer choices and Agent-definition projection.
//
// Agent YAML is owned by the runtime catalog (`GET /api/agents`), never by the
// frontend bundle. The browser may apply the launch-time permission posture to
// the selected definition, but model/provider/effort remain separate request
// fields and are never written into YAML.

import type { AgentCatalogEntry } from "./types";

export interface ModelChoice {
  provider: string;
  id: string;
  label: string;
  sub: string;
}

export const MODELS: ModelChoice[] = [
  { provider: "gemini", id: "gemini-flash-latest", label: "Gemini Flash", sub: "Fast · default" },
  { provider: "gemini", id: "gemini-flash-lite-latest", label: "Gemini Flash Lite", sub: "Fastest, lightest" },
  { provider: "gemini", id: "gemini-3.5-flash", label: "Gemini 3.5 Flash", sub: "Newest flash" },
  { provider: "gemini", id: "gemini-pro-latest", label: "Gemini Pro", sub: "Most capable" },
  { provider: "anthropic", id: "claude-sonnet-5", label: "Claude Sonnet 5", sub: "Anthropic · needs creds" },
];

export const DEFAULT_MODEL = MODELS[0];

export function modelById(provider: string, id: string): ModelChoice | undefined {
  return MODELS.find((m) => m.provider === provider && m.id === id);
}

export type EffortId = "light" | "medium" | "high" | "xhigh";

export interface EffortLevel {
  id: EffortId;
  label: string;
  desc: string;
}

export const EFFORT_LEVELS: EffortLevel[] = [
  { id: "light", label: "Light", desc: "A little reasoning before answering" },
  { id: "medium", label: "Medium", desc: "Balanced reasoning on most sessions" },
  { id: "high", label: "High", desc: "Thorough reasoning on hard problems" },
  { id: "xhigh", label: "Extra High", desc: "Maximum reasoning — slower, spends more tokens" },
];

export const DEFAULT_EFFORT: EffortId = "medium";

export function effortById(id: string): EffortLevel {
  return EFFORT_LEVELS.find((e) => e.id === id) || EFFORT_LEVELS.find((e) => e.id === DEFAULT_EFFORT)!;
}

export type AccessId = "full" | "ask" | "acceptEdits" | "plan";

export interface AccessLevel {
  id: AccessId;
  label: string;
  desc: string;
  mode: string;
  risk: "high" | "med" | "low";
}

export const ACCESS_LEVELS: AccessLevel[] = [
  { id: "full", label: "Full access", desc: "Nothing is gated — the agent acts freely", mode: "", risk: "high" },
  { id: "ask", label: "Ask to approve", desc: "Reads run freely; edits, shell & network ask first", mode: "", risk: "low" },
  { id: "acceptEdits", label: "Auto-accept edits", desc: "File edits apply automatically; other gated tools ask", mode: "acceptEdits", risk: "med" },
  { id: "plan", label: "Plan · read-only", desc: "No changes — the agent plans and proposes only", mode: "plan", risk: "low" },
];

export const DEFAULT_ACCESS: AccessId = "ask";

export function accessById(id: string): AccessLevel {
  return ACCESS_LEVELS.find((a) => a.id === id) || ACCESS_LEVELS[0];
}

export function runtimeModeTarget(id: AccessId): "default" | "acceptEdits" | null {
  if (id === "ask") return "default";
  if (id === "acceptEdits") return "acceptEdits";
  return null;
}

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
  return "permissions:\n  - { action: allow }";
}

export const DEFAULT_PERSONA = "dev";

const AGENT_LABELS: Record<string, string> = {
  dev: "Dev",
  lead: "Team Lead",
  auditor: "Auditor",
  reviewer: "Reviewer",
  chat: "Chat",
  worker: "Worker",
  explore: "Explore",
  plan: "Plan",
};

export function agentLabel(name: string): string {
  return AGENT_LABELS[name] || name.replace(/[-_]+/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

export function agentById(catalog: AgentCatalogEntry[], id: string): AgentCatalogEntry | undefined {
  return catalog.find((agent) => agent.name === id) || catalog[0];
}

export function personaFromSpec(spec: string): string | null {
  const m = spec.match(/^name:\s*([a-zA-Z0-9_-]+)/m);
  return m ? m[1] : null;
}

// Edits one YAML top-level block without reserializing the rest of the
// runtime-provided definition. Comments and prompt formatting stay intact.
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

// Browser-local compatibility only: sessions created by pre-INC-96 Web UI
// builds may have remembered a model block inside their Agent YAML. Strip it
// before the advanced editor resubmits the definition.
export function stripLegacyModel(spec: string): string {
  const lines = spec.replace(/\r\n/g, "\n").trimEnd().split("\n");
  const start = lines.findIndex((line) => line.startsWith("model:"));
  if (start < 0) return spec;
  let end = start + 1;
  while (end < lines.length && (lines[end].trim() === "" || /^[ \t]/.test(lines[end]))) end += 1;
  lines.splice(start, end - start);
  return `${lines.join("\n")}\n`;
}

export function legacyModelFromSpec(spec: string): { provider: string; model: string; effort: EffortId } | null {
  const selection = spec.match(/model:\s*\{[^}]*provider:\s*([a-z0-9_-]+)[^}]*id:\s*([a-z0-9._/-]+)/i);
  if (!selection) return null;
  const budget = Number(spec.match(/budget_tokens:\s*(\d+)/)?.[1] || 6144);
  const effort = budget === 2048 ? "light" : budget === 12288 ? "high" : budget === 24576 ? "xhigh" : "medium";
  return { provider: selection[1], model: selection[2], effort };
}

export function buildSpec(opts: { agent: AgentCatalogEntry; access: AccessId }): string {
  return replaceTopLevelBlock(opts.agent.yaml, "permissions", permissionsBlock(opts.access));
}

export function buildGoalDriver(opts: { prompt: string; maxIterations: number; verifier?: string }): string {
  const verifiers = opts.verifier?.trim()
    ? `verifiers:\n  - { kind: command, command: ${JSON.stringify(opts.verifier.trim())} }`
    : `verifiers: []`;
  return `name: goal
schedule: immediate
agent_spec: worker
prompt: ${JSON.stringify(opts.prompt)}
max_iterations: ${opts.maxIterations}
${verifiers}
`;
}

export function buildLoopDriver(opts: { prompt: string; interval: string; maxIterations: number }): string {
  return `name: loop
schedule: interval
interval: ${JSON.stringify(opts.interval)}
agent_spec: worker
prompt: ${JSON.stringify(opts.prompt)}
max_iterations: ${opts.maxIterations}
verifiers: []
`;
}

export function buildBestOfNDriver(opts: { prompt: string; n: number; verifier?: string }): string {
  const verifiers = opts.verifier?.trim()
    ? `verifiers:\n  - { kind: command, command: ${JSON.stringify(opts.verifier.trim())} }`
    : `verifiers: []`;
  return `name: best-of-n
schedule: parallel
n: ${Math.max(2, opts.n)}
agent_spec: worker
prompt: ${JSON.stringify(opts.prompt)}
${verifiers}
`;
}

export const DEFAULT_DRIVER = `name: progress-loop
agent_spec: worker
prompt: Append one line describing this round's progress to progress.txt in the workspace (use bash or write_file); do not repeat the previous line.
max_iterations: 3
verifiers:
  - { kind: command, command: "test -f progress.txt" }
`;
