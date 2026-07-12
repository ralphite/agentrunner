import { useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "../store";
import { nextTheme } from "../theme";
import { displayTitle } from "../title";
import { projectLabel, sessionNeedsAttention } from "../viewModels";
import { paletteTaskGroups } from "../viewModels.nav";
import { modLabel } from "../shortcuts";

interface Item {
  id: string;
  label: string;
  hint?: string;
  group: string;
  quickNum?: number; // ⌘1..9 quick-switch badge on recent-task rows
  task?: boolean; // task rows reserve a leading status-dot gutter (Codex parity)
  attention?: boolean; // needs-looking-at rows show a blue leading dot
  run: () => void;
}

// CommandPalette is Codex's ⌘K: one fuzzy search over sessions + a set of
// commands, keyboard-navigable (↑/↓, Enter, Esc). Opened from a global
// key handler in App.
export function CommandPalette({ onClose }: { onClose: (restoreFocus?: boolean) => void }) {
  const { sessions, runs, archived, select, selectRun, showPage, openModal, toggleShowArchived, theme, cycleTheme, openHelp, renames } =
    useStore();
  const [q, setQ] = useState("");
  const [idx, setIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

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
    let sess: Item[];
    if (!ql) {
      const { quick, unread } = paletteTaskGroups(sessions, { archived });
      const row = (s: (typeof sessions)[number], quickNum?: number): Item => ({
        id: "s" + s.id,
        label: displayTitle(renames, s.id, s.title),
        hint: projectLabel(s.workspace),
        group: quickNum ? "Tasks" : "Unread tasks",
        quickNum,
        task: true,
        attention: sessionNeedsAttention(s.status),
        run: go(() => select(s.id)),
      });
      sess = [...quick.map((s, i) => row(s, i + 1)), ...unread.map((s) => row(s))];
    } else {
      sess = [...sessions]
        .sort((a, b) => b.id.localeCompare(a.id)) // newest first, same as the sidebar
        .filter((s) => match(displayTitle(renames, s.id, s.title)) || match(s.id) || match(s.workspace || ""))
        .slice(0, 8)
        .map((s) => ({
          id: "s" + s.id,
          label: displayTitle(renames, s.id, s.title),
          hint: projectLabel(s.workspace),
          group: "Tasks",
          task: true,
          attention: sessionNeedsAttention(s.status),
          run: go(() => select(s.id)),
        }));
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
  }, [q, sessions, runs, archived, theme, renames]);

  useEffect(() => setIdx(0), [q]);

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") onClose(true);
    else if (e.key === "ArrowDown") {
      e.preventDefault();
      setIdx((i) => Math.min(i + 1, items.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setIdx((i) => Math.max(i - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      items[idx]?.run();
    }
  };

  return (
    <div className="backdrop cmdk-back" onMouseDown={(e) => e.target === e.currentTarget && onClose(true)}>
      <div className="cmdk" onKeyDown={onKey} role="dialog" aria-modal="true" aria-label="Command palette">
        <input
          ref={inputRef}
          className="cmdk-input"
          placeholder="Search tasks or run a command"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          role="combobox"
          aria-controls="command-palette-results"
          aria-expanded="true"
          aria-activedescendant={items[idx]?.id}
        />
        <div className="cmdk-list" id="command-palette-results" role="listbox">
          {items.length === 0 && <div className="cmdk-empty">No matches</div>}
          {items.map((it, i) => {
            const showGroup = i === 0 || items[i - 1].group !== it.group;
            return (
              <div key={it.id}>
                {showGroup && <div className="cmdk-group">{it.group}</div>}
                <button
                  type="button"
                  id={it.id}
                  className={"cmdk-item" + (i === idx ? " sel" : "")}
                  role="option"
                  aria-selected={i === idx}
                  onMouseEnter={() => setIdx(i)}
                  onClick={() => it.run()}
                >
                  {it.task && (
                    // Blue leading dot flags tasks that need looking at; other
                    // task rows keep an equal-width gutter so labels stay aligned.
                    <span
                      className={"status-dot" + (it.attention ? " unread" : "")}
                      style={it.attention ? undefined : { visibility: "hidden" }}
                      aria-hidden="true"
                    />
                  )}
                  <span className="cmdk-label">{it.label}</span>
                  {it.hint && <span className="cmdk-hint">{it.hint}</span>}
                  {it.quickNum && it.quickNum <= 9 && (
                    <span className="cmdk-kbd" aria-hidden="true">{modLabel}{it.quickNum}</span>
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
