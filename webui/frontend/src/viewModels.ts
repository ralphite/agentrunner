import type { ProjectDef, Session } from "./types";
import { friendlyStatus, sessionFriendlyStatus } from "./components/pill";
import { sessionDate } from "./time";

export interface ProjectGroup {
  key: string; // a workspace path (derived group) or "project:<id>" (explicit)
  label: string;
  workspace?: string; // explicit: folders[0] (the primary); derived: the path
  sessions: Session[];
  // Explicit-project fields (INC-104); absent on derived groups.
  projectId?: string;
  folders?: string[];
  order?: number; // manual sort rank (1-based; 0/absent = unranked)
  createdAt?: number; // recency stand-in while the project has no sessions
  missing?: string[]; // folders currently absent from disk
}

// The overlay key for an explicit project's cosmetic state. Workspace paths
// always start with "/", so the prefix can never collide with a derived key.
export const PROJECT_KEY_PREFIX = "project:";
export const projectGroupKey = (id: string) => PROJECT_KEY_PREFIX + id;

export type SidebarOrganize = "by-project" | "one-list";
export type SidebarSort = "priority" | "updated" | "manual";

export interface SidebarModel {
  pinned: Session[];
  projects: ProjectGroup[];
  // Sessions without a workspace are not a project: rendering a folder would
  // incorrectly assert that they live in a directory on disk.
  workspaceLessSessions: Session[];
}

// sessionUpdatedDate resolves the journal-backed activity time. Older rows
// from a pre-INC-94 backend lack `updatedAt`, so their UTC creation stamp is a
// truthful, stable fallback rather than an invented current time.
export function sessionUpdatedDate(session: Pick<Session, "id" | "updatedAt">): Date | null {
  if (session.updatedAt) {
    const updated = new Date(session.updatedAt);
    if (!isNaN(updated.getTime())) return updated;
  }
  return sessionDate(session.id);
}

function sessionUpdatedNanoKey(session: Pick<Session, "updatedAt">): string | null {
  // Go emits UTC RFC3339Nano. Normalize the optional fraction to nine digits
  // so lexicographic order preserves sub-millisecond journal writes that a JS
  // Date would otherwise collapse onto the same millisecond.
  const match = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?Z$/.exec(session.updatedAt || "");
  return match ? `${match[1]}.${(match[2] || "").padEnd(9, "0")}Z` : null;
}

export function compareSessionsByUpdate(a: Session, b: Session): number {
  const aNano = sessionUpdatedNanoKey(a);
  const bNano = sessionUpdatedNanoKey(b);
  if (aNano && bNano) return bNano.localeCompare(aNano) || b.id.localeCompare(a.id);
  const aTime = sessionUpdatedDate(a)?.getTime() ?? 0;
  const bTime = sessionUpdatedDate(b)?.getTime() ?? 0;
  return bTime - aTime || b.id.localeCompare(a.id);
}

// ProjectOverlay is the client view of one project's server-side overlay
// (INC-53): a custom display name, folded state, and last-opened time. Mirrors
// ProjectMeta in types.ts; kept structural here so the pure helpers stay
// dependency-free and unit-testable.
export interface ProjectOverlay {
  displayName?: string;
  folded?: boolean;
  lastOpened?: number;
}

// projectDisplayName resolves the label a project group renders: the user's
// custom overlay name when set (trimmed), else the journal-derived label. The
// overlay only renames the group — it never changes which sessions belong to
// it (grouping stays keyed on workspace, DESIGN §12).
export function projectDisplayName(project: ProjectGroup, overlay?: ProjectOverlay): string {
  const custom = (overlay?.displayName || "").trim();
  return custom || project.label;
}

