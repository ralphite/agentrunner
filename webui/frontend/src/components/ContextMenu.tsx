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
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
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
    <div className="ctx-menu" ref={ref} style={style} onClick={onClose}>
      {children}
    </div>
  );
}
