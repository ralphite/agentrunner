import { forwardRef, type ReactNode } from "react";
import {
  Button,
  type ButtonProps,
  type ButtonSize,
} from "./Button";

export interface IconButtonProps
  extends Omit<ButtonProps, "aria-label" | "children"> {
  "aria-label": string;
  children: ReactNode;
}

const SQUARE_SIZE_CLASSES: Record<ButtonSize, string> = {
  sm: "w-6 !px-0",
  md: "w-8 !px-0",
  lg: "w-10 !px-0",
};

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  function IconButton(
    {
      "aria-label": accessibleLabel,
      children,
      className,
      size = "md",
      title,
      ...props
    },
    ref,
  ) {
    return (
      <Button
        {...props}
        ref={ref}
        size={size}
        aria-label={accessibleLabel}
        title={title ?? accessibleLabel}
        className={[SQUARE_SIZE_CLASSES[size], className]
          .filter(Boolean)
          .join(" ")}
      >
        <span className="inline-flex items-center justify-center" aria-hidden="true">
          {children}
        </span>
      </Button>
    );
  },
);