// visibleProjectSessions decides which sessions a project group shows given its
// persisted fold state, the local "show all" toggle, an active search, and the
// currently open session. A folded group hides its sessions entirely — but
// search overrides fold so a match is never hidden. An unfolded group shows all
// when expanded or searching, otherwise the first `cap`.
//
// INC-90: the current session overrides only the automatic row cap, never the
// user's explicit project fold. A deep link or ⌘K jump still brings a current
// row past the six-row cap into view while the group is open. Once the user
// folds that group, the heading remains the navigation anchor and every row is
// hidden until they expand it again.
export function visibleProjectSessions(
  project: ProjectGroup,
  opts: { folded?: boolean; expanded?: boolean; searching?: boolean; cap?: number; current?: string },
): Session[] {
  const cap = opts.cap ?? 6;
  if (opts.folded && !opts.searching) return [];
  const current = opts.current ? project.sessions.find((session) => session.id === opts.current) : undefined;
  if (opts.expanded || opts.searching) return project.sessions;
  const shown = project.sessions.slice(0, cap);
  // Appended at the tail rather than sorted in: the cap window stays exactly
  // what it was, so the rows above the current one never shuffle under it.
  if (current && !shown.includes(current)) shown.push(current);
  return shown;
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

// AgentRunner forks append one `-branch-YYYYMMDD-HHMMSS` segment per hop to
// the original workspace basename. The full chain is useful on disk but is not
// a project name, so the sidebar keeps the stable root only. Restrict this to
// AgentRunner's managed worktree root so a real repository with a timestamped
// name is never rewritten.
function managedWorktreeLineage(workspace: string): { label: string; detail: string } | null {
  const clean = workspace.trim().replace(/\/+$/, "");
  if (!clean.includes("/agentrunner/worktrees/")) return null;
  const base = clean.split("/").filter(Boolean).pop() || "";
  const hops = [...base.matchAll(/-([^-]+)-(20\d{6})-(\d{6})/g)];
  if (hops.length === 0) return null;
  const first = hops[0].index ?? -1;
  if (first <= 0 || hops.map((hop) => hop[0]).join("") !== base.slice(first)) return null;
  const latest = hops[hops.length - 1];
  const date = latest[2];
  const time = latest[3];
  return {
    label: base.slice(0, first),
    detail: `${date.slice(4, 6)}-${date.slice(6, 8)} ${time.slice(0, 2)}:${time.slice(2, 4)}`,
  };
}

// projectLabel names the project a workspace path belongs to — and says nothing
// when there is no workspace.
//
// SB-13: it used to answer "Other sessions" for the empty path, which every
// caller then rendered as if it were a real project (a folder-icon group in the
// rail, a project hint in the palette, a chip on the Scheduled row). It is not a
// project; it is the absence of one. The empty string is the honest answer, and
// it is falsy — so the `{hint && …}` / `.filter(Boolean)` guards the call sites
// already have now do the right thing instead of painting a fiction.
export function projectLabel(workspace?: string): string {
  const clean = (workspace || "").trim().replace(/\/+$/, "");
  if (!clean) return "";
  const parts = clean.split("/").filter(Boolean);
  const base = parts[parts.length - 1] || "";
  const lineage = managedWorktreeLineage(clean);
  if (lineage) return lineage.label;
  // INC-78: an auto-created workspace is its own project everywhere — the
  // palette hint, Scheduled chips, and the rail must name the SAME group
  // ("Scratch · 07-18 18:33"), not collapse some surfaces back to a bare
  // "Scratch" that no longer exists as a group (QA-0719 review #14).
  return scratchLabel(base) || base;
}

// isScratchWorkspace: does this path name an auto-created scratch dir? The
// seed/filter call sites need the judgement, not the label's exact string.
export function isScratchWorkspace(workspace?: string): boolean {
  const clean = (workspace || "").trim().replace(/\/+$/, "");
  const base = clean.split("/").filter(Boolean).pop() || "";
  return !!scratchLabel(base);
}

// AgentRunner-created worktrees live below the shared data root's
// `agentrunner/worktrees/` directory. The sidebar needs this cheap, durable
// identity signal while rendering hundreds of rows; it must not run git once
// per session or guess from a generic `worktree` basename elsewhere on disk.
export function isManagedWorktreeWorkspace(workspace?: string): boolean {
  const clean = (workspace || "").trim().replace(/\\/g, "/").replace(/\/+$/, "");
  return clean.includes("/agentrunner/worktrees/");
}

// scheduledUnread returns the ids of driver (scheduled) sessions that carry
// new activity the user hasn't opened. It is the single source behind the
// Scheduled nav dot (E3) and the Scheduled page's "Mark all as read" (F2).
// Runs (Run[]) hold no per-item unread state, so only driver sessions
// participate — keeping the badge honest about what it actually tracks.
export function scheduledUnread(sessions: Session[], unread: string[]): string[] {
  const flagged = new Set(unread);
  return sessions
    .filter((session) => session.kind === "driver" && flagged.has(session.id))
    .map((session) => session.id);
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

// sessionNeedsAttention decides whether a session's status calls for the user:
// waiting on an approval, stranded/needing recovery, a hit iteration/step/
// budget limit (all "stranded"), or a crash. It reuses friendlyStatus so the
// Scheduled list and the command palette agree with the sidebar's dot colours
// (INC-41 W7/W8).
export function sessionNeedsAttention(session: string | Pick<Session, "status" | "attention">): boolean {
  const cls = typeof session === "string" ? friendlyStatus(session).cls : sessionFriendlyStatus(session).cls;
  return cls === "appr" || cls === "stranded" || cls === "crash";
}

// quickSwitchSessions builds the ⌘1..9 quick-switch list shared by the command
// palette's badges and the global cmd-digit key binding (INC-41 W8). It covers
// conversational sessions only — drivers live on Scheduled — and drops archived
// ones. Attention-worthy sessions float to the front so they claim the lowest
// ⌘-numbers; the rest follow newest-first (session ids are creation stamps, so
// that is a plain descending id sort). Capped at nine: there are nine digits.
export function quickSwitchSessions(sessions: Session[], opts: { archived?: string[] } = {}): Session[] {
  const archived = new Set(opts.archived || []);
  const candidates = sessions.filter((s) => s.kind !== "driver" && !archived.has(s.id));
  const byRecency = [...candidates].sort((a, b) => b.id.localeCompare(a.id));
  const attention = byRecency.filter((s) => sessionNeedsAttention(s));
  const rest = byRecency.filter((s) => !sessionNeedsAttention(s));
  return [...attention, ...rest].slice(0, 9);
}

// projectIdentity is only ever called with a *real* (non-empty) workspace —
// workspace-less sessions never become a group at all (SB-13, see below).
function projectIdentity(clean: string): Pick<ProjectGroup, "key" | "label" | "workspace"> {
  // Auto-created WebUI workspaces used to be pooled into one "__scratch__"
  // aggregate, which mixed unrelated projects in a single folder (INC-78,
  // user adjudication 2026-07-19). Every workspace is its own project now,
  // keyed on its real path; projectLabel already hides the implementation id
  // behind "Scratch · <created>", and the INC-53 overlay rename gives it a
  // proper name.
  return { key: clean, label: projectLabel(clean), workspace: clean };
}

export function buildSidebarModel(
  sessions: Session[],
  options: {
    pinned: string[];
    archived: string[];
    showArchived: boolean;
    query: string;
    titleOf: (session: Session) => string;
    // Explicit project registry (INC-104). A session whose workspace exactly
    // matches one of a project's folders belongs to that project's group; the
    // per-workspace derived groups remain the fallback for everything else.
    projectDefs?: ProjectDef[];
    // "one-list" flattens the rail: no project groups, every non-pinned
    // session in the flat section, newest first (INC-104 organize menu).
    organize?: SidebarOrganize;
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

  // Journal mtime is the durable activity clock behind the paged API. Sorting
  // every sidebar partition from it makes an old session with a new turn rise
  // immediately; group insertion order then makes project recency equal the
  // maximum update time of its member sessions. ID is only a legacy/tie fallback.
  const ordered = [...visible].sort(compareSessionsByUpdate);
  const requestedPins = new Set(options.pinned);
  const pinned = ordered.filter((session) => requestedPins.has(session.id));
  const pinnedIds = new Set(pinned.map((session) => session.id));

  if (options.organize === "one-list") {
    // The flat section IS the sidebar in this mode. Pinned still wins — a
    // session appears in exactly one section, never two.
    return {
      pinned,
      projects: [],
      workspaceLessSessions: ordered.filter((session) => !pinnedIds.has(session.id)),
    };
  }

  const folderToProject = new Map<string, ProjectDef>();
  for (const def of options.projectDefs ?? []) {
    for (const folder of def.folders) {
      const clean = folder.trim().replace(/\/+$/, "");
      // First declaration wins; the server already refuses duplicate folders,
      // so this is only a defensive tiebreak.
      if (clean && !folderToProject.has(clean)) folderToProject.set(clean, def);
    }
  }
  const explicitGroup = (def: ProjectDef): ProjectGroup => ({
    key: projectGroupKey(def.id),
    label: def.name,
    workspace: def.folders[0],
    projectId: def.id,
    folders: def.folders,
    order: def.order,
    createdAt: def.createdAt,
    missing: def.missing,
    sessions: [],
  });

  const groups = new Map<string, ProjectGroup>();
  // Workspace-less sessions stay out of the project map entirely. Grouping
  // them under a synthetic "Other sessions" folder made the rail claim a
  // directory that does not exist; they belong to no project, so they come back
  // as a flat list under its own `Sessions` heading. Pinned still wins: a
  // session appears in exactly one section, never two.
  const workspaceLessSessions: Session[] = [];
  for (const session of ordered) {
    if (pinnedIds.has(session.id)) continue;
    const clean = (session.workspace || "").trim().replace(/\/+$/, "");
    if (!clean) {
      workspaceLessSessions.push(session);
      continue;
    }
    const def = folderToProject.get(clean);
    const identity = def ? explicitGroup(def) : { ...projectIdentity(clean), sessions: [] };
    const key = identity.key;
    if (!groups.has(key)) {
      groups.set(key, identity);
    }
    groups.get(key)!.sessions.push(session);
  }

  // Projects with no sessions yet still exist — the user declared them. They
  // render with a "No chats" row. A live search stays scoped to sessions
  // (title/id/workspace), so empty groups sit this one out.
  if (!query) {
    for (const def of options.projectDefs ?? []) {
      const key = projectGroupKey(def.id);
      if (!groups.has(key)) groups.set(key, explicitGroup(def));
    }
  }

  return { pinned, projects: [...groups.values()], workspaceLessSessions };
}

// orderProjectGroups is the one place project-group ordering lives (INC-94 →
// INC-104). Group recency is the FIRST member session — buildSidebarModel
// inserts groups while walking the update-ordered session list, so the first
// member is the group's newest and this reproduces the historical
// insertion-order behaviour exactly. An empty explicit project stands on its
// createdAt.
//
// - "priority": pinned groups first, then recency (the pre-INC-104 default).
// - "updated":  pure recency; pins keep their glyph but stop sorting.
// - "manual":   explicit projects by their saved order (1-based; 0 =
//   unranked), everything unranked — including every derived group — sinks
//   below by recency. Honest boundary: manual ordering only orders what the
//   user can actually drag, which is explicit projects.
export function orderProjectGroups(
  groups: ProjectGroup[],
  options: {
    overlays: Record<string, ProjectOverlay & { pinned?: boolean }>;
    sort: SidebarSort;
  },
): ProjectGroup[] {
  const recencyStandIn = (group: ProjectGroup): Session => {
    if (group.sessions.length > 0) return group.sessions[0];
    const created = group.createdAt ? new Date(group.createdAt).toISOString() : undefined;
    return { id: group.key, status: "", turns: 0, updatedAt: created } as Session;
  };
  const byRecency = (a: ProjectGroup, b: ProjectGroup) =>
    compareSessionsByUpdate(recencyStandIn(a), recencyStandIn(b));
  const pinnedRank = (group: ProjectGroup) => (options.overlays[group.key]?.pinned ? 0 : 1);
  const manualRank = (group: ProjectGroup) =>
    group.projectId && group.order && group.order > 0 ? group.order : Number.MAX_SAFE_INTEGER;
  const sorted = [...groups];
  switch (options.sort) {
    case "updated":
      sorted.sort(byRecency);
      break;
    case "manual":
      sorted.sort((a, b) => manualRank(a) - manualRank(b) || byRecency(a, b));
      break;
    default:
      sorted.sort((a, b) => pinnedRank(a) - pinnedRank(b) || byRecency(a, b));
  }
  return sorted;
}

// projectNameForWorkspace answers "what does the user call the place this
// session lives?" for every surface that names a workspace — sidebar hints,
// the command palette, Scheduled chips, the Home headline. Explicit project
// name (through its overlay rename) wins, then the derived group's overlay
// rename, then the derived label. "" for no workspace, as ever (SB-13).
export function projectNameForWorkspace(
  workspace: string | undefined,
  defs?: ProjectDef[],
  overlays?: Record<string, ProjectOverlay>,
): string {
  const clean = (workspace || "").trim().replace(/\/+$/, "");
  if (!clean) return "";
  for (const def of defs ?? []) {
    for (const folder of def.folders) {
      if (folder.trim().replace(/\/+$/, "") === clean) {
        const custom = (overlays?.[projectGroupKey(def.id)]?.displayName || "").trim();
        return custom || def.name;
      }
    }
  }
  const custom = (overlays?.[clean]?.displayName || "").trim();
  return custom || projectLabel(clean);
}

export function buildArchivedModel(
  sessions: Session[],
  archived: string[],
  query: string,
  titleOf: (session: Session) => string,
  projectDefs?: ProjectDef[],
): SidebarModel {
  const archivedIds = new Set(archived);
  const model = buildSidebarModel(
    sessions.filter((session) => archivedIds.has(session.id)),
    { pinned: [], archived: [], showArchived: true, query, titleOf, projectDefs },
  );
  // The archived browser only shows groups that hold archived sessions — an
  // empty explicit project has nothing to browse here.
  model.projects = model.projects.filter((group) => group.sessions.length > 0);
  // Settings → Archived is a purely *grouped* browser (it has no flat section),
  // so workspace-less sessions from the sidebar's flat `Sessions` section would
  // silently vanished from it. Fold them back into one trailing bucket here —
  // the SB-13 fix is about what the rail asserts, not about hiding archived
  // disappear from the one screen that exists to find them.
  if (model.workspaceLessSessions.length === 0) return model;
  return {
    ...model,
    projects: [...model.projects, { key: "__other__", label: "Other sessions", sessions: model.workspaceLessSessions }],
    workspaceLessSessions: [],
  };
}

export function daemonVersionLabel(version?: string): string {
  const token = (version || "").replace(/^agentrunner\s*/, "").split(" ")[0].trim();
  return !token || token.toLowerCase() === "unknown" ? "local" : token;
}
