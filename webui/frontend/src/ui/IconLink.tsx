import {
  forwardRef,
  type AnchorHTMLAttributes,
  type ReactNode,
} from "react";
import {
  buttonClassName,
  type ButtonSize,
  type ButtonTone,
  type ButtonVariant,
} from "./Button";

export interface IconLinkProps
  extends Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "aria-label" | "children"> {
  "aria-label": string;
  children: ReactNode;
  size?: ButtonSize;
  tone?: ButtonTone;
  variant?: ButtonVariant;
}

const SQUARE_SIZE_CLASSES: Record<ButtonSize, string> = {
  sm: "w-6 !px-0",
  md: "w-8 !px-0",
  lg: "w-10 !px-0",
};

const LINK_INTERACTION_CLASSES: Record<
  ButtonTone,
  Record<ButtonVariant, string>
> = {
  neutral: {
    ghost: "hover:border-line hover:bg-panel-2 hover:text-ink active:bg-line-2",
    outline: "hover:border-dim hover:bg-panel-2 active:bg-line-2",
    solid: "hover:opacity-90 active:opacity-80",
  },
  danger: {
    ghost: "hover:border-red hover:bg-red-soft active:bg-red-soft",
    outline: "hover:border-red hover:bg-red-soft active:bg-red-soft",
    solid: "hover:opacity-90 active:opacity-80",
  },
  inverse: {
    ghost:
      "hover:border-white/20 hover:bg-white/10 hover:text-white active:bg-white/20",
    outline: "hover:border-white/30 hover:bg-white/20 active:bg-white/30",
    solid: "hover:opacity-90 active:opacity-80",
  },
};

/**
 * Anchor counterpart to IconButton. Navigation and downloads keep native link
 * semantics while sharing the same geometry, tones, tooltip, and touch target.
 */
export const IconLink = forwardRef<HTMLAnchorElement, IconLinkProps>(
  function IconLink(
    {
      "aria-label": accessibleLabel,
      children,
      className,
      size = "md",
      title,
      tone = "neutral",
      variant = "ghost",
      ...props
    },
    ref,
  ) {
    return (
      <a
        {...props}
        ref={ref}
        aria-label={accessibleLabel}
        title={title ?? accessibleLabel}
        data-ui-button=""
        data-ui-icon-button=""
        data-size={size}
        data-tone={tone}
        data-variant={variant}
        className={buttonClassName({
          className: [
            SQUARE_SIZE_CLASSES[size],
            "no-underline hover:no-underline",
            className,
          ]
            .concat(LINK_INTERACTION_CLASSES[tone][variant])
            .filter(Boolean)
            .join(" "),
          size,
          tone,
          variant,
        })}
      >
        <span className="inline-flex items-center justify-center" aria-hidden="true">
          {children}
        </span>
      </a>
    );
  },
);
