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
  sm: "h-6 gap-1 px-2 py-0 text-[12px]",
  md: "h-8 gap-1.5 px-3 py-0 text-[13px]",
  lg: "h-10 gap-2 px-4 py-0 text-[14px]",
};

const SPINNER_SIZE: Record<ButtonSize, "sm" | "md" | "lg"> = {
  sm: "sm",
  md: "md",
  lg: "lg",
};

const APPEARANCE_CLASSES: Record<
  ButtonTone,
  Record<ButtonVariant, string>
> = {
  neutral: {
    ghost:
      "border-transparent bg-transparent text-ink-2 shadow-none enabled:hover:border-line enabled:hover:bg-panel-2 enabled:hover:text-ink enabled:hover:shadow-none enabled:active:bg-line-2 aria-[pressed=true]:border-line aria-[pressed=true]:bg-panel-2 aria-[pressed=true]:text-ink disabled:border-transparent disabled:bg-transparent disabled:text-ink-2 disabled:shadow-none",
    outline:
      "border-line bg-panel text-ink shadow-none enabled:hover:border-dim enabled:hover:bg-panel-2 enabled:active:bg-line-2 aria-[pressed=true]:border-dim aria-[pressed=true]:bg-panel-2 disabled:border-line disabled:bg-panel disabled:text-ink-2 disabled:shadow-none",
    solid:
      "border-accent bg-accent text-accent-ink shadow-none enabled:hover:border-accent enabled:hover:bg-accent enabled:hover:opacity-90 enabled:active:opacity-80 aria-[pressed=true]:opacity-80 disabled:border-accent disabled:bg-accent disabled:text-accent-ink disabled:shadow-none",
  },
  danger: {
    ghost:
      "border-transparent bg-transparent text-red shadow-none enabled:hover:border-red enabled:hover:bg-red-soft enabled:active:bg-red-soft aria-[pressed=true]:border-red aria-[pressed=true]:bg-red-soft disabled:border-transparent disabled:bg-transparent disabled:text-red disabled:shadow-none",
    outline:
      "border-red bg-panel text-red shadow-none enabled:hover:border-red enabled:hover:bg-red-soft enabled:active:bg-red-soft aria-[pressed=true]:bg-red-soft disabled:border-red disabled:bg-panel disabled:text-red disabled:shadow-none",
    solid:
      "border-red bg-red text-accent-ink shadow-none enabled:hover:border-red enabled:hover:bg-red enabled:hover:opacity-90 enabled:active:opacity-80 aria-[pressed=true]:opacity-80 disabled:border-red disabled:bg-red disabled:text-accent-ink disabled:shadow-none",
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
    "relative m-0 inline-flex shrink-0 select-none items-center justify-center whitespace-nowrap rounded-[8px] border font-medium leading-none transition-[background-color,border-color,color,opacity,box-shadow] duration-100",
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
