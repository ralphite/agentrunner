import { Popover } from "./Popover";

// Menu is a small click-to-open dropdown used to tuck the low-level /
// developer actions (journal, inspect, fork, resume…) out of the primary UX,
// the way Codex keeps a clean session surface and hides plumbing.
export function Menu({ label, children, ariaLabel, triggerClassName = "" }: { label: React.ReactNode; children: React.ReactNode; ariaLabel?: string; triggerClassName?: string }) {
  return (
    <div className="menu">
      <Popover
        align="right"
        panelClass="menu-pop"
        trigger={(open, toggle) => (
          <button
            className={`menu-trigger${triggerClassName ? ` ${triggerClassName}` : ""}`}
            onClick={toggle}
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
  disabled,
}: {
  onClick: () => void;
  children: React.ReactNode;
  danger?: boolean;
  title?: string;
  disabled?: boolean;
}) {
  return (
    <button
      className={
        "menu-item" +
        (danger ? " danger" : "") +
        (disabled ? " opacity-45" : "")
      }
      role="menuitem"
      tabIndex={-1}
      onClick={onClick}
      title={title}
      disabled={disabled}
    >
      {children}
    </button>
  );
}

export function MenuLabel({ children }: { children: React.ReactNode }) {
  return <div className="menu-label">{children}</div>;
}
