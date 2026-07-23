import { describe, expect, it } from "vitest";

// @ts-ignore -- no @types/node in the browser production tsconfig
import { readFileSync } from "node:fs";

// @ts-ignore -- vitest runs this contract from webui/frontend
const src = readFileSync(`${process.cwd()}/src/App.tsx`, "utf8");
// @ts-ignore -- vitest runs this contract from webui/frontend
const css = readFileSync(`${process.cwd()}/src/tw.css`, "utf8");

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

  it("removes the underlying sidebar trigger while the mobile Changes overlay owns the surface", () => {
    expect(css).toContain(
      ".main:has(.session-layout.changes) > .sidebar-show { display: none; }",
    );
  });
});

describe("Changes split layout", () => {
  it("sizes the desktop review rail from the content track, not the whole viewport", () => {
    expect(css).toContain(
      "grid-template-columns: minmax(0, 1fr) minmax(320px, clamp(320px, calc(100% - 390px), 54%));",
    );
    expect(css).not.toContain("minmax(320px, 46vw)");
  });

  it("compacts the conversation by its actual split-track width", () => {
    expect(css).toContain("container-name: session-primary;");
    expect(css).toContain("@container session-primary (max-width: 500px)");
    expect(css).toContain(".cx-session .cx-mode-label { @apply hidden; }");
    expect(css).toContain(".cx-session .cx-model { @apply max-w-[116px]");
    expect(css).toContain(".tl-inner { @apply px-[14px]; }");
  });
});

describe("Environment layout contract", () => {
  it("keeps Environment on one track and implements the rail as one shared floating card rule", () => {
    expect(css).toContain(
      ".session-layout.environment { grid-template-columns: minmax(0, 1fr); }",
    );
    expect(css).not.toContain(
      ".session-layout.environment { grid-template-columns: minmax(0, 1fr) minmax(300px, 360px); }",
    );

    const selector = ".session-view .supervision-panel.session-side {";
    const ruleStart = css.indexOf(selector);
    const mobileStart = css.indexOf("@media (max-width: 900px)");
    expect(ruleStart).toBeGreaterThan(-1);
    expect(ruleStart).toBeLessThan(mobileStart);
    expect(css.indexOf(selector, ruleStart + selector.length)).toBe(-1);

    const ruleEnd = css.indexOf("}", ruleStart);
    const rule = css.slice(ruleStart, ruleEnd);
    expect(rule).toContain("@apply absolute right-3 top-[60px] z-[25] block");
    expect(rule).toContain("overflow-y-auto");
    expect(rule).toContain("width: min(340px, calc(100% - 24px));");
    expect(rule).toContain("max-height: calc(100% - 72px);");
  });
});

describe("Settings focus return", () => {
  it("returns a sidebar-menu launch to the persistent More options trigger", () => {
    expect(src).toContain("active.closest('[role=\"menu\"]')");
    expect(src).toContain("button[aria-label=\"More options\"]");
  });
});
