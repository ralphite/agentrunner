import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowDown, ArrowUp, MagnifyingGlass, X } from "@phosphor-icons/react";
import { IconButton } from "../ui/IconButton";

// FindBar is Codex's in-chat Find (⌘F): a search box over the current
// conversation that highlights matches and steps through them with ↑/↓. It
// uses the CSS Custom Highlight API so it never mutates React-owned DOM —
// matches are painted via ::highlight() ranges, cleared on close.
const HL = "arfind";
const HL_CUR = "arfind-current";

function highlightSupported(): boolean {
  return (
    typeof CSS !== "undefined" &&
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    "highlights" in (CSS as any) &&
    typeof (window as unknown as { Highlight?: unknown }).Highlight !== "undefined"
  );
}

// findRanges walks the text nodes under root and returns a Range for every
// case-insensitive occurrence of needle.
function findRanges(root: HTMLElement, needle: string): Range[] {
  const ranges: Range[] = [];
  const q = needle.toLowerCase();
  if (!q) return ranges;
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: (node) =>
      node.nodeValue && node.nodeValue.length ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT,
  });
  let node: Node | null;
  while ((node = walker.nextNode())) {
    const text = node.nodeValue!.toLowerCase();
    let from = 0;
    let idx: number;
    while ((idx = text.indexOf(q, from)) !== -1) {
      const r = document.createRange();
      try {
        r.setStart(node, idx);
        r.setEnd(node, idx + q.length);
        ranges.push(r);
      } catch {
        /* node changed under us; skip */
      }
      from = idx + q.length;
    }
  }
  return ranges;
}

function clearHighlights() {
  // Runs from cleanup, where a throw would skip the focus restore after it.
  if (typeof CSS === "undefined") return;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const hs = (CSS as any).highlights;
  if (hs) {
    hs.delete(HL);
    hs.delete(HL_CUR);
  }
}

export function FindBar({ scope, onClose }: { scope: () => HTMLElement | null; onClose: () => void }) {
  const [q, setQ] = useState("");
  const [count, setCount] = useState(0);
  const [cur, setCur] = useState(0); // 0-based index of the active match
  const rangesRef = useRef<Range[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    // Focus must go back where it came from on close (A11Y-3) — same contract
    // as Modal: capture before we steal it, hand it back on unmount, whatever
    // closed us (Esc, ✕, or a session switch). The element can be gone by then.
    const previous = document.activeElement as HTMLElement | null;
    inputRef.current?.focus();
    return () => {
      clearHighlights();
      if (previous?.isConnected) previous.focus();
    };
  }, []);

  // Paint all matches, with the active one on a higher-priority layer, and
  // scroll it into view.
  const paint = useCallback((ranges: Range[], active: number) => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const w = window as any;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const hs = (CSS as any).highlights;
    if (!hs || !w.Highlight) return;
    if (!ranges.length) {
      clearHighlights();
      return;
    }
    const all = new w.Highlight(...ranges);
    all.priority = 0;
    hs.set(HL, all);
    const activeRange = ranges[active];
    if (activeRange) {
      const one = new w.Highlight(activeRange);
      one.priority = 1;
      hs.set(HL_CUR, one);
      const el =
        activeRange.startContainer.parentElement ??
        (activeRange.startContainer as HTMLElement | null);
      try {
        el?.scrollIntoView({ block: "center", behavior: "smooth" });
      } catch {
        /* detached */
      }
    }
  }, []);

  // Recompute matches whenever the query changes.
  useEffect(() => {
    const root = scope();
    if (!highlightSupported() || !root) {
      rangesRef.current = [];
      setCount(0);
      clearHighlights();
      return;
    }
    const ranges = findRanges(root, q);
    rangesRef.current = ranges;
    setCount(ranges.length);
    setCur(0);
    paint(ranges, 0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q]);

  const step = (delta: number) => {
    const ranges = rangesRef.current;
    if (!ranges.length) return;
    const next = (cur + delta + ranges.length) % ranges.length;
    setCur(next);
    paint(ranges, next);
  };

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    } else if (e.key === "Enter") {
      e.preventDefault();
      step(e.shiftKey ? -1 : 1);
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      step(1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      step(-1);
    }
  };

  const unsupported = !highlightSupported();

  return (
    <div className="findbar" onKeyDown={onKey}>
      <span className="fb-ico" aria-hidden="true"><MagnifyingGlass size={14} /></span>
      <input
        ref={inputRef}
        className="fb-input"
        aria-label="Search conversation"
        placeholder="Search chat…"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      <span className="fb-count" title={unsupported ? "Find unsupported" : undefined}>
        {unsupported ? "N/A" : q ? (count ? `${cur + 1} / ${count}` : "0 / 0") : ""}
      </span>
      <div className="fb-nav">
        <IconButton
          size="md"
          variant="ghost"
          aria-label="Previous match"
          title="Previous (⇧Enter)"
          disabled={count === 0}
          onClick={() => step(-1)}
        >
          <ArrowUp size={14} />
        </IconButton>
        <IconButton
          size="md"
          variant="ghost"
          aria-label="Next match"
          title="Next (Enter)"
          disabled={count === 0}
          onClick={() => step(1)}
        >
          <ArrowDown size={14} />
        </IconButton>
      </div>
      <IconButton
        size="md"
        variant="ghost"
        title="Close (Esc)"
        aria-label="Close find"
        onClick={onClose}
      >
        <X size={14} />
      </IconButton>
    </div>
  );
}
