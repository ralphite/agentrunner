import {
  forwardRef,
  useLayoutEffect,
  useRef,
  type HTMLAttributes,
  type MutableRefObject,
  type Ref,
  type RefObject,
} from "react";

export type FocusTarget =
  | string
  | HTMLElement
  | RefObject<HTMLElement | null>
  | (() => HTMLElement | null);

export interface FocusScopeProps extends Omit<HTMLAttributes<HTMLDivElement>, "onEscape"> {
  /**
   * Preferred first-focus target(s). Selectors are resolved within the scope;
   * the first available target wins, then the first tabbable, then the root.
   */
  initialFocus?: FocusTarget | FocusTarget[];
  /** Restore the element active before mount, an explicit target, or nothing. */
  restoreFocus?: boolean | FocusTarget;
  /** Evaluated during cleanup so callers can suppress restore while closing. */
  shouldRestoreFocus?: () => boolean;
  /** Called only by the top-most active scope. */
  onEscape?: (event: KeyboardEvent) => void;
  /** Lets a persistent surface activate/deactivate the focus contract. */
  enabled?: boolean;
}

const FOCUSABLE_SELECTOR = [
  "a[href]",
  "area[href]",
  "button",
  "input:not([type='hidden'])",
  "select",
  "textarea",
  "iframe",
  "object",
  "embed",
  "audio[controls]",
  "video[controls]",
  "summary",
  "[contenteditable]:not([contenteditable='false'])",
  "[tabindex]",
].join(",");

const activeScopes: symbol[] = [];

function isUnavailable(element: HTMLElement, allowProgrammatic = false): boolean {
  if (
    element.matches(":disabled, [disabled], [hidden], [inert], [aria-hidden='true']") ||
    (!allowProgrammatic && element.tabIndex < 0)
  ) {
    return true;
  }

  for (let node: HTMLElement | null = element; node; node = node.parentElement) {
    if (node.hidden || node.matches("[inert], [aria-hidden='true']")) return true;
    const style = window.getComputedStyle(node);
    if (style.display === "none" || style.visibility === "hidden") return true;
  }
  return false;
}

function focusableElements(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (element) => !isUnavailable(element),
  );
}

function resolveTarget(
  target: FocusTarget | undefined,
  selectorRoot: ParentNode,
): HTMLElement | null {
  if (!target) return null;
  if (typeof target === "string") {
    return Array.from(selectorRoot.querySelectorAll<HTMLElement>(target))
      .find((element) => !isUnavailable(element, true)) || null;
  }
  if (typeof target === "function") return target();
  if ("current" in target) return target.current;
  return target;
}

function focusElement(element: HTMLElement | null, allowProgrammatic = false): boolean {
  if (!element || !element.isConnected || isUnavailable(element, allowProgrammatic)) return false;
  element.focus({ preventScroll: true });
  return document.activeElement === element;
}

export function useFocusScope(
  rootRef: RefObject<HTMLElement | null>,
  {
    initialFocus,
    restoreFocus = true,
    shouldRestoreFocus,
    onEscape,
    enabled = true,
  }: Pick<
    FocusScopeProps,
    "initialFocus" | "restoreFocus" | "shouldRestoreFocus" | "onEscape" | "enabled"
  >,
) {
  const optionsRef = useRef({
    initialFocus,
    restoreFocus,
    shouldRestoreFocus,
    onEscape,
  });
  optionsRef.current = {
    initialFocus,
    restoreFocus,
    shouldRestoreFocus,
    onEscape,
  };

  useLayoutEffect(() => {
    if (!enabled) return;
    const root = rootRef.current;
    if (!root) return;

    const id = Symbol("focus-scope");
    const previous = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
    activeScopes.push(id);

    const targets = Array.isArray(optionsRef.current.initialFocus)
      ? optionsRef.current.initialFocus
      : [optionsRef.current.initialFocus];
    const preferred = targets
      .map((target) => resolveTarget(target, root))
      .find((target): target is HTMLElement => !!target && !isUnavailable(target, target === root));
    const focusTarget = preferred || focusableElements(root)[0] || root;
    if (!focusElement(focusTarget, focusTarget === root)) {
      root.focus({ preventScroll: true });
    }

    const onKeyDown = (event: KeyboardEvent) => {
      if (activeScopes[activeScopes.length - 1] !== id) return;
      if (event.key === "Escape" && optionsRef.current.onEscape) {
        event.preventDefault();
        event.stopPropagation();
        optionsRef.current.onEscape(event);
        return;
      }
      if (event.key !== "Tab") return;

      const items = focusableElements(root);
      if (items.length === 0) {
        event.preventDefault();
        root.focus({ preventScroll: true });
        return;
      }

      const active = document.activeElement;
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && (active === first || !root.contains(active))) {
        event.preventDefault();
        last.focus({ preventScroll: true });
      } else if (!event.shiftKey && (active === last || !root.contains(active))) {
        event.preventDefault();
        first.focus({ preventScroll: true });
      }
    };

    document.addEventListener("keydown", onKeyDown, true);
    return () => {
      document.removeEventListener("keydown", onKeyDown, true);
      const index = activeScopes.lastIndexOf(id);
      if (index >= 0) activeScopes.splice(index, 1);

      const options = optionsRef.current;
      if (options.restoreFocus === false || options.shouldRestoreFocus?.() === false) return;
      const explicit = options.restoreFocus === true
        ? null
        : resolveTarget(options.restoreFocus, document);
      if (!focusElement(explicit, true)) focusElement(previous, true);
    };
  }, [enabled, rootRef]);
}

function assignRef<T>(ref: Ref<T> | undefined, value: T | null) {
  if (typeof ref === "function") ref(value);
  else if (ref) (ref as MutableRefObject<T | null>).current = value;
}

export const FocusScope = forwardRef<HTMLDivElement, FocusScopeProps>(
  function FocusScope(
    {
      initialFocus,
      restoreFocus = true,
      shouldRestoreFocus,
      onEscape,
      enabled = true,
      tabIndex = -1,
      ...props
    },
    forwardedRef,
  ) {
    const rootRef = useRef<HTMLDivElement>(null);
    useFocusScope(rootRef, {
      initialFocus,
      restoreFocus,
      shouldRestoreFocus,
      onEscape,
      enabled,
    });

    return (
      <div
        {...props}
        ref={(node) => {
          assignRef(rootRef, node);
          assignRef(forwardedRef, node);
        }}
        tabIndex={tabIndex}
      />
    );
  },
);
