import {
  forwardRef,
  type HTMLAttributes,
} from "react";

export type StatusIndicatorDisplay = "dot" | "pill" | "text";
export type StatusIndicatorTone =
  | "neutral"
  | "info"
  | "success"
  | "warning"
  | "danger";

export interface StatusIndicatorProps
  extends Omit<HTMLAttributes<HTMLSpanElement>, "children"> {
  /** User-facing status name. It is also the accessible name. */
  label: string;
  display?: StatusIndicatorDisplay;
  tone?: StatusIndicatorTone;
  className?: string;
}

const TONE_CLASSES: Record<
  StatusIndicatorTone,
  Record<StatusIndicatorDisplay, string>
> = {
  neutral: {
    dot: "bg-status-terminal",
    pill: "border-line bg-panel-2 text-dim",
    text: "text-dim",
  },
  info: {
    dot: "bg-status-ready",
    pill: "border-blue/30 bg-blue-soft text-blue",
    text: "text-blue",
  },
  success: {
    dot: "bg-status-running",
    pill: "border-green/30 bg-green-soft text-green",
    text: "text-green",
  },
  warning: {
    dot: "bg-status-attention",
    pill: "border-amber/30 bg-amber-soft text-amber",
    text: "text-amber",
  },
  danger: {
    dot: "bg-status-failed",
    pill: "border-red/30 bg-red-soft text-red",
    text: "text-red",
  },
};

const DISPLAY_CLASSES: Record<StatusIndicatorDisplay, string> = {
  dot: "inline-block h-2 w-2 shrink-0 rounded-full",
  pill:
    "inline-flex min-h-5 max-w-full items-center rounded-full border px-2 py-0.5 text-[11px] font-medium leading-none",
  text: "inline min-w-0 text-[12px] font-medium",
};

/**
 * One visual and semantic contract for lifecycle state. Use aria-hidden when a
 * neighbouring text node already announces the same status.
 */
export const StatusIndicator = forwardRef<
  HTMLSpanElement,
  StatusIndicatorProps
>(function StatusIndicator(
  {
    className,
    display = "dot",
    label,
    role = "status",
    tone = "neutral",
    ...props
  },
  ref,
) {
  const decorative = props["aria-hidden"] === true || props["aria-hidden"] === "true";

  return (
    <span
      {...props}
      ref={ref}
      role={decorative ? undefined : role}
      aria-label={decorative ? undefined : label}
      data-display={display}
      data-tone={tone}
      className={[
        DISPLAY_CLASSES[display],
        TONE_CLASSES[tone][display],
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {display === "dot" ? null : label}
    </span>
  );
});
