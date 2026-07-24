import {
  cloneElement,
  forwardRef,
  isValidElement,
  useId,
  type InputHTMLAttributes,
  type KeyboardEventHandler,
  type ReactElement,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from "react";

export type FieldControlVariant = "default" | "unstyled";

const CONTROL_BASE =
  "min-w-0 font-[inherit] text-ink placeholder:text-dim transition-[background-color,border-color,box-shadow,color,opacity] duration-100 focus-visible:outline-none";
const CONTROL_VARIANTS: Record<FieldControlVariant, string> = {
  default:
    "rounded-[8px] border border-line bg-panel px-[9px] py-[7px] hover:border-dim focus:border-blue focus:outline-none focus:ring-2 focus:ring-blue/30 aria-[invalid=true]:border-red aria-[invalid=true]:focus:border-red aria-[invalid=true]:focus:ring-red/30 read-only:bg-panel-2 read-only:text-ink-2 disabled:cursor-not-allowed disabled:bg-panel-2 disabled:text-ink-2 disabled:opacity-50",
  unstyled:
    "rounded-none border-0 bg-transparent p-0 shadow-none hover:border-transparent focus:border-transparent focus:outline-none focus:ring-0 focus-visible:outline-none disabled:bg-transparent",
};

function cx(...classes: Array<string | undefined | false>) {
  return classes.filter(Boolean).join(" ");
}

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  variant?: FieldControlVariant;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, variant = "default", ...props },
  ref,
) {
  return (
    <input
      {...props}
      ref={ref}
      data-ui-input=""
      data-variant={variant}
      className={cx(CONTROL_BASE, CONTROL_VARIANTS[variant], className)}
    />
  );
});

export interface TextareaProps
  extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  variant?: FieldControlVariant;
  code?: boolean;
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  function Textarea(
    { className, code = false, variant = "default", ...props },
    ref,
  ) {
    return (
      <textarea
        {...props}
        ref={ref}
        data-ui-textarea=""
        data-variant={variant}
        data-code={code || undefined}
        className={cx(
          CONTROL_BASE,
          CONTROL_VARIANTS[variant],
          variant === "default" && "min-h-24 resize-y",
          code && "font-mono text-[length:var(--code-font-size)] leading-[1.55]",
          className,
        )}
      />
    );
  },
);

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  variant?: FieldControlVariant;
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  function Select({ className, variant = "default", ...props }, ref) {
    return (
      <select
        {...props}
        ref={ref}
        data-ui-select=""
        data-variant={variant}
        className={cx(
          CONTROL_BASE,
          CONTROL_VARIANTS[variant],
          "appearance-auto",
          className,
        )}
      />
    );
  },
);

interface FieldChildProps {
  id?: string;
  disabled?: boolean;
  required?: boolean;
  "aria-describedby"?: string;
  "aria-invalid"?: boolean | "false" | "true" | "grammar" | "spelling";
}

export interface FieldProps {
  label: ReactNode;
  children: ReactElement<FieldChildProps>;
  help?: ReactNode;
  error?: ReactNode;
  required?: boolean;
  disabled?: boolean;
  className?: string;
}

function describedBy(...ids: Array<string | undefined>) {
  const value = ids.filter(Boolean).join(" ");
  return value || undefined;
}

export function Field({
  children,
  className,
  disabled = false,
  error,
  help,
  label,
  required = false,
}: FieldProps) {
  const generatedId = useId();
  const controlId = children.props.id ?? `${generatedId}-control`;
  const helpId = help ? `${generatedId}-help` : undefined;
  const errorId = error ? `${generatedId}-error` : undefined;

  const control = isValidElement(children)
    ? cloneElement(children, {
        id: controlId,
        disabled: children.props.disabled ?? disabled,
        required: children.props.required ?? required,
        "aria-invalid": children.props["aria-invalid"] ?? (error ? true : undefined),
        "aria-describedby": describedBy(
          children.props["aria-describedby"],
          helpId,
          errorId,
        ),
      })
    : children;

  return (
    <div
      className={cx(
        "grid min-w-0 gap-1.5 text-[12px]",
        disabled && "text-dim",
        className,
      )}
      data-ui-field=""
      data-disabled={disabled || undefined}
      data-invalid={!!error || undefined}
    >
      <label
        className={cx(
          "min-w-0 font-medium leading-[1.4] text-ink-2",
          disabled && "text-dim",
        )}
        htmlFor={controlId}
      >
        {label}
        {required && (
          <span className="ml-0.5 text-red" aria-hidden="true">
            *
          </span>
        )}
      </label>
      {control}
      {help && (
        <span className="leading-[1.4] text-dim" id={helpId}>
          {help}
        </span>
      )}
      {error && (
        <span className="leading-[1.4] text-red" id={errorId}>
          {error}
        </span>
      )}
    </div>
  );
}

export type SearchFieldVariant = "default" | "flush" | "unstyled";

const SEARCH_VARIANTS: Record<SearchFieldVariant, string> = {
  default:
    "rounded-[8px] border border-line bg-panel px-[9px] py-[7px] hover:border-dim focus-within:border-blue focus-within:ring-2 focus-within:ring-blue/30",
  flush:
    "rounded-none border-x-0 border-t-0 border-b border-line bg-panel px-4 py-3 focus-within:border-blue focus-within:ring-2 focus-within:ring-inset focus-within:ring-blue/30",
  unstyled: "",
};

export interface SearchFieldProps
  extends Omit<InputProps, "children" | "variant"> {
  icon?: ReactNode;
  endActions?: ReactNode;
  containerClassName?: string;
  onContainerKeyDown?: KeyboardEventHandler<HTMLDivElement>;
  variant?: SearchFieldVariant;
}

export const SearchField = forwardRef<HTMLInputElement, SearchFieldProps>(
  function SearchField(
    {
      className,
      containerClassName,
      endActions,
      icon,
      onContainerKeyDown,
      type = "search",
      variant = "default",
      ...props
    },
    ref,
  ) {
    const invalid =
      props["aria-invalid"] === true || props["aria-invalid"] === "true";
    return (
      <div
        className={cx(
          "relative flex min-w-0 items-center gap-2 text-dim transition-[background-color,border-color,box-shadow] duration-100",
          SEARCH_VARIANTS[variant],
          invalid &&
            "border-red focus-within:border-red focus-within:ring-red/30",
          props.disabled && "cursor-not-allowed bg-panel-2 opacity-50",
          containerClassName,
        )}
        data-ui-search-field=""
        data-variant={variant}
        data-invalid={invalid || undefined}
        onKeyDown={onContainerKeyDown}
      >
        {icon && (
          <span
            className="inline-flex shrink-0 items-center justify-center"
            aria-hidden="true"
          >
            {icon}
          </span>
        )}
        <Input
          {...props}
          ref={ref}
          type={type}
          variant="unstyled"
          className={cx("min-w-0 flex-1 text-ink", className)}
        />
        {endActions && (
          <div className="inline-flex shrink-0 items-center gap-1 [&_:focus-visible]:outline-none">
            {endActions}
          </div>
        )}
      </div>
    );
  },
);
