import { useRef } from "react";
import { Popover } from "./Popover";

// Menu is a small click-to-open dropdown used to tuck the low-level /
// developer actions (journal, inspect, fork, resume…) out of the primary UX,
// the way Codex keeps a clean session surface and hides plumbing.
export function Menu({ label, children, ariaLabel, triggerClassName = "" }: { label: React.ReactNode; children: React.ReactNode; ariaLabel?: string; triggerClassName?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  return (
    <div className="menu" ref={ref}>
      <Popover
        align="right"
        panelClass="menu-pop"
        trigger={(open, toggle) => (
          <button
            className={`menu-trigger${triggerClassName ? ` ${triggerClassName}` : ""}`}
            onClick={() => {
              toggle();
              if (!open) requestAnimationFrame(() => ref.current?.querySelector<HTMLElement>("[role='menuitem']")?.focus());
            }}
            aria-label={ariaLabel}
            aria-haspopup="menu"
            aria-expanded={open}
          >
            {label}
          </button>
        )}
      >
        {(close) => <div className="contents" onClick={close}>{children}</div>}
      </Popover>
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
