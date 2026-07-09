import { useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "../store";
import { nextTheme } from "../theme";
import { displayTitle } from "../title";

interface Item {
  id: string;
  label: string;
  hint?: string;
  group: string;
  run: () => void;
}

// CommandPalette is Codex's ⌘K: one fuzzy search over sessions + a set of
// commands, keyboard-navigable (↑/↓, Enter, Esc). Opened from a global
// key handler in App.
export function CommandPalette({ onClose }: { onClose: () => void }) {
  const { sessions, runs, select, selectRun, openModal, toggleShowArchived, theme, cycleTheme, openHelp, renames } =
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
      { id: "c-new", label: "New task", group: "Commands", run: go(() => openModal({ kind: "new" })) },
      { id: "c-run", label: "Background run…", group: "Commands", run: go(() => openModal({ kind: "run" })) },
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
    const sess: Item[] = sessions
      .filter((s) => match(displayTitle(renames, s.id, s.title)) || match(s.id))
      .slice(0, 8)
      .map((s) => ({
        id: "s" + s.id,
        label: displayTitle(renames, s.id, s.title),
        hint: s.status,
        group: "Sessions",
        run: go(() => select(s.id)),
      }));
    const rn: Item[] = runs
      .filter((r) => match(r.label || r.id))
      .slice(0, 4)
      .map((r) => ({
        id: "r" + r.id,
        label: r.label || r.id,
        hint: r.kind,
        group: "Background runs",
        run: go(() => selectRun(r.id)),
      }));
    return [...cmds, ...sess, ...rn];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q, sessions, runs, theme, renames]);

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
      <div className="cmdk" onKeyDown={onKey}>
        <input
          ref={inputRef}
          className="cmdk-input"
          placeholder="Search sessions or run a command…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <div className="cmdk-list">
          {items.length === 0 && <div className="cmdk-empty">No matches</div>}
          {items.map((it, i) => {
            const showGroup = i === 0 || items[i - 1].group !== it.group;
            return (
              <div key={it.id}>
                {showGroup && <div className="cmdk-group">{it.group}</div>}
                <div
                  className={"cmdk-item" + (i === idx ? " sel" : "")}
                  onMouseEnter={() => setIdx(i)}
                  onClick={() => it.run()}
                >
                  <span className="cmdk-label">{it.label}</span>
                  {it.hint && <span className="cmdk-hint">{it.hint}</span>}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
