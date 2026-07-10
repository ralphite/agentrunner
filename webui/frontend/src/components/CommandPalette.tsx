import { useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "../store";
import { nextTheme } from "../theme";
import { displayTitle } from "../title";
import { projectLabel, quickSwitchTasks, sessionNeedsAttention } from "../viewModels";
import { modLabel } from "../shortcuts";

interface Item {
  id: string;
  label: string;
  hint?: string;
  group: string;
  quickNum?: number; // ⌘1..9 quick-switch badge on recent-task rows
  run: () => void;
}

// CommandPalette is Codex's ⌘K: one fuzzy search over sessions + a set of
// commands, keyboard-navigable (↑/↓, Enter, Esc). Opened from a global
// key handler in App.
export function CommandPalette({ onClose }: { onClose: () => void }) {
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
      onClose();
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
    // With no query this is the ⌘1..9 quick-switch list: recent tasks, the
    // attention-worthy ones grouped on top with the lowest numbers (matching
    // the global cmd-digit binding). Typing a query switches to a plain fuzzy
    // search over every task, without badges.
    let sess: Item[];
    if (!ql) {
      sess = quickSwitchTasks(sessions, { archived }).map((s, i) => ({
        id: "s" + s.id,
        label: displayTitle(renames, s.id, s.title),
        hint: projectLabel(s.workspace),
        group: sessionNeedsAttention(s.status) ? "Needs attention" : "Tasks",
        quickNum: i + 1,
        run: go(() => select(s.id)),
      }));
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
    return [...cmds, ...sess, ...rn];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q, sessions, runs, archived, theme, renames]);

  useEffect(() => setIdx(0), [q]);

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") onClose();
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
    <div className="backdrop cmdk-back" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="cmdk" onKeyDown={onKey} role="dialog" aria-modal="true" aria-label="Command palette">
        <input
          ref={inputRef}
          className="cmdk-input"
          placeholder="Search sessions or run a command…"
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
