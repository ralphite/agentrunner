import { useEffect, useMemo, useRef, useState } from "react";
import { SHORTCUT_GROUPS, keyLabel } from "../shortcuts";

// Shortcuts is Codex's Keyboard-shortcuts reference: a searchable, grouped list
// of every binding the app has, rendered as key badges. Opened globally with
// "?" or from the command palette.
export function Shortcuts({ onClose }: { onClose: () => void }) {
  const [q, setQ] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const groups = useMemo(() => {
    const ql = q.trim().toLowerCase();
    if (!ql) return SHORTCUT_GROUPS;
    return SHORTCUT_GROUPS.map((g) => ({
      ...g,
      items: g.items.filter(
        (it) =>
          it.label.toLowerCase().includes(ql) ||
          (it.desc || "").toLowerCase().includes(ql) ||
          it.keys.map(keyLabel).join(" ").toLowerCase().includes(ql),
      ),
    })).filter((g) => g.items.length);
  }, [q]);

  return (
    <div className="backdrop cmdk-back" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="cmdk shortcuts" onKeyDown={(e) => e.key === "Escape" && onClose()}>
        <div className="sc-head">
          <div className="sc-title">Keyboard shortcuts</div>
          <input
            ref={inputRef}
            className="cmdk-input sc-search"
            placeholder="Search shortcuts…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <div className="cmdk-list sc-list">
          {groups.length === 0 && <div className="cmdk-empty">No matching shortcuts</div>}
          {groups.map((g) => (
            <div key={g.title} className="sc-group">
              <div className="cmdk-group">{g.title}</div>
              {g.items.map((it, i) => (
                <div key={i} className="sc-row">
                  <div className="sc-label">
                    <span>{it.label}</span>
                    {it.desc && <span className="sc-desc">{it.desc}</span>}
                  </div>
                  <div className="sc-keys">
                    {it.keys.map((k, j) => (
                      <kbd key={j} className="kbd">
                        {keyLabel(k)}
                      </kbd>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
