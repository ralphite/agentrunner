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
 * @example
 * const bp = useBreakpoint();
 * if (bp.tablet) { ... }  // true if > 900px
 */
export function useBreakpoint() {
  const [bp, setBp] = useState(() => measureBreakpoint());

  useEffect(() => {
    const measure = () => setBp(measureBreakpoint());
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, []);

  return bp;
}

function measureBreakpoint() {
  const w = window.innerWidth;
  return {
    compact: w <= BREAKPOINTS.compact,      // ≤ 680px
    tablet: w > BREAKPOINTS.compact && w <= BREAKPOINTS.tablet,    // 680–900px
    desktop: w > BREAKPOINTS.tablet && w <= BREAKPOINTS.wide,      // 900–1400px
    wide: w > BREAKPOINTS.wide,             // > 1400px
  } as const;
}
