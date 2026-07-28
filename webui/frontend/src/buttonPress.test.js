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
