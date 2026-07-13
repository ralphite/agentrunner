// Slash command table + parser, extracted from Composer so it is unit-testable
// on its own (the dispatch switch stays in Composer, which needs its state).

// A slash command: what the menu shows and what Enter/click does. `needsArgs`
// commands complete to "/name " and wait; the rest run immediately.
export interface SlashCmd {
  name: string;
  arg?: string;
  desc: string;
  variants: ("home" | "session")[];
  needsArgs?: boolean;
}

export const SLASH: SlashCmd[] = [
  { name: "goal", arg: "<goal>", desc: "Attach a goal — the agent keeps working until it's met", variants: ["home", "session"], needsArgs: true },
  { name: "loop", arg: "<prompt>", desc: "Start a run that repeats on a fixed cadence", variants: ["home", "session"], needsArgs: true },
  { name: "bestof", arg: "<prompt>", desc: "Run N isolated attempts, keep the best", variants: ["home", "session"], needsArgs: true },
  { name: "optimize", arg: "<draft>", desc: "Rewrite a draft prompt into a clearer instruction", variants: ["home", "session"], needsArgs: true },
  { name: "plan", desc: "Read-only planning mode — no changes", variants: ["home"] },
  { name: "compact", desc: "Summarize & shrink this conversation's context", variants: ["session"] },
  { name: "clear", desc: "Drop this conversation's context and start fresh", variants: ["session"] },
  { name: "mode", arg: "<default|acceptEdits>", desc: "Switch permission mode — acceptEdits auto-allows edits", variants: ["session"], needsArgs: true },
  { name: "diff", desc: "Show the workspace changes (git diff)", variants: ["session"] },
  { name: "fork", desc: "Fork into a new worktree from a checkpoint", variants: ["session"] },
  { name: "model", arg: "<id>", desc: "Switch the model", variants: ["home", "session"], needsArgs: true },
  { name: "reasoning", arg: "<level>", desc: "Set reasoning effort (off/light/medium/high/xhigh)", variants: ["home", "session"], needsArgs: true },
  { name: "interrupt", desc: "Stop the in-flight turn", variants: ["session"] },
  { name: "resume", desc: "Recover a crashed / interrupted session", variants: ["session"] },
];

// parseSlash recognizes "/name [rest]" against the table for a given variant.
// A needsArgs command with no rest returns null so the menu completes it
// instead of running empty.
export function parseSlash(text: string, variant: "home" | "session"): { cmd: string; rest: string } | null {
  const m = text.match(/^\/(\w+)(?:\s+([\s\S]*))?$/);
  if (!m) return null;
  const name = m[1].toLowerCase();
  const cmd = SLASH.find((c) => c.name === name && c.variants.includes(variant));
  if (!cmd) return null;
  const rest = (m[2] || "").trim();
  if (cmd.needsArgs && !rest) return null; // "/goal" alone → let the menu complete it
  return { cmd: name, rest };
}
