import {
  Circle,
  CircleNotch,
  CheckCircle,
  WarningCircle,
  XCircle,
} from "@phosphor-icons/react";
import {
  forwardRef,
  type HTMLAttributes,
  type ReactNode,
} from "react";

export type LifecycleStatusState =
  | "running"
  | "done"
  | "waiting"
  | "idle"
  | "attention"
  | "failed";

export type LifecycleStatusSize = "sm" | "md";

export interface LifecycleStatusProps
  extends Omit<HTMLAttributes<HTMLSpanElement>, "aria-label" | "children"> {
  state: LifecycleStatusState;
  /** Contextual name announced to assistive technology. */
  accessibleLabel: string;
  /** Optional compact copy shown beside the glyph. */
  visibleLabel?: ReactNode;
  size?: LifecycleStatusSize;
  className?: string;
}

const ICON_SIZE: Record<LifecycleStatusSize, number> = {
  sm: 13,
  md: 16,
};

const STATE_CLASS: Record<LifecycleStatusState, string> = {
  running: "text-blue",
  done: "text-green",
  waiting: "text-dim",
  idle: "text-dim",
  attention: "text-amber",
  failed: "text-red",
};

/**
 * Keeps the daemon's existing friendly-status classification while presenting
 * one Codex-like lifecycle vocabulary.
 */
export function lifecycleStateFromStatusClass(
  cls: string,
): LifecycleStatusState {
  if (cls === "run") return "running";
  if (cls === "closed") return "done";
  if (cls === "appr" || cls === "stranded") return "attention";
  if (cls === "crash") return "failed";
  if (cls === "idle") return "idle";
  return "waiting";
}

function lifecycleGlyph(state: LifecycleStatusState, size: number) {
  if (state === "running") {
    return (
      <CircleNotch
        aria-hidden="true"
        className="shrink-0 motion-safe:animate-spin motion-reduce:animate-none"
        size={size}
      />
    );
  }
  if (state === "done") {
    return <CheckCircle aria-hidden="true" size={size} weight="fill" />;
  }
  if (state === "attention") {
    return <WarningCircle aria-hidden="true" size={size} weight="fill" />;
  }
  if (state === "failed") {
    return <XCircle aria-hidden="true" size={size} weight="fill" />;
  }
  if (state === "idle") {
    return <Circle aria-hidden="true" size={size} weight="fill" />;
  }
  return <Circle aria-hidden="true" size={size} weight="regular" />;
}

/**
 * Shared lifecycle glyph. Visible copy is optional and deliberately separate
 * from the contextual accessible name so icon-only placements stay compact.
 */
export const LifecycleStatus = forwardRef<
  HTMLSpanElement,
  LifecycleStatusProps
>(function LifecycleStatus(
  {
    accessibleLabel,
    className,
    role = "img",
    size = "sm",
    state,
    visibleLabel,
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
      aria-label={decorative ? undefined : accessibleLabel}
      aria-busy={decorative || state !== "running" ? undefined : "true"}
      data-lifecycle-state={state}
      className={[
        "inline-flex min-w-0 shrink-0 items-center gap-1.5 align-middle !h-auto !w-auto !rounded-none !bg-transparent",
        STATE_CLASS[state],
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {lifecycleGlyph(state, ICON_SIZE[size])}
      {visibleLabel}
    </span>
  );
});
