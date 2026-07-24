import { X } from "@phosphor-icons/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "../store";
import { nextTheme } from "../theme";
import { displayTitle } from "../title";
import { projectLabel } from "../viewModels";
import { paletteSessionGroups } from "../viewModels.nav";
import { sessionFriendlyStatus } from "./pill";
import { modLabel } from "../shortcuts";
import type { Session } from "../types";
import { FocusScope } from "../ui/FocusScope";
import {
  CommandPaletteItem,
  type CommandPaletteItemModel,
} from "./CommandPaletteItem";

// A session row shows a dot on exactly the terms the sidebar uses:
// unread beats status, and only the statuses that actually want the user get a
// colour. Anything else keeps an empty (but reserved) gutter so labels align.
// CP-6: the palette used to paint every session dot blue `unread`, so a session
// stuck on an approval or a crash was advertised in ⌘K as "new activity" while
// the rail next to it showed amber/red. Same source, same colour now.
const DOTTED = ["run", "appr", "stranded", "crash"];
function sessionDot(session: Session, isUnread: boolean): { dot?: string; dotTitle?: string; actionCount?: number } {
  const status = sessionFriendlyStatus(session);
  const actionCount =
    (session.attention?.approvals || 0) +
    (session.attention?.answers || 0);
  if (actionCount > 1 && status.cls === "appr") {
    return { dot: "appr", dotTitle: status.text, actionCount };
  }
  if (isUnread && status.cls !== "appr") return { dot: "unread", dotTitle: "New activity" };
  if (DOTTED.includes(status.cls)) return { dot: status.cls, dotTitle: status.text };
  return {};
}

