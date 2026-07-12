// Nav / command-palette view models (INC-41 RH-3). Kept beside viewModels.ts
// so the palette's grouping is a pure, testable function rather than JSX-local
// arithmetic.
import type { Session } from "./types";
import { quickSwitchTasks, sessionNeedsAttention } from "./viewModels";

export interface PaletteTaskGroups {
  // The Tasks group: exactly the list the global ⌘1..9 binding indexes
  // (App.tsx uses quickSwitchTasks(...)[digit - 1]). Position i therefore *is*
  // the ⌘(i+1) badge — the badge cannot drift from the binding.
  quick: Session[];
  // The Unread tasks group: attention-worthy tasks that fell past the nine
  // digits. No badges — there is no key to press for them (Codex parity:
  // `codex-crop-command-palette.jpg` shows exactly this two-group split).
  unread: Session[];
}

// UNREAD_OVERFLOW_CAP keeps the palette a keyboard list, not a task browser:
// past this, the sidebar (and a typed query) is the right surface.
const UNREAD_OVERFLOW_CAP = 9;

// paletteTaskGroups splits the no-query palette into Codex's two task groups.
//
// RH-3: the old code badged only the *non*-attention rows, so on any machine
// where the nine quick-switch slots were all attention tasks the palette showed
// zero badges and no Tasks group at all — while ⌘1..9 still worked. Badges now
// ride every row of the quick-switch list, unread dot or not, so what the
// palette shows and what the keyboard does are the same thing.
export function paletteTaskGroups(
  sessions: Session[],
  opts: { archived?: string[] } = {},
): PaletteTaskGroups {
  const quick = quickSwitchTasks(sessions, opts);
  const inQuick = new Set(quick.map((s) => s.id));
  const archived = new Set(opts.archived || []);
  const unread = sessions
    .filter(
      (s) =>
        s.kind !== "driver" &&
        !archived.has(s.id) &&
        !inQuick.has(s.id) &&
        sessionNeedsAttention(s.status),
    )
    .sort((a, b) => b.id.localeCompare(a.id)) // newest first (ids are creation stamps)
    .slice(0, UNREAD_OVERFLOW_CAP);
  return { quick, unread };
}
