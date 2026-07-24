import { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  getAdjacentTabbableElement,
  getAvailableMenuItems,
  isAvailableMenuItem,
  setRovingMenuItem,
} from "./menuFocus";
import { useFocusScope } from "../ui/FocusScope";

const VIEWPORT_GUTTER = 8;

// ContextMenu is a cursor-anchored popup (Codex's right-click chat menu). Unlike
// Menu (which hangs off a trigger button), this renders at fixed (x, y) and
// closes on outside click, Esc, or scroll. Items reuse .menu-item / .menu-label.
export function ContextMenu({
  x,
  y,
  onClose,
  children,
  ariaLabel,
  returnFocus,
}: {
  x: number;
  y: number;
  onClose: () => void;
  children: React.ReactNode;
  ariaLabel?: string;
  returnFocus?: HTMLElement | null;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  const rovingItemRef = useRef<HTMLElement | null>(null);
  const autoFocusedRef = useRef(false);
  // Callers can name the exact invoking row/button. The active-element fallback
  // preserves existing call sites and is captured before the first item focuses.
  const returnFocusRef = useRef<HTMLElement | null>(
    returnFocus !== undefined
      ? returnFocus
      : document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null,
  );
  const [position, setPosition] = useState({
    left: Math.max(VIEWPORT_GUTTER, x),
    top: Math.max(VIEWPORT_GUTTER, y),
  });
  const restoreReturnFocus = () => {
    const target = returnFocusRef.current;
    if (target?.isConnected) target.focus();
    window.setTimeout(() => {
      if (target?.isConnected) target.focus();
    }, 0);
  };

  // A cursor menu can live inside another focus scope (notably the mobile
  // sidebar). Register it as the top focus/Escape layer so the parent yields,
  // but disable Tab trapping because this menu deliberately hands Tab back to
  // the surrounding page order.
  useFocusScope(ref, {
    initialFocus: () => {
      const panel = ref.current;
      return panel ? getAvailableMenuItems(panel)[0] ?? null : null;
    },
    restoreFocus: false,
    onEscape: () => {
      onCloseRef.current();
      restoreReturnFocus();
    },
    trapTab: false,
  });

  useLayoutEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useLayoutEffect(() => {
    const panel = ref.current;
    if (!panel) return;

    const place = () => {
      const { width, height } = panel.getBoundingClientRect();
      const left = Math.min(
        Math.max(VIEWPORT_GUTTER, x),
        Math.max(VIEWPORT_GUTTER, window.innerWidth - width - VIEWPORT_GUTTER),
      );
      const top = Math.min(
        Math.max(VIEWPORT_GUTTER, y),
        Math.max(VIEWPORT_GUTTER, window.innerHeight - height - VIEWPORT_GUTTER),
      );
      setPosition((current) => current.left === left && current.top === top ? current : { left, top });
    };

    place();
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(place);
    observer?.observe(panel);
    window.addEventListener("resize", place);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", place);
    };
  }, [x, y]);

  useLayoutEffect(() => {
    const panel = ref.current;
    if (!panel) return;
    const target = getAvailableMenuItems(panel)[0];
    setRovingMenuItem(panel, target ?? null);
    rovingItemRef.current = target ?? null;
    if (!target) return;
    target.focus();
    if (document.activeElement === target) autoFocusedRef.current = true;

    // Synthetic pointer sequences can finish after layout effects. Reassert
    // once, but never steal focus from another item or a resulting surface.
    const retry = window.setTimeout(() => {
      if (
        !target.isConnected ||
        document.activeElement === target ||
        panel.contains(document.activeElement)
      ) {
        return;
      }
      setRovingMenuItem(panel, target);
      rovingItemRef.current = target;
      target.focus();
      if (document.activeElement === target) autoFocusedRef.current = true;
    }, 50);
    return () => window.clearTimeout(retry);
  }, []);

  useEffect(() => {
    const panel = ref.current;
    if (!panel) return;

    const syncRovingItem = () => {
      const items = getAvailableMenuItems(panel);
      const active =
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null;
      const previous = rovingItemRef.current;
      const target =
        (active && items.includes(active) ? active : null) ||
        (previous && items.includes(previous) ? previous : null) ||
        items.find((item) => item.tabIndex === 0) ||
        items[0] ||
        null;

      setRovingMenuItem(panel, target);
      rovingItemRef.current = target;
      if (!target) {
        autoFocusedRef.current = false;
        return;
      }

      const previousBecameUnavailable =
        !!previous &&
        (!previous.isConnected || !items.includes(previous));
      if (!autoFocusedRef.current || previousBecameUnavailable) {
        target.focus();
        if (document.activeElement === target) {
          autoFocusedRef.current = true;
        }
      }
    };

    syncRovingItem();
    const observer = new MutationObserver(syncRovingItem);
    observer.observe(panel, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: [
        "aria-disabled",
        "aria-hidden",
        "class",
        "disabled",
        "hidden",
        "inert",
        "style",
      ],
    });
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const restoreIfStranded = (selectedItem: HTMLElement | null) => {
      const target = returnFocusRef.current;
      window.setTimeout(() => {
        const active =
          document.activeElement instanceof HTMLElement
            ? document.activeElement
            : null;
        const stranded =
          !active ||
          active === document.body ||
          active === document.documentElement ||
          active === selectedItem ||
          !active.isConnected;
        if (stranded && target?.isConnected) target.focus();
      }, 0);
    };
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onCloseRef.current();
      }
    };
    const onKey = (e: KeyboardEvent) => {
      const panel = ref.current;
      if (!panel) return;
      const active =
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null;
      if (e.key === "Tab") {
        const items = getAvailableMenuItems(panel);
        // A dynamic menu can briefly become empty after its focused item is
        // removed. In that state focus falls back to <body>; still treat Tab as
        // leaving the temporary surface instead of leaving an empty menu open.
        if ((!active || !panel.contains(active)) && items.length > 0) return;
        const target = getAdjacentTabbableElement(
          returnFocusRef.current,
          panel,
          e.shiftKey,
        );
        e.preventDefault();
        onCloseRef.current();
        target?.focus();
        window.setTimeout(() => {
          if (target?.isConnected) target.focus();
        }, 0);
        return;
      }
      if (!active || !panel.contains(active)) return;
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
      const items = getAvailableMenuItems(panel);
      if (!items.length) return;
      e.preventDefault();
      const current = items.indexOf(active);
      const next =
        e.key === "Home"
          ? 0
          : e.key === "End"
            ? items.length - 1
            : e.key === "ArrowDown"
              ? current < 0 || current === items.length - 1
                ? 0
                : current + 1
              : current <= 0
                ? items.length - 1
                : current - 1;
      setRovingMenuItem(panel, items[next]);
      rovingItemRef.current = items[next];
      items[next].focus();
    };
    const onScroll = (e: Event) => {
      const panel = ref.current;
      if (!panel) return;
      if (e.target instanceof Node && panel.contains(e.target)) return;
      const active =
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null;
      const focusWasInMenu = !!active && panel.contains(active);
      onCloseRef.current();
      if (focusWasInMenu) restoreIfStranded(active);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    // Any scroll invalidates the cursor position — dismiss rather than float stale.
    window.addEventListener("scroll", onScroll, true);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", onScroll, true);
    };
  }, []);

  return (
    <div
      className="ctx-menu [&_.menu-item:focus-visible]:bg-panel-2 max-[900px]:[&_.menu-item]:min-h-11 [@media(any-pointer:coarse)]:[&_.menu-item]:min-h-11"
      ref={ref}
      style={position}
      role="menu"
      aria-label={ariaLabel}
      onClickCapture={(event) => {
        const item = (event.target as HTMLElement).closest<HTMLElement>(
          '[role="menuitem"]',
        );
        const target = returnFocusRef.current;
        if (
          item &&
          ref.current?.contains(item) &&
          isAvailableMenuItem(item) &&
          target?.isConnected
        ) {
          // FocusScope records the active element when a menu action opens a
          // dialog. Seed it with the durable invoking row before the item's
          // onClick runs, so closing Rename/Confirm/Prompt returns to the real
          // caller rather than the menuitem that unmounts in this same event.
          target.focus({ preventScroll: true });
        }
      }}
      onFocusCapture={(event) => {
        const item = (event.target as HTMLElement).closest<HTMLElement>(
          '[role="menuitem"]',
        );
        if (
          item &&
          ref.current?.contains(item) &&
          isAvailableMenuItem(item)
        ) {
          setRovingMenuItem(ref.current, item);
          rovingItemRef.current = item;
        }
      }}
      onClick={(event) => {
        const item = (event.target as HTMLElement).closest<HTMLElement>(
          '[role="menuitem"]',
        );
        if (
          !item ||
          !ref.current?.contains(item) ||
          !isAvailableMenuItem(item)
        ) {
          return;
        }
        onCloseRef.current();
        const target = returnFocusRef.current;
        window.setTimeout(() => {
          const active =
            document.activeElement instanceof HTMLElement
              ? document.activeElement
              : null;
          const stranded =
            !active ||
            active === document.body ||
            active === document.documentElement ||
            active === item ||
            !active.isConnected;
          if (stranded && target?.isConnected) target.focus();
        }, 0);
      }}
    >
      {children}
    </div>
  );
}
