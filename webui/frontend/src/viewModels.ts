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

// deNoiseSegment strips a YYYYMMDD date token from a path segment while
// keeping the distinguishing remainder ("qa39-20260710-004434" →
// "qa39-004434"). The shared date between sibling QA dirs is pure noise; the
// prefix ("qa39") and the time suffix ("004434") are what tell them apart.
// Falls back to the raw segment when stripping would leave nothing (W4).
export function deNoiseSegment(segment: string): string {
  const stripped = segment
    .replace(/(^|[-_.])(20\d{6})(?=[-_.]|$)/g, (_match, sep) => sep)
    .replace(/[-_.]{2,}/g, (run) => run[0])
    .replace(/^[-_.]+|[-_.]+$/g, "");
  return stripped || segment;
}

// projectSubtitle derives a short disambiguating detail for one workspace,
// given the sibling workspaces that share its primary name (W4). Scratch
// workspaces are told apart by their own creation time (their parent dir is
// shared, e.g. /tmp); real projects by the shortest de-noised parent-path
// suffix that stays unique within the group.
export function projectSubtitle(workspace: string, siblings: string[]): string {
  const clean = (path?: string) => (path || "").trim().replace(/\/+$/, "");
  const ws = clean(workspace);
  const base = ws.split("/").filter(Boolean).pop() || "";
  const scratch = scratchLabel(base);
  if (scratch) return scratch.replace(/^Scratch · /, "");
  const parents = (path: string) => clean(path).split("/").filter(Boolean).slice(0, -1);
  const mine = parents(ws);
  const others = siblings.map(clean).filter((other) => other !== ws).map(parents);
  let pretty = "";
  for (let depth = 1; depth <= Math.max(1, mine.length); depth++) {
    pretty = mine.slice(Math.max(0, mine.length - depth)).map(deNoiseSegment).join("/");
    const collides = others.some((other) => other.slice(Math.max(0, other.length - depth)).map(deNoiseSegment).join("/") === pretty);
    if (pretty && !collides) return pretty;
  }
  return pretty;
}

// projectSubtitles is the batch form used by the sidebar and the composer's
// project picker: for a list of workspaces it returns a subtitle only for the
// ones whose primary name collides with another's. Uniquely-named workspaces
// are absent from the map, so callers render no subtitle for them (W4).
export function projectSubtitles(workspaces: string[]): Map<string, string> {
  const clean = (path?: string) => (path || "").trim().replace(/\/+$/, "");
  const items = workspaces.map(clean).filter(Boolean);
  const byName = new Map<string, string[]>();
  for (const ws of items) {
    const name = projectLabel(ws);
    byName.set(name, [...(byName.get(name) || []), ws]);
  }
  const out = new Map<string, string>();
  for (const group of byName.values()) {
    if (group.length < 2) continue;
    for (const ws of group) out.set(ws, projectSubtitle(ws, group));
  }
  return out;
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
  // indistinguishable by label alone — disambiguate with a short, de-noised
  // parent-path segment (W4). The full path stays in the row's tooltip.
  const byLabel = new Map<string, ProjectGroup[]>();
  for (const g of groups.values()) {
    byLabel.set(g.label, [...(byLabel.get(g.label) || []), g]);
  }
  for (const twins of byLabel.values()) {
    if (twins.length < 2) continue;
    const paths = twins.map((g) => g.workspace).filter((w): w is string => !!w);
    for (const g of twins) {
      if (!g.workspace) continue;
      g.hint = projectSubtitle(g.workspace, paths);
    }
  }

  return { pinned, projects: [...groups.values()] };
}
