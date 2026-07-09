import { useEffect, useRef, useState } from "react";

// Menu is a small click-to-open dropdown used to tuck the low-level /
// developer actions (journal, inspect, fork, resume…) out of the primary UX,
// the way Codex keeps a clean task surface and hides plumbing.
export function Menu({ label, children }: { label: string; children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);
  return (
    <div className="menu" ref={ref}>
      <button className="menu-trigger" onClick={() => setOpen((v) => !v)}>
        {label}
      </button>
      {open && (
        <div className="menu-pop" onClick={() => setOpen(false)}>
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
}: {
  onClick: () => void;
  children: React.ReactNode;
  danger?: boolean;
}) {
  return (
    <button className={"menu-item" + (danger ? " danger" : "")} onClick={onClick}>
      {children}
    </button>
  );
}

export function MenuLabel({ children }: { children: React.ReactNode }) {
  return <div className="menu-label">{children}</div>;
}
