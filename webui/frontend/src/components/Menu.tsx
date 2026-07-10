import { useEffect, useRef, useState } from "react";

// Menu is a small click-to-open dropdown used to tuck the low-level /
// developer actions (journal, inspect, fork, resume…) out of the primary UX,
// the way Codex keeps a clean task surface and hides plumbing.
export function Menu({ label, children, ariaLabel }: { label: React.ReactNode; children: React.ReactNode; ariaLabel?: string }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    requestAnimationFrame(() => ref.current?.querySelector<HTMLElement>("[role='menuitem']")?.focus());
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        setOpen(false);
        ref.current?.querySelector<HTMLElement>(".menu-trigger")?.focus();
        return;
      }
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
      const items = Array.from(ref.current?.querySelectorAll<HTMLElement>("[role='menuitem']:not(:disabled)") || []);
      if (!items.length) return;
      e.preventDefault();
      const current = Math.max(0, items.indexOf(document.activeElement as HTMLElement));
      const next = e.key === "Home" ? 0 : e.key === "End" ? items.length - 1 : e.key === "ArrowDown" ? (current + 1) % items.length : (current - 1 + items.length) % items.length;
      items[next].focus();
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);
  return (
    <div className="menu" ref={ref}>
      <button className="menu-trigger" onClick={() => setOpen((v) => !v)} aria-label={ariaLabel} aria-haspopup="menu" aria-expanded={open}>
        {label}
      </button>
      {open && (
        <div className="menu-pop" role="menu" onClick={() => setOpen(false)}>
          {children}
        </div>
      )}
    </div>
  );
}

export function MenuItem({
  onClick,
  children,
  danger,
  title,
}: {
  onClick: () => void;
  children: React.ReactNode;
  danger?: boolean;
  title?: string;
}) {
  return (
    <button className={"menu-item" + (danger ? " danger" : "")} role="menuitem" onClick={onClick} title={title}>
      {children}
    </button>
  );
}

export function MenuLabel({ children }: { children: React.ReactNode }) {
  return <div className="menu-label">{children}</div>;
}
