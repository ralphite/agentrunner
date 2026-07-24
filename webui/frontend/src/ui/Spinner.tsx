import {
  forwardRef,
  type HTMLAttributes,
} from "react";
import { CircleNotch } from "@phosphor-icons/react";

export type SpinnerSize = "sm" | "md" | "lg";
export type SpinnerDisplay = "inline" | "standalone";

export interface SpinnerProps
  extends Omit<HTMLAttributes<HTMLSpanElement>, "children"> {
  size?: SpinnerSize;
  display?: SpinnerDisplay;
  /** Visible and accessible copy. Without it, the accessible name is Loading. */
  label?: string;
  className?: string;
}

const ICON_SIZE: Record<SpinnerSize, number> = {
  sm: 12,
  md: 16,
  lg: 20,
};

const LABEL_SIZE: Record<SpinnerSize, string> = {
  sm: "text-[11px]",
  md: "text-[12px]",
  lg: "text-[13px]",
};

/**
 * Shared indeterminate loading indicator. Animation automatically stops for
 * people who request reduced motion while the loading semantics remain intact.
 */
export const Spinner = forwardRef<HTMLSpanElement, SpinnerProps>(
  function Spinner(
    {
      className,
      display = "inline",
      label,
      role = "status",
      size = "md",
      ...props
    },
    ref,
  ) {
    const decorative =
      props["aria-hidden"] === true || props["aria-hidden"] === "true";

    return (
      <span
        {...props}
        ref={ref}
        role={decorative ? undefined : role}
        aria-label={decorative ? undefined : label || "Loading"}
        aria-busy={decorative ? undefined : "true"}
        data-display={display}
        data-size={size}
        className={[
          display === "standalone"
            ? "flex min-h-20 w-full flex-col items-center justify-center gap-2 text-center"
            : "inline-flex items-center gap-1.5",
          "text-dim",
          LABEL_SIZE[size],
          className,
        ]
          .filter(Boolean)
          .join(" ")}
      >
        <CircleNotch
          size={ICON_SIZE[size]}
          className="shrink-0 motion-safe:animate-spin motion-reduce:animate-none"
          aria-hidden="true"
        />
        {label ? <span>{label}</span> : null}
      </span>
    );
  },
);
