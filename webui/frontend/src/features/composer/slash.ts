// Slash command table + parser, extracted from Composer so it is unit-testable
// on its own (the dispatch switch stays in Composer, which needs its state).

// A slash command: what the menu shows and what Enter/click does. `needsArgs`
// commands complete to "/name " and wait; the rest run immediately.
// `group` separates the built-in table from the workspace's dynamic surface
// (custom commands and skills — both expand server-side at ingest, so
// selecting one only completes "/name " and the plain send does the rest).
export interface SlashCmd {
  name: string;
  arg?: string;
  desc: string;
  variants: ("home" | "session")[];
  needsArgs?: boolean;
  group?: "builtin" | "command" | "skill";
  source?: string;
}

// The /api/slash payload: the workspace's custom commands and skills.
export interface SlashCatalog {
  commands?: { name: string; description?: string }[] | null;
  skills?: { name: string; description?: string; source?: string }[] | null;
}

// dynamicSlash turns a catalog into menu rows. Both kinds complete to
// "/name " (args optional, ingest expands the body either way); a name
// already taken by a built-in is dropped — the built-in runs client-side and
// must stay reachable.
export function dynamicSlash(catalog: SlashCatalog | null): SlashCmd[] {
  if (!catalog) return [];
  const taken = new Set(SLASH.map((c) => c.name));
  const out: SlashCmd[] = [];
  for (const c of catalog.commands || []) {
    if (taken.has(c.name)) continue;
    taken.add(c.name);
    out.push({ name: c.name, desc: c.description || "Workspace command", variants: ["home", "session"], needsArgs: true, group: "command", source: "workspace" });
  }
  for (const s of catalog.skills || []) {
    if (taken.has(s.name)) continue; // a same-named command shadows the skill, matching ingest
    taken.add(s.name);
    out.push({ name: s.name, desc: s.description || "Skill", variants: ["home", "session"], needsArgs: true, group: "skill", source: s.source });
  }
  return out;
}

export const SLASH: SlashCmd[] = [
  { name: "goal", arg: "<goal>", desc: "Attach a goal — the agent keeps working until it's met", variants: ["home", "session"], needsArgs: true },
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
