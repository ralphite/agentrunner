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
    <div className="backdrop cmdk-back items-start pt-[12vh]" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div
        className="cmdk shortcuts w-[min(620px,94vw)] bg-panel border border-line rounded-[14px] shadow-[0_16px_48px_rgba(0,0,0,0.28)] overflow-hidden"
        onKeyDown={(e) => e.key === "Escape" && onClose()}
      >
        <div className="sc-head border-b border-line">
          <div className="sc-title pt-[12px] px-[16px] text-[13px] font-semibold text-ink">Keyboard shortcuts</div>
          <input
            ref={inputRef}
            className="cmdk-input sc-search w-full border-0 rounded-none bg-transparent pt-[8px] px-[16px] pb-[12px] text-[14px] focus:outline-none"
            placeholder="Search shortcuts…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <div className="cmdk-list sc-list max-h-[min(64vh,620px)] overflow-y-auto pt-[6px] px-[6px] pb-[10px]">
          {groups.length === 0 && <div className="cmdk-empty p-[18px] text-center text-dim text-[13px]">No matching shortcuts</div>}
          {groups.map((g) => (
            <div key={g.title} className="sc-group mb-[4px]">
              <div className="cmdk-group pt-[6px] px-[10px] pb-[3px] text-[13.5px] text-dim">{g.title}</div>
              {g.items.map((it, i) => (
                <div key={i} className="sc-row flex items-center gap-[12px] px-[10px] py-[7px] rounded-[8px] hover:bg-panel-2">
                  <div className="sc-label flex-1 min-w-0 flex flex-col gap-px">
                    <span className="text-[13.5px] text-ink">{it.label}</span>
                    {it.desc && <span className="sc-desc text-[11.5px] text-dim">{it.desc}</span>}
                  </div>
                  <div className="sc-keys flex items-center gap-[4px] shrink-0">
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