// CommandPalette is Codex's ⌘K: one fuzzy search over sessions + a set of
// commands, keyboard-navigable (↑/↓, Enter, Esc). Opened from a global
// key handler in App.
export function CommandPalette({ onClose, onOpenSettings, shouldRestoreFocus }: {
  onClose: (restoreFocus?: boolean) => void;
  onOpenSettings?: () => void;
  shouldRestoreFocus?: () => boolean;
}) {
  const { sessions, runs, archived, unread, select, selectRun, showPage, openModal, toggleShowArchived, theme, cycleTheme, openHelp, renames } =
    useStore();
  const [q, setQ] = useState("");
  const [idx, setIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const selRef = useRef<HTMLButtonElement>(null);
  // CP-5: while the keyboard is driving, a row scrolling *under* a parked
  // pointer fires mouseenter and used to yank the selection back to wherever
  // the mouse happens to sit. Hover only claims the selection after a real
  // pointer move.
  const kbdNav = useRef(false);

  const items = useMemo<CommandPaletteItemModel[]>(() => {
    const ql = q.trim().toLowerCase();
    const match = (s: string) => !ql || s.toLowerCase().includes(ql);
    const go = (fn: () => void) => () => {
      // A selected command owns the next focus target (for example, a modal or
      // the destination session). Only dismissals restore pre-palette focus.
      onClose(false);
      fn();
    };
    const cmds: CommandPaletteItemModel[] = [
      { id: "c-new", label: "New session", group: "Commands", run: go(() => showPage("home")) },
      { id: "c-run", label: "New run…", hint: "one-shot / scheduled", group: "Commands", run: go(() => openModal({ kind: "run" })) },
      // CP-8: Scheduled is the app's other top-level destination and Settings is
      // a whole page (⌘,) — ⌘K could reach neither, so the palette was a
      // session switcher pretending to be a command palette.
      { id: "c-sched", label: "Go to Scheduled", group: "Commands", run: go(() => showPage("scheduled")) },
      ...(onOpenSettings
        ? [{ id: "c-settings", label: "Open settings", hint: `${modLabel},`, group: "Commands", run: go(onOpenSettings) }]
        : []),
      { id: "c-trust", label: "Trust directory…", group: "Commands", run: go(() => openModal({ kind: "trust" })) },
      { id: "c-arch", label: "Toggle archived", group: "Commands", run: go(() => toggleShowArchived()) },
      {
        id: "c-theme",
        label: `Switch theme to ${nextTheme(theme)}`,
        hint: theme,
        group: "Commands",
        run: go(() => cycleTheme()),
      },
      { id: "c-keys", label: "Keyboard shortcuts", hint: "?", group: "Commands", run: go(() => openHelp()) },
    ].filter((c) => match(c.label));
    // With no query this is the ⌘1..9 quick-switch list (Codex parity, RH-3):
    // every one of the nine rows lives in the `Sessions` group and carries its
    // ⌘-digit badge — a blue unread dot does not take the badge away, it just
    // adds a dot. The badge number is the row's index in the very list App.tsx's
    // cmd-digit handler indexes, so ⌘3 always opens the row labelled ⌘3.
    // Attention sessions past the ninth digit get a badge-less group. Typing a
    // query switches to a plain fuzzy search over every session.
    const unreadSet = new Set(unread);
    let sess: CommandPaletteItemModel[];
    if (!ql) {
      const { quick, attention: overflow } = paletteSessionGroups(sessions, { archived });
      const row = (s: Session, quickNum?: number): CommandPaletteItemModel => ({
        id: "s" + s.id,
        label: displayTitle(renames, s.id, s.title),
        hint: projectLabel(s.workspace),
        group: quickNum ? "Sessions" : "Needs attention",
        quickNum,
        session: true,
        ...sessionDot(s, unreadSet.has(s.id)),
        run: go(() => select(s.id)),
      });
      sess = [...quick.map((s, i) => row(s, i + 1)), ...overflow.map((s) => row(s))];
    } else {
      // CP-7: search runs over every session, archived ones included — they used
      // to come back indistinguishable from live sessions, silently un-archived in
      // the one place the user is most likely to hit Enter. They stay reachable
      // (that is the point of search) but land in their own honest group.
      const archivedSet = new Set(archived);
      const hits = [...sessions]
        .sort((a, b) => b.id.localeCompare(a.id)) // newest first, same as the sidebar
        .filter((s) => match(displayTitle(renames, s.id, s.title)) || match(s.id) || match(s.workspace || ""))
        .slice(0, 8)
        .map((s) => {
          const isArchived = archivedSet.has(s.id);
          return {
            id: "s" + s.id,
            label: displayTitle(renames, s.id, s.title),
            hint: projectLabel(s.workspace),
            group: isArchived ? "Archived" : "Sessions",
            session: true,
            ...sessionDot(s, !isArchived && unreadSet.has(s.id)),
            run: go(() => select(s.id)),
          } satisfies CommandPaletteItemModel;
        });
      // Group headers are drawn off runs of equal `group`, so archived hits sit
      // together at the bottom rather than interleaving with live sessions.
      sess = [...hits.filter((h) => h.group === "Sessions"), ...hits.filter((h) => h.group === "Archived")];
    }
    const rn: CommandPaletteItemModel[] = runs
      .filter((r) => match(r.label || r.id))
      .slice(0, 4)
      .map((r) => ({
        id: "r" + r.id,
        label: r.label || r.id,
        hint: r.kind,
        group: "Scheduled",
        run: go(() => selectRun(r.id)),
      }));
    // Empty query keeps the nine truthful ⌘-digit rows first, then exposes the
    // commands before attention overflow. With a large shared store the old
    // order put up to nine extra attention rows ahead of Commands, pushing the
    // very actions promised by "run a command" below the first scroll viewport.
    // Codex likewise shows its short chat switcher before Suggested/Settings;
    // overflow history belongs after those actions. While typing, commands stay
    // on top so a matching quick action wins the first Enter.
    return ql
      ? [...cmds, ...sess, ...rn]
      : [
          ...sess.filter((item) => item.group === "Sessions"),
          ...cmds,
          ...sess.filter((item) => item.group === "Needs attention"),
          ...rn,
        ];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q, sessions, runs, archived, unread, theme, renames, onOpenSettings]);

  useEffect(() => setIdx(0), [q]);

  // CP-5: ↓ used to walk the selection straight out of the scroll box — 14 of
  // 24 rows were keyboard-reachable but invisible, so Enter opened a session the
  // user could not see. Keep the selected row in view on every idx change.
  useEffect(() => {
    selRef.current?.scrollIntoView?.({ block: "nearest" });
  }, [idx]);

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      kbdNav.current = true;
      setIdx((i) => Math.min(i + 1, items.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      kbdNav.current = true;
      setIdx((i) => Math.max(i - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      items[idx]?.run();
    }
  };

  return (
    <div className="backdrop cmdk-back command-palette-back" onMouseDown={(e) => e.target === e.currentTarget && onClose(true)}>
      <FocusScope
        className="cmdk"
        initialFocus={inputRef}
        shouldRestoreFocus={shouldRestoreFocus}
        onEscape={() => onClose(true)}
        onKeyDown={onKey}
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
      >
        <div className="cmdk-search">
          <input
            ref={inputRef}
            className="cmdk-input"
            placeholder="Search sessions or run a command"
            aria-label="Search sessions or run a command"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            role="combobox"
            aria-controls="command-palette-results"
            aria-expanded="true"
            aria-activedescendant={items[idx]?.id}
          />
          <button type="button" className="cmdk-close" onClick={() => onClose(true)} title="Close" aria-label="Close command palette">
            <X size={18} />
          </button>
        </div>
        <div
          className="cmdk-list"
          id="command-palette-results"
          role="listbox"
          onMouseMove={() => {
            kbdNav.current = false;
          }}
        >
          {items.length === 0 && <div className="cmdk-empty">No matches</div>}
          {items.map((it, i) => {
            const showGroup = i === 0 || items[i - 1].group !== it.group;
            return (
              <div key={it.id}>
                {showGroup && <div className="cmdk-group">{it.group}</div>}
                <CommandPaletteItem
                  item={it}
                  selected={i === idx}
                  buttonRef={i === idx ? selRef : undefined}
                  onHover={() => {
                    if (!kbdNav.current) setIdx(i);
                  }}
                  onSelect={it.run}
                />
              </div>
            );
          })}
        </div>
      </FocusScope>
    </div>
  );
}
