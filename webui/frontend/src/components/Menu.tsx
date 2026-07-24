import { Popover } from "./Popover";
import { IconButton } from "../ui/IconButton";

// Menu is a small click-to-open dropdown used to tuck the low-level /
// developer actions (journal, inspect, fork, resume…) out of the primary UX,
// the way Codex keeps a clean session surface and hides plumbing.
export function Menu({
  label,
  children,
  ariaLabel,
  triggerClassName = "",
  iconTrigger = false,
}: {
  label: React.ReactNode;
  children: React.ReactNode;
  ariaLabel?: string;
  triggerClassName?: string;
  iconTrigger?: boolean;
}) {
  return (
    <div className="menu">
      <Popover
        align="right"
        panelClass="menu-pop"
        trigger={(open, toggle) => {
          const className = `menu-trigger${triggerClassName ? ` ${triggerClassName}` : ""}`;
          const triggerProps = {
            className,
            onClick: toggle,
            "aria-label": ariaLabel,
            "aria-haspopup": "menu" as const,
            "aria-expanded": open,
          };
          return iconTrigger && ariaLabel ? (
            <IconButton
              {...triggerProps}
              size="sm"
              variant="ghost"
              aria-label={ariaLabel}
            >
              {label}
            </IconButton>
          ) : (
            <button {...triggerProps}>{label}</button>
          );
        }}
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

export function MenuLabel({
  children,
  title,
}: {
  children: React.ReactNode;
  title?: string;
}) {
  return (
    <div
      className="menu-label"
      title={title ?? (typeof children === "string" ? children : undefined)}
    >
      {children}
    </div>
  );
}
