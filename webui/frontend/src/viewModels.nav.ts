// Nav / command-palette view models (INC-41 RH-3). Kept beside viewModels.ts
// so the palette's grouping is a pure, testable function rather than JSX-local
// arithmetic.
import type { Session } from "./types";
import { quickSwitchSessions, sessionNeedsAttention } from "./viewModels";

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
        sessionNeedsAttention(s),
    )
    .sort((a, b) => b.id.localeCompare(a.id)) // newest first (ids are creation stamps)
    .slice(0, ATTENTION_OVERFLOW_CAP);
  return { quick, attention };
}

// The Projects section renders every group, always, in the order the model
// gives (activity mtime, buildSidebarModel). There is deliberately no section
// truncation and no current-project promotion: the rail's order may only move
// on a real mutation to a project (a new turn, a pin), never because the user
// merely *selected* a session. An earlier SB-4 design capped the section at 4
// groups behind "Show more projects" and floated the current group to the top;
// both made selection reorder the rail and were removed (user adjudication
// 2026-07-28).
