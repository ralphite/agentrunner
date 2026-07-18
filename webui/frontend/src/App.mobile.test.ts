import { describe, expect, it } from "vitest";

// @ts-ignore -- no @types/node in the browser production tsconfig
import { readFileSync } from "node:fs";

// @ts-ignore -- vitest runs this contract from webui/frontend
const src = readFileSync(`${process.cwd()}/src/App.tsx`, "utf8");

describe("mobile navigation breakpoint", () => {
  it("uses the same 900px boundary as the sidebar drawer CSS", async () => {
    // The boundary moved behind the centralized breakpoint scale (Problem 5
    // migration): mobile nav = compact|tablet tiers, whose upper edge is
    // BREAKPOINTS.tablet — pin that it is still the drawer CSS's 900px.
    const { BREAKPOINTS } = await import("./hooks/useBreakpoint");
    expect(BREAKPOINTS.tablet).toBe(900);
    expect(src).toContain("useBreakpoint");
    expect(src).toContain("bp.compact || bp.tablet");
    expect(src).not.toContain("max-width: 680px");
  });
});
