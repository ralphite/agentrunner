import { useEffect, useRef } from "react";

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
  useEffect(() => {
    requestAnimationFrame(() => ref.current?.querySelector<HTMLElement>("[role='menuitem']")?.focus());
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
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

  // Clamp so the menu stays on-screen near the edges.
  const style: React.CSSProperties = {
    left: Math.min(x, window.innerWidth - 220),
    top: Math.min(y, window.innerHeight - 250),
  };
  return (
    <div
      className="ctx-menu fixed z-[80] min-w-[200px] max-w-[240px] rounded-[10px] border border-line bg-panel p-[5px] shadow-[0_10px_34px_rgba(0,0,0,0.22)]"
      ref={ref}
      style={style}
      role="menu"
      onClick={onClose}
    >
      {children}
    </div>
  );
}
