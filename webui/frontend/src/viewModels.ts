import type { Session } from "./types";

export interface ProjectGroup {
  key: string;
  label: string;
  workspace?: string;
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

export function projectLabel(workspace?: string): string {
  const clean = (workspace || "").trim().replace(/\/+$/, "");
  if (!clean) return "Other sessions";
  const parts = clean.split("/").filter(Boolean);
  return parts[parts.length - 1] || "Other sessions";
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

  const groups = new Map<string, ProjectGroup>();
  for (const session of visible) {
    if (pinnedIds.has(session.id)) continue;
    const workspace = (session.workspace || "").trim() || undefined;
    const key = workspace || "__other__";
    if (!groups.has(key)) {
      groups.set(key, {
        key,
        label: projectLabel(workspace),
        workspace,
        sessions: [],
      });
    }
    groups.get(key)!.sessions.push(session);
  }

  return { pinned, projects: [...groups.values()] };
}
