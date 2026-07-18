import { useEffect, useState } from "react";

/**
 * Unified breakpoint scale for the application (Problem 5 fix).
 * Replaces fragmented 640/680/900/1100/1400px thresholds with one source of truth.
 */
export const BREAKPOINTS = {
  compact: 680,      // Mobile: max-width for 44px touch targets, single-column layout
  tablet: 900,       // Tablet: drawer→dock transition, split view enablement
  desktop: 1100,     // Desktop: supervision panel defaults to open
  wide: 1400,        // Wide: diff chip layout compaction
} as const;

/**
 * useBreakpoint returns which breakpoint tier the current viewport matches.
 * Replaces ad-hoc matchMedia/ResizeObserver calls across SessionView/DiffView.
 *
 * Measurement goes through matchMedia (the SAME source CSS media queries use,
 * and the seam existing tests mock with per-query granularity — reading
 * window.innerWidth here broke three DiffView specs whose viewport is a
 * matchMedia stub, jsdom's innerWidth being a constant 1024). innerWidth is
 * only the fallback where matchMedia is absent.
 *
 * @example
 * const bp = useBreakpoint();
 * if (bp.tablet) { ... }  // true if 680–900px
 */
export function useBreakpoint() {
  const [bp, setBp] = useState(() => measureBreakpoint());

  useEffect(() => {
    const measure = () => setBp(measureBreakpoint());
    window.addEventListener("resize", measure);
    // Real matchMedia also notifies on zoom/rotation without a resize event.
    const queries: MediaQueryList[] = [];
    if (typeof window.matchMedia === "function") {
      for (const px of Object.values(BREAKPOINTS)) {
        const mq = window.matchMedia(`(max-width: ${px}px)`);
        if (mq.addEventListener) {
          mq.addEventListener("change", measure);
          queries.push(mq);
        }
      }
    }
    return () => {
      window.removeEventListener("resize", measure);
      for (const mq of queries) {
        mq.removeEventListener("change", measure);
      }
    };
  }, []);

  return bp;
}

function measureBreakpoint() {
  let atMost: (px: number) => boolean;
  if (typeof window.matchMedia === "function") {
    atMost = (px) => window.matchMedia(`(max-width: ${px}px)`).matches;
  } else {
    const w = window.innerWidth;
    atMost = (px) => w <= px;
  }
  const compact = atMost(BREAKPOINTS.compact); //        ≤ 680px
  const tablet = !compact && atMost(BREAKPOINTS.tablet); // 680–900px
  const desktop = !compact && !tablet && atMost(BREAKPOINTS.wide); // 900–1400px
  return { compact, tablet, desktop, wide: !compact && !tablet && !desktop } as const;
}
