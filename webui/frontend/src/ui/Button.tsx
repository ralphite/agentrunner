import {
  forwardRef,
  type ButtonHTMLAttributes,
  type ReactNode,
} from "react";
import { Spinner } from "./Spinner";

export type ButtonSize = "sm" | "md" | "lg";
export type ButtonVariant = "ghost" | "outline" | "solid";
export type ButtonTone = "neutral" | "danger" | "inverse";

export interface ButtonProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "aria-pressed"> {
  size?: ButtonSize;
  variant?: ButtonVariant;
  tone?: ButtonTone;
  pressed?: boolean;
  loading?: boolean;
  children: ReactNode;
  /**
   * Extends layout or positioning at the call site. Visual appearance belongs
   * to size, variant, and tone so action styling remains consistent.
   */
  className?: string;
}

export const BUTTON_SIZE_CLASSES: Record<ButtonSize, string> = {
  sm: "h-[var(--control-sm)] gap-1 px-2 py-0 text-[length:var(--type-meta)]",
  md: "h-[var(--control-md)] gap-1.5 px-3 py-0 text-[length:var(--type-ui)]",
  lg: "h-[var(--control-lg)] gap-2 px-4 py-0 text-[length:var(--type-body)]",
};

const SPINNER_SIZE: Record<ButtonSize, "sm" | "md" | "lg"> = {
  sm: "sm",
  md: "md",
  lg: "lg",
};

/* CX-BTN (T7/T16). Two changes from the previous matrix:
   1. Hover/press no longer swap background-color. They come from `.ix`, whose
      translucent overlay composes over each variant's own fill — so one rule
      serves ghost (transparent), outline (panel), and solid (accent) instead of
      each tone hand-picking a hover fill, and a pressed/selected button still
      darkens by a visible step.
   2. The outlined button matches Codex's secondary: a 9%-black hairline over
      white plus a 2% resting shadow, not a full --line box. That border is what
      made our secondary buttons read a weight heavier than Codex's.
   aria-pressed keeps an explicit fill: it is a persistent state, and an overlay
   that only exists while hovered cannot express it. */
const APPEARANCE_CLASSES: Record<
  ButtonTone,
  Record<ButtonVariant, string>
> = {
  neutral: {
    ghost:
      "border-transparent bg-transparent text-ink-2 shadow-none enabled:hover:text-ink aria-[pressed=true]:bg-panel-2 aria-[pressed=true]:text-ink disabled:border-transparent disabled:bg-transparent disabled:text-ink-2 disabled:shadow-none",
    outline:
      "border-card-line bg-panel text-ink shadow-[var(--shadow-micro)] [--ix-rest-shadow:var(--shadow-micro)] aria-[pressed=true]:bg-panel-2 disabled:border-card-line disabled:bg-panel disabled:text-ink-2",
    solid:
      "border-accent bg-accent text-accent-ink shadow-none disabled:border-accent disabled:bg-accent disabled:text-accent-ink disabled:shadow-none",
  },
  danger: {
    ghost:
      "border-transparent bg-transparent text-red shadow-none enabled:hover:bg-red-soft aria-[pressed=true]:bg-red-soft disabled:border-transparent disabled:bg-transparent disabled:text-red disabled:shadow-none",
    outline:
      "border-red bg-panel text-red shadow-none enabled:hover:bg-red-soft aria-[pressed=true]:bg-red-soft disabled:border-red disabled:bg-panel disabled:text-red disabled:shadow-none",
    solid:
      "border-red bg-red text-accent-ink shadow-none disabled:border-red disabled:bg-red disabled:text-accent-ink disabled:shadow-none",
  },
  inverse: {
    ghost:
      "border-transparent bg-transparent text-white/80 shadow-none enabled:hover:border-white/20 enabled:hover:bg-white/10 enabled:hover:text-white enabled:hover:shadow-none enabled:active:bg-white/20 aria-[pressed=true]:border-white/20 aria-[pressed=true]:bg-white/10 aria-[pressed=true]:text-white disabled:border-transparent disabled:bg-transparent disabled:text-white/50 disabled:shadow-none",
    outline:
      "border-white/20 bg-white/10 text-white shadow-none enabled:hover:border-white/30 enabled:hover:bg-white/20 enabled:active:bg-white/30 aria-[pressed=true]:border-white/30 aria-[pressed=true]:bg-white/20 disabled:border-white/10 disabled:bg-white/5 disabled:text-white/50 disabled:shadow-none",
    solid:
      "border-white bg-white text-black shadow-none enabled:hover:border-white enabled:hover:bg-white enabled:hover:opacity-90 enabled:active:opacity-80 aria-[pressed=true]:opacity-80 disabled:border-white disabled:bg-white disabled:text-black disabled:shadow-none",
  },
};

export function buttonClassName({
  className,
  size,
  tone,
  variant,
}: {
  className?: string;
  size: ButtonSize;
  tone: ButtonTone;
  variant: ButtonVariant;
}): string {
  return [
    "ix relative m-0 inline-flex shrink-0 select-none items-center justify-center whitespace-nowrap rounded-[var(--radius-control)] border font-medium leading-none transition-[background-color,border-color,color,opacity,box-shadow] duration-100",
    BUTTON_SIZE_CLASSES[size],
    APPEARANCE_CLASSES[tone][variant],
    className,
  ]
    .filter(Boolean)
    .join(" ");
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  function Button(
    {
      children,
      className,
      disabled = false,
      loading = false,
      pressed,
      size = "md",
      tone = "neutral",
      type = "button",
      variant = "outline",
      ...props
    },
    ref,
  ) {
    const unavailable = disabled || loading;

    return (
      <button
        {...props}
        ref={ref}
        type={type}
        disabled={unavailable}
        aria-busy={loading || undefined}
        aria-pressed={pressed}
        data-ui-button=""
        data-size={size}
        data-tone={tone}
        data-variant={variant}
        className={buttonClassName({ className, size, tone, variant })}
      >
        <span
          className={[
            "inline-flex min-w-0 max-w-full items-center justify-center gap-[inherit]",
            loading ? "opacity-0" : "",
          ]
            .filter(Boolean)
            .join(" ")}
        >
          {children}
        </span>
        {loading && (
          <Spinner
            aria-hidden="true"
            className="absolute"
            size={SPINNER_SIZE[size]}
          />
        )}
      </button>
    );
  },
);
