import { useEffect, useLayoutEffect, useRef, useState } from "react";

// Popover is the drop-up menu primitive the composer controls hang off of. It
// anchors a panel to a trigger button, opens *upward* (the composer sits at the
// bottom of the screen), and closes on outside-click / Escape. Kept dependency-
// free and controlled-optional so each control can drive its own open state.
export function Popover({
  trigger,
  children,
  align = "left",
  panelClass = "",
  onOpen,
}: {
  trigger: (open: boolean, toggle: () => void) => React.ReactNode;
  children: (close: () => void) => React.ReactNode;
  align?: "left" | "right";
  panelClass?: string;
  onOpen?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [drop, setDrop] = useState<"up" | "down">("up");
  const [maxH, setMaxH] = useState<number | undefined>(undefined);
  const wrapRef = useRef<HTMLDivElement>(null);
  const close = () => setOpen(false);

  // Flip: the composer sits near the top on the Home hero (menus would overflow
  // above the viewport) but at the bottom in a session. Measure on open, drop
  // toward the larger side, and cap the panel to the space that side actually
  // has (W13: a fixed max-height taller than the room above still overflowed
  // past the top of the viewport).
  useLayoutEffect(() => {
    if (!open) return;
    const el = wrapRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const above = rect.top;
    const below = window.innerHeight - rect.bottom;
    const down = above < 360 && below > above;
    setDrop(down ? "down" : "up");
    setMaxH(Math.max(160, (down ? below : above) - 16));
  }, [open]);
  const toggle = () =>
    setOpen((v) => {
      if (!v) onOpen?.();
      return !v;
    });

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className="pop-wrap" ref={wrapRef}>
      {trigger(open, toggle)}
      {open && (
        <div
          className={`pop-panel pop-${align} pop-${drop} ${panelClass}`}
          role="menu"
          style={maxH !== undefined ? { maxHeight: maxH } : undefined}
        >
          {children(close)}
        </div>
      )}
    </div>
  );
}

// PopSection / PopItem / PopHint are the building blocks used inside a Popover
// panel — a labelled group, a selectable row (with optional check + description),
// and a small footer hint.
export function PopSection({ label, children }: { label?: string; children: React.ReactNode }) {
  return (
    <div className="pop-section">
      {label && <div className="pop-section-label">{label}</div>}
      {children}
    </div>
  );
}

export function PopItem({
  onClick,
  active,
  icon,
  title,
  desc,
  right,
  danger,
  highlight,
}: {
  onClick: () => void;
  active?: boolean;
  icon?: React.ReactNode;
  title: React.ReactNode;
  desc?: React.ReactNode;
  right?: React.ReactNode;
  danger?: boolean;
  highlight?: boolean;
}) {
  return (
    <button
      className={"pop-item" + (danger ? " danger" : "") + (highlight ? " hl" : "")}
      onClick={onClick}
      role="menuitem"
    >
      {icon !== undefined && <span className="pop-ico">{icon}</span>}
      <span className="pop-body">
        <span className="pop-title">{title}</span>
        {desc && <span className="pop-desc">{desc}</span>}
      </span>
      {right !== undefined ? <span className="pop-right">{right}</span> : active ? <span className="pop-check">✓</span> : null}
    </button>
  );
}
