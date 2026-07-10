import type { Session } from "./types";

export interface ProjectGroup {
  key: string;
  label: string;
  workspace?: string;
  // Short parent-path hint shown when two groups share a label (W20).
  hint?: string;
  sessions: Session[];
}

export interface SidebarModel {
  pinned: Session[];
  projects: ProjectGroup[];
}

export function dedupeInspectNodes<T extends { session?: string; call_id?: string }>(nodes: T[]): T[] {
  const order: string[] = [];
  const unique = new Map<string, T>();
  nodes.forEach((node, index) => {
    const key = node.session || node.call_id || `anonymous-${index}`;
    if (!unique.has(key)) order.push(key);
    // Later inspect entries carry the freshest status after a child resumes.
    unique.set(key, node);
  });
  return order.map((key) => unique.get(key)!);
}

// scratchLabel turns an auto-created workspace basename into a friendly
// sidebar label: "ws-20260710-221530" → "Scratch · 07-10 22:15" (W2/W42).
// Covers the current readable names, the legacy raw-nanosecond ones, and
// fork worktrees. Returns "" when the name isn't an auto-created shape.
export function scratchLabel(base: string): string {
  let m = /^(?:ws|wt)-(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})/.exec(base);
  if (m) return `Scratch · ${m[2]}-${m[3]} ${m[4]}:${m[5]}`;
  m = /^(?:ws|wt)(\d{19})(?:-fork-[\w-]+)?$/.exec(base); // legacy UnixNano names
  if (m) {
    const d = new Date(Number(m[1].slice(0, 13)));
    if (!isNaN(d.getTime())) {
      const p = (n: number) => String(n).padStart(2, "0");
      return `Scratch · ${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
    }
    return "Scratch workspace";
  }
  return "";
}

export function projectLabel(workspace?: string): string {
  const clean = (workspace || "").trim().replace(/\/+$/, "");
  if (!clean) return "Other sessions";
  const parts = clean.split("/").filter(Boolean);
  const base = parts[parts.length - 1] || "Other sessions";
  return scratchLabel(base) ? "Scratch" : base;
}

export function scheduleLabel(schedule?: string): string {
  switch ((schedule || "immediate").toLowerCase()) {
    case "interval": return "Repeating";
    case "cron": return "Scheduled";
    case "parallel": return "Best of N";
    case "self_paced": return "Self-paced";
    default: return "Goal";
  }
}

function projectIdentity(workspace?: string): Pick<ProjectGroup, "key" | "label" | "workspace"> {
  const clean = (workspace || "").trim().replace(/\/+$/, "");
  const label = projectLabel(clean);
  // Auto-created WebUI workspaces use opaque timestamp names. Treat them as
  // one product-level Scratch project instead of leaking implementation ids.
  if (label === "Scratch") {
    return { key: "__scratch__", label: "Scratch", workspace: undefined };
  }
  return { key: clean || "__other__", label, workspace: clean || undefined };
}

export function buildSidebarModel(
  sessions: Session[],
  options: {
    pinned: string[];
    archived: string[];
    showArchived: boolean;
    query: string;
    titleOf: (session: Session) => string;
  },
): SidebarModel {
  const query = options.query.trim().toLowerCase();
  const visible = sessions.filter((session) => {
    if (session.kind === "driver") return false;
    if (!options.showArchived && options.archived.includes(session.id)) return false;
    if (!query) return true;
    return (
      options.titleOf(session).toLowerCase().includes(query) ||
      session.id.toLowerCase().includes(query) ||
      (session.workspace || "").toLowerCase().includes(query)
    );
  });

  const byId = new Map(visible.map((session) => [session.id, session]));
  const pinned = options.pinned
    .map((id) => byId.get(id))
    .filter((session): session is Session => !!session);
  const pinnedIds = new Set(pinned.map((session) => session.id));

  // Stable, predictable order (W8): session ids start with their creation
  // stamp, so newest-first is a plain reverse lexicographic sort. Groups are
  // built in that order too — a project group sorts by its newest session —
  // which keeps the sidebar from reshuffling on every poll.
  const ordered = [...visible].sort((a, b) => b.id.localeCompare(a.id));

  const groups = new Map<string, ProjectGroup>();
  for (const session of ordered) {
    if (pinnedIds.has(session.id)) continue;
    const identity = projectIdentity(session.workspace);
    const key = identity.key;
    if (!groups.has(key)) {
      groups.set(key, {
        ...identity,
        sessions: [],
      });
    }
    groups.get(key)!.sessions.push(session);
  }

  // Two different paths ending in the same basename ("ws", "workspace") are
  // indistinguishable by label alone — disambiguate with a shortened parent
  // path (W20).
  const byLabel = new Map<string, ProjectGroup[]>();
  for (const g of groups.values()) {
    byLabel.set(g.label, [...(byLabel.get(g.label) || []), g]);
  }
  for (const twins of byLabel.values()) {
    if (twins.length < 2) continue;
    for (const g of twins) {
      if (!g.workspace) continue;
      const parts = g.workspace.replace(/\/+$/, "").split("/").filter(Boolean);
      const parent = parts.length > 1 ? parts[parts.length - 2] : "";
      if (parent) g.hint = "…/" + parent;
    }
  }

  return { pinned, projects: [...groups.values()] };
}
