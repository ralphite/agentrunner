import { useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "../store";
import { nextTheme } from "../theme";
import { displayTitle } from "../title";
import { projectLabel } from "../viewModels";
import { paletteTaskGroups } from "../viewModels.nav";
import { friendlyStatus } from "./pill";
import { modLabel } from "../shortcuts";
import type { Session } from "../types";

interface Item {
  id: string;
  label: string;
  hint?: string;
  group: string;
  quickNum?: number; // ⌘1..9 quick-switch badge on recent-task rows
  task?: boolean; // task rows reserve a leading status-dot gutter (Codex parity)
  dot?: string; // status-dot class: `unread`, or friendlyStatus().cls — never both
  dotTitle?: string; // what the dot means, spelled out on hover
  run: () => void;
}

// A task row shows a dot on exactly the terms the sidebar uses (Sidebar.tsx):
// unread beats status, and only the statuses that actually want the user get a
// colour. Anything else keeps an empty (but reserved) gutter so labels align.
// CP-6: the palette used to paint *every* task dot blue `unread`, so a task
// stuck on an approval or a crash was advertised in ⌘K as "new activity" while
// the rail next to it showed amber/red. Same source, same colour now.
const DOTTED = ["run", "appr", "stranded", "crash"];
function taskDot(session: Session, isUnread: boolean): { dot?: string; dotTitle?: string } {
  const status = friendlyStatus(session.status);
  if (isUnread) return { dot: "unread", dotTitle: "New activity" };
  if (DOTTED.includes(status.cls)) return { dot: status.cls, dotTitle: status.text };
  return {};
}

// CommandPalette is Codex's ⌘K: one fuzzy search over sessions + a set of
// commands, keyboard-navigable (↑/↓, Enter, Esc). Opened from a global
// key handler in App.
export function CommandPalette({ onClose, onOpenSettings }: {
  onClose: (restoreFocus?: boolean) => void;
  onOpenSettings?: () => void;
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

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const items = useMemo<Item[]>(() => {
    const ql = q.trim().toLowerCase();
    const match = (s: string) => !ql || s.toLowerCase().includes(ql);
    const go = (fn: () => void) => () => {
      // A selected command owns the next focus target (for example, a modal or
      // the destination task). Only dismissals restore the pre-palette focus.
      onClose(false);
      fn();
    };
    const cmds: Item[] = [
      { id: "c-new", label: "New task", group: "Commands", run: go(() => showPage("home")) },
      { id: "c-run", label: "New run…", hint: "submit / drive", group: "Commands", run: go(() => openModal({ kind: "run" })) },
      // CP-8: Scheduled is the app's other top-level destination and Settings is
      // a whole page (⌘,) — ⌘K could reach neither, so the palette was a
      // task-switcher pretending to be a command palette.
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
    // every one of the nine rows lives in the `Tasks` group and carries its
    // ⌘-digit badge — a blue unread dot does not take the badge away, it just
    // adds a dot. The badge number is the row's index in the very list App.tsx's
    // cmd-digit handler indexes, so ⌘3 always opens the row labelled ⌘3.
    // Attention tasks that fell past the ninth digit get their own badge-less
    // `Unread tasks` group. Typing a query switches to a plain fuzzy search over
    // every task, without badges.
    const unreadSet = new Set(unread);
    let sess: Item[];
    if (!ql) {
      const { quick, unread: overflow } = paletteTaskGroups(sessions, { archived });
      const row = (s: Session, quickNum?: number): Item => ({
        id: "s" + s.id,
        label: displayTitle(renames, s.id, s.title),
        hint: projectLabel(s.workspace),
        group: quickNum ? "Tasks" : "Unread tasks",
        quickNum,
        task: true,
        ...taskDot(s, unreadSet.has(s.id)),
        run: go(() => select(s.id)),
      });
      sess = [...quick.map((s, i) => row(s, i + 1)), ...overflow.map((s) => row(s))];
    } else {
      // CP-7: search runs over every session, archived ones included — they used
      // to come back indistinguishable from live tasks, silently un-archived in
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
            group: isArchived ? "Archived" : "Tasks",
            task: true,
            ...taskDot(s, !isArchived && unreadSet.has(s.id)),
            run: go(() => select(s.id)),
          } satisfies Item;
        });
      // Group headers are drawn off runs of equal `group`, so archived hits sit
      // together at the bottom rather than interleaving with live tasks.
      sess = [...hits.filter((h) => h.group === "Tasks"), ...hits.filter((h) => h.group === "Archived")];
    }
    const rn: Item[] = runs
      .filter((r) => match(r.label || r.id))
      .slice(0, 4)
      .map((r) => ({
        id: "r" + r.id,
        label: r.label || r.id,
        hint: r.kind,
        group: "Scheduled",
        run: go(() => selectRun(r.id)),
      }));
    // Empty query is the task-switcher: surface tasks before commands (Codex
    // parity). While typing, commands stay on top so quick actions win the
    // first Enter.
    return ql ? [...cmds, ...sess, ...rn] : [...sess, ...cmds, ...rn];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q, sessions, runs, archived, unread, theme, renames, onOpenSettings]);

  useEffect(() => setIdx(0), [q]);

  // CP-5: ↓ used to walk the selection straight out of the scroll box — 14 of
  // 24 rows were keyboard-reachable but invisible, so Enter opened a task the
  // user could not see. Keep the selected row in view on every idx change.
  useEffect(() => {
    selRef.current?.scrollIntoView?.({ block: "nearest" });
  }, [idx]);

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") onClose(true);
    else if (e.key === "ArrowDown") {
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
    <div className="backdrop cmdk-back items-start pt-[12vh]" onMouseDown={(e) => e.target === e.currentTarget && onClose(true)}>
      <div
        className="cmdk w-[min(560px,92vw)] overflow-hidden rounded-[14px] border border-line bg-panel shadow-[0_16px_48px_rgba(0,0,0,0.28)]"
        onKeyDown={onKey}
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
      >
        <input
          ref={inputRef}
          className="cmdk-input w-full border-0 border-b border-line rounded-none bg-transparent px-4 py-[14px] text-[16px] focus:outline-none"
          placeholder="Search tasks or run a command"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          role="combobox"
          aria-controls="command-palette-results"
          aria-expanded="true"
          aria-activedescendant={items[idx]?.id}
        />
        <div
          className="cmdk-list max-h-[min(64vh,620px)] overflow-y-auto p-[6px] scroll-p-[6px]"
          id="command-palette-results"
          role="listbox"
          onMouseMove={() => {
            kbdNav.current = false;
          }}
        >
          {items.length === 0 && <div className="cmdk-empty pt-3 px-[14px] pb-[13px] text-center text-[12.5px] text-dim">No matches</div>}
          {items.map((it, i) => {
            const showGroup = i === 0 || items[i - 1].group !== it.group;
            return (
              <div key={it.id}>
                {showGroup && <div className="cmdk-group pt-[6px] px-[10px] pb-[3px] text-[13.5px] text-dim">{it.group}</div>}
                <button
                  type="button"
                  id={it.id}
                  ref={i === idx ? selRef : undefined}
                  className={
                    "cmdk-item flex w-full items-center gap-[10px] rounded-lg border-0 px-3 py-[7px] text-left text-ink hover:bg-panel-2 " +
                    (i === idx ? "sel bg-panel-2" : "bg-transparent")
                  }
                  role="option"
                  aria-selected={i === idx}
                  onMouseEnter={() => {
                    if (!kbdNav.current) setIdx(i);
                  }}
                  onClick={() => it.run()}
                >
                  {it.task && (
                    // The leading dot says the same thing here as in the rail:
                    // blue for new activity, otherwise the status' own colour.
                    // Dot-less task rows keep an equal-width gutter so labels
                    // stay aligned.
                    <span
                      className={"status-dot" + (it.dot ? " " + it.dot : "")}
                      style={it.dot ? undefined : { visibility: "hidden" }}
                      title={it.dotTitle}
                      aria-hidden="true"
                    />
                  )}
                  <span className="cmdk-label flex-1 truncate text-[15px] leading-[1.4]">{it.label}</span>
                  {it.hint && <span className="cmdk-hint shrink-0 text-[13px] text-dim">{it.hint}</span>}
                  {it.quickNum && it.quickNum <= 9 && (
                    <span
                      className={
                        "cmdk-kbd shrink-0 ml-[6px] min-w-[26px] px-[6px] py-px border rounded-md bg-panel-2 text-center font-mono text-[11px] leading-[1.5] " +
                        (i === idx ? "border-[color-mix(in_srgb,var(--blue)_38%,var(--line))] text-ink-2" : "border-line text-dim")
                      }
                      aria-hidden="true"
                    >{modLabel}{it.quickNum}</span>
                  )}
                </button>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
