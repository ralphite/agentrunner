import { useEffect, useLayoutEffect, useRef, useState } from "react";

const VIEWPORT_GUTTER = 8;

// ContextMenu is a cursor-anchored popup (Codex's right-click chat menu). Unlike
// Menu (which hangs off a trigger button), this renders at fixed (x, y) and
// closes on outside click, Esc, or scroll. Items reuse .menu-item / .menu-label.
export function ContextMenu({
  x,
  y,
  onClose,
  children,
}: {
  x: number;
  y: number;
  onClose: () => void;
  children: React.ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  // Capture the invoking control before the first menu item takes focus. A
  // keyboard context menu is a temporary focus excursion: Escape must return
  // to the row that opened it, while item clicks/outside dismissal are free to
  // move focus into their resulting action (dialog, navigation, etc.).
  const returnFocusRef = useRef<HTMLElement | null>(
    document.activeElement instanceof HTMLElement ? document.activeElement : null,
  );
  const [position, setPosition] = useState({
    left: Math.max(VIEWPORT_GUTTER, x),
    top: Math.max(VIEWPORT_GUTTER, y),
  });

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

  useEffect(() => {
    requestAnimationFrame(() => ref.current?.querySelector<HTMLElement>("[role='menuitem']")?.focus());
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
        const target = returnFocusRef.current;
        requestAnimationFrame(() => {
          if (target?.isConnected) target.focus();
        });
        return;
      }
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
      const items = Array.from(ref.current?.querySelectorAll<HTMLElement>("[role='menuitem']:not(:disabled)") || []);
      if (!items.length) return;
      e.preventDefault();
      const current = Math.max(0, items.indexOf(document.activeElement as HTMLElement));
      const next = e.key === "Home" ? 0 : e.key === "End" ? items.length - 1 : e.key === "ArrowDown" ? (current + 1) % items.length : (current - 1 + items.length) % items.length;
      items[next].focus();
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    // Any scroll invalidates the cursor position — dismiss rather than float stale.
    window.addEventListener("scroll", onClose, true);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", onClose, true);
    };
  }, [onClose]);

  return (
    <div
      className="ctx-menu"
      ref={ref}
      style={position}
      role="menu"
      onClick={onClose}
    >
      {children}
    </div>
  );
}
