// Nav / command-palette view models (INC-41 RH-3). Kept beside viewModels.ts
// so the palette's grouping is a pure, testable function rather than JSX-local
// arithmetic.
import type { Session } from "./types";
import { type ProjectGroup, quickSwitchSessions, sessionNeedsAttention } from "./viewModels";

export interface PaletteSessionGroups {
  // The Sessions group is exactly the list the global ⌘1..9 binding indexes.
  // (App.tsx uses quickSwitchSessions(...)[digit - 1]). Position i therefore *is*
  // the ⌘(i+1) badge — the badge cannot drift from the binding.
  quick: Session[];
  // Attention-worthy sessions past the nine digits have no shortcut badge.
  attention: Session[];
}

// ATTENTION_OVERFLOW_CAP keeps the palette a keyboard list, not a session browser:
// past this, the sidebar (and a typed query) is the right surface.
const ATTENTION_OVERFLOW_CAP = 9;

// paletteSessionGroups splits the no-query palette into quick and attention groups.
//
// RH-3: the old code badged only the *non*-attention rows, so on any machine
// where the nine quick-switch slots were all attention sessions the palette
// showed zero badges and no Sessions group while ⌘1..9 still worked. Badges now
// ride every row of the quick-switch list, unread dot or not, so what the
// palette shows and what the keyboard does are the same thing.
export function paletteSessionGroups(
  sessions: Session[],
  opts: { archived?: string[] } = {},
): PaletteSessionGroups {
  const quick = quickSwitchSessions(sessions, opts);
  const inQuick = new Set(quick.map((s) => s.id));
  const archived = new Set(opts.archived || []);
  const attention = sessions
    .filter(
      (s) =>
        s.kind !== "driver" &&
        !archived.has(s.id) &&
        !inQuick.has(s.id) &&
        sessionNeedsAttention(s.status),
    )
    .sort((a, b) => b.id.localeCompare(a.id)) // newest first (ids are creation stamps)
    .slice(0, ATTENTION_OVERFLOW_CAP);
  return { quick, attention };
}

// PROJECT_GROUP_LIMIT — how many project groups the Projects section renders
// before the section-level "Show more" (SB-4). Codex's sidebar is a *navigator*:
// a short, one-screen list you scan. Ours rendered every group it had (127 on
// the author's machine → 14073px of rail), so the account footer sat a dozen
// screens below the fold and the section stopped being navigable at all.
// Groups sort newest-first (buildSidebarModel), so the first 8 are the projects
// you actually touched recently — the rest are history, one click away.
export const PROJECT_GROUP_LIMIT = 8;

export interface VisibleProjectGroups {
  groups: ProjectGroup[];
  // Groups the section is withholding. 0 ⇒ no "Show more" row to render.
  hidden: number;
}

// visibleProjectGroups truncates the Projects section to `limit` groups.
//
// SB-4 invariant (the same one visibleProjectSessions holds one level down):
// the group holding the session you have open is *always* rendered, even when
// it sorts past the limit. Truncation is a default view, not a claim that the
// current session should vanish — without this, deep-linking or ⌘K-jumping into
// the 40th project would leave the rail with no highlighted row and no trace of
// where you are, which is strictly worse than the long list we're fixing.
// The current group is appended at the tail so the first `limit` rows never
// shuffle under the user.
export function visibleProjectGroups(
  projects: ProjectGroup[],
  opts: { expanded?: boolean; limit?: number; current?: string } = {},
): VisibleProjectGroups {
  const limit = opts.limit ?? PROJECT_GROUP_LIMIT;
  if (opts.expanded || projects.length <= limit) return { groups: projects, hidden: 0 };
  const groups = projects.slice(0, limit);
  const current = opts.current
    ? projects.find((project) => project.sessions.some((session) => session.id === opts.current))
    : undefined;
  if (current && !groups.includes(current)) groups.push(current);
  return { groups, hidden: projects.length - groups.length };
}
