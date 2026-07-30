// @vitest-environment node
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const css = readFileSync(new URL("tw.css", import.meta.url), "utf8");

describe("button pressed-state sizing (INC-89)", () => {
  it("never scales buttons in an active rule", () => {
    expect(css).not.toMatch(/:active\s*\{[^}]*\bscale(?:-|\()/s);
  });

  it("keeps every shared button at least 44px on coarse pointers", () => {
    expect(css).toMatch(
      /@media \(any-pointer: coarse\)[\s\S]*?\[data-ui-button\]\s*\{[^}]*min-height:\s*44px[^}]*\}[\s\S]*?\[data-ui-icon-button\]\s*\{[^}]*min-width:\s*44px/s,
    );
  });
});

// INC-92/93 protect WHICH ELEMENT lights up — the full wrapper including the
// trailing action icons, never just the inner label button. These assertions
// deliberately do not pin HOW it lights up: the paint moved from a bg-panel-2
// fill to the shared translucent overlay (CX-IX) so that selected+hovered is a
// visible step darker than selected, and pinning the declaration would have
// blocked that without any behaviour actually regressing.
const paints = (rule) => /box-shadow:[^;]*--ix-hover|bg-panel-2|bg-sel/.test(rule);

describe("menu surfaces stay opaque (CX-OVERLAY)", () => {
  it("never leans on backdrop-filter to make a menu readable", () => {
    // Shipped once and reverted: a frosted menu degrades to a see-through one
    // on every path where the blur does not render. Here the minifier collapsed
    // the hand-written `backdrop-filter` / `-webkit-backdrop-filter` pair down
    // to the prefixed form alone, which the browser then ignored — leaving a
    // 70%-opaque panel with the sidebar list legible straight through it.
    expect(css).not.toMatch(/backdrop-filter\s*:/);
  });

  it("paints attached menus on a solid surface", () => {
    const rule = css.match(/\.pop-panel,\s*\.menu-pop,\s*\.ctx-menu\s*\{[^}]*\}/s)?.[0] || "";
    expect(rule).toMatch(/background:\s*var\(--panel\)/);
  });
});

describe("sidebar session row highlight extent (INC-92)", () => {
  it("paints the complete wrapper for current, hover, and focus", () => {
    const hoverRule =
      css.match(/\.project-session-wrap:hover,[^{]+\{[^}]*\}/s)?.[0] || "";
    expect(hoverRule).toContain(".pseudo-hover .project-session-wrap");
    expect(hoverRule).toContain(".project-session-wrap:focus-within");
    expect(paints(hoverRule)).toBe(true);

    const currentRule =
      css.match(/\.project-session-wrap\.current\s*\{[^}]*\}/s)?.[0] || "";
    expect(paints(currentRule)).toBe(true);

    // The inner button must stay unpainted, or the highlight stops short of
    // the row's trailing icons.
    const buttonRule = css.match(/\.project-session\s*\{([^}]*)\}/s)?.[1] || "";
    expect(paints(buttonRule)).toBe(false);
  });
});

describe("sidebar project row highlight extent (INC-93)", () => {
  it("paints the complete heading-and-actions wrapper on hover and focus", () => {
    const highlightRule =
      css.match(/\.project-heading-row:hover,[^{]+\{[^}]*\}/s)?.[0] || "";
    expect(highlightRule).toContain(".pseudo-hover .project-heading-row");
    expect(highlightRule).toContain(".project-heading-row:focus-within");
    expect(paints(highlightRule)).toBe(true);
    const headingRule = css.match(/\.project-heading\s*\{([^}]*)\}/s)?.[1] || "";
    expect(paints(headingRule)).toBe(false);
  });

  it("lets any focused child paint the full wrapper outline, not only the main button", () => {
    const outlined = (selector) =>
      new RegExp(
        `\\${selector}:has\\(:focus-visible\\)\\s*\\{[^}]*outline:[^}]*var\\(--blue\\)[^}]*\\}`,
        "s",
      ).test(css);
    expect(outlined(".project-heading-row")).toBe(true);
    expect(outlined(".project-session-wrap")).toBe(true);
  });
});

// Same bug class as INC-92/93, one level up the rail: the "Pinned"/"Projects"
// section header. Its fill used to live on the inner .section-toggle, which is
// flex-1 — so revealing the ⋯/+ strip on hover shrank the button and the fill
// stopped ~58px short of the 224px row, reading as a chip instead of the
// section it heads. It also used opaque --panel-2 where every row below uses
// the translucent --ix-hover.
describe("sidebar section header highlight extent", () => {
  it("paints the wrapper, not the inner toggle, with the same token as the rows", () => {
    const highlightRule =
      css.match(/\.section-heading-row:hover,[^{]+\{[^}]*\}/s)?.[0] || "";
    expect(highlightRule).toContain(".pseudo-hover .section-heading-row");
    expect(highlightRule).toContain(".section-heading-row:focus-within");
    expect(highlightRule).toMatch(/box-shadow:[^;]*--ix-hover/);

    const toggleRule = css.match(/\.section-toggle\s*\{([^}]*)\}/s)?.[1] || "";
    expect(paints(toggleRule)).toBe(false);
  });

  it("keeps the header radius on the wrapper that paints it", () => {
    const rowRule = css.match(/\.section-heading-row\s*\{([^}]*)\}/s)?.[1] || "";
    expect(rowRule).toMatch(/rounded-\[var\(--radius-row\)\]/);
  });
});

// The rail reads as one icon column: the section header's caret sits in the
// same fixed 16px slot as the project folders, with the same gap to the label,
// so heading text and project names share a left edge (both 40px in the live
// 224px rail). A bare 11px caret with gap-1 put the heading text at 31px.
describe("sidebar icon column", () => {
  it("gives the section caret the same slot geometry as the project folder", () => {
    const slotRule =
      css.match(/\.proj-icon-slot,\s*\n?\s*\.section-icon-slot\s*\{([^}]*)\}/s)?.[1] || "";
    expect(slotRule).toMatch(/h-4/);
    expect(slotRule).toMatch(/w-4/);
    expect(slotRule).toMatch(/place-items-center/);
  });

  it("uses one gap for the section toggle and the project heading", () => {
    const gapOf = (selector) => {
      const rule =
        css.match(new RegExp(`\\${selector}\\s*\\{([^}]*)\\}`, "s"))?.[1] || "";
      return rule.match(/\bgap-(\S+?)\b/)?.[1] || null;
    };
    // .project-heading inherits gap-2 from the shared .section-label base rule.
    expect(gapOf(".section-toggle")).toBe("2");
    expect(gapOf(".section-label, .project-heading")).toBe("2");
  });
});
