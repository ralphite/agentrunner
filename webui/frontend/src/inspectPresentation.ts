import { friendlyStatus } from "./components/pill";

export interface InspectActivity {
  modelCalls: number;
  toolCalls: number;
  blocked: number;
  recentTools: { name: string; detail: string; blocked: boolean }[];
}

export interface InspectSummary {
  status: { text: string; cls: string };
  spec: string;
  model: string;
  mode: string;
  steps: number;
  turns: number;
  agents: number;
  usage: { billed: number; input: number; output: number; cache: number };
  waiting?: { title: string; subject: string };
  provider: string;
  modalities: string[];
  capabilities: string[];
  activity: InspectActivity;
}

function record(value: unknown): Record<string, any> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, any> : {};
}

function number(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function waitingSubject(waiting: Record<string, any>): string {
  const args = record((() => {
    if (typeof waiting.args !== "string") return waiting.args;
    try { return JSON.parse(waiting.args); } catch { return {}; }
  })());
  return String(args.command || args.path || args.url || waiting.tool || "Review the pending request");
}

function modeLabel(mode: unknown): string {
  switch (String(mode || "default").toLowerCase()) {
    case "plan": return "Plan (read-only)";
    case "acceptedits":
    case "accept_edits": return "Auto-accept edits";
    case "full": return "Full access";
    default: return "From agent specification";
  }
}

export function summarizeInspect(raw: unknown): InspectSummary {
  const data = record(raw);
  const waiting = record(data.waiting);
  const usage = record(data.usage);
  const provider = record(data.provider_capabilities);
  const capMap = record(provider.capabilities);
  const entries = Array.isArray(data.entries) ? data.entries.map(record) : [];
  const tools = entries.filter((entry) => entry.kind === "tool" && entry.name);
  const reason = waiting.kind === "approval"
    ? "waiting:approval"
    : String(data.reason || data.status || "unknown");

  return {
    status: friendlyStatus(reason),
    spec: String(data.spec || "Default agent"),
    model: String(data.model || provider.model || "Unknown model"),
    mode: modeLabel(data.mode),
    steps: number(data.gen_steps),
    turns: number(data.turns),
    agents: Array.isArray(data.children) ? data.children.length : 0,
    usage: {
      billed: number(usage.billed) || number(usage.input_tokens) + number(usage.output_tokens),
      input: number(usage.input_tokens),
      output: number(usage.output_tokens),
      cache: number(usage.cache_read) + number(usage.cache_write),
    },
    waiting: waiting.kind ? {
      title: waiting.kind === "approval" ? "Approval required" : `Waiting for ${String(waiting.kind)}`,
      subject: waitingSubject(waiting),
    } : undefined,
    provider: String(provider.provider || "").trim(),
    modalities: Array.isArray(provider.input_modalities) ? provider.input_modalities.map(String) : [],
    capabilities: Object.entries(capMap).filter(([, enabled]) => enabled === true).map(([name]) => name.replaceAll("_", " ")),
    activity: {
      modelCalls: entries.filter((entry) => entry.kind === "llm").length,
      toolCalls: tools.length,
      blocked: entries.filter((entry) => entry.verdict === "deny").length,
      recentTools: tools.slice(-6).reverse().map((entry) => ({
        name: String(entry.name).replaceAll("_", " "),
        detail: String(entry.detail || ""),
        blocked: entry.verdict === "deny",
      })),
    },
  };
}

export function compactCount(value: number): string {
  return new Intl.NumberFormat(undefined, { notation: value >= 1000 ? "compact" : "standard", maximumFractionDigits: 1 }).format(value);
}
