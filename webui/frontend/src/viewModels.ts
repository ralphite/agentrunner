import type { Session } from "./types";
import { friendlyStatus, sessionFriendlyStatus } from "./components/pill";
import { sessionDate } from "./time";

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
// a project name: keep the stable root in the rail and use the latest hop as a
// short disambiguator. Restrict this to AgentRunner's managed worktree root so
// a real repository with a timestamped name is never rewritten.
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
  const lineage = managedWorktreeLineage(ws);
  if (lineage) return lineage.detail;
  if (siblings.some((sibling) => managedWorktreeLineage(clean(sibling))?.label === base)) return "Root";
  const scratch = scratchLabel(base);
  if (scratch) {
    // INC-78 后 scratch 组的 label 自带分钟级时间;孪生消歧 hint 若再给
    // 同一个分钟就是三重复读且零区分度(QA-0719 审查 #8:两个
    // "Scratch · 07-13 21:23" 并排)。带秒位分开同分钟孪生;fork 目录
    // 继承原时间戳(ws-…-212334 vs ws-…-212334-fork-61de 连秒都同,
    // QA-0719 审查 #17),fork 段是它唯一的身份,必须进 hint。
    let detail = scratch.replace(/^Scratch · /, "");
    const seconds = /^(?:ws|wt)-\d{8}-\d{4}(\d{2})/.exec(base);
    if (seconds) detail += `:${seconds[1]}`;
    const fork = /-fork-(\w+)/.exec(base);
    if (fork) detail += ` · fork ${fork[1]}`;
    return detail;
  }
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
    const identity = projectIdentity(clean);
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

  return { pinned, projects: [...groups.values()], workspaceLessSessions };
}

export function buildArchivedModel(
  sessions: Session[],
  archived: string[],
  query: string,
  titleOf: (session: Session) => string,
): SidebarModel {
  const archivedIds = new Set(archived);
  const model = buildSidebarModel(
    sessions.filter((session) => archivedIds.has(session.id)),
    { pinned: [], archived: [], showArchived: true, query, titleOf },
  );
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
