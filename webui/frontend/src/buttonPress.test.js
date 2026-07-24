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

describe("sidebar session row highlight extent (INC-92)", () => {
  it("uses the same complete wrapper background for current, hover, and focus", () => {
    const highlightRule =
      css.match(/\.project-session-wrap\.current,[^{]+\{[^}]*bg-panel-2[^}]*\}/s)?.[0] || "";
    expect(highlightRule).toContain(".project-session-wrap:hover");
    expect(highlightRule).toContain(".pseudo-hover .project-session-wrap");
    expect(highlightRule).toContain(".project-session-wrap:focus-within");
    const buttonRule = css.match(/\.project-session\s*\{([^}]*)\}/s)?.[1] || "";
    expect(buttonRule).not.toContain("hover:bg-panel-2");
  });
});

describe("sidebar project row highlight extent (INC-93)", () => {
  it("paints the complete heading-and-actions wrapper on hover and focus", () => {
    const highlightRule =
      css.match(/\.project-heading-row:hover,[^{]+\{[^}]*bg-panel-2[^}]*\}/s)?.[0] || "";
    expect(highlightRule).toContain(".pseudo-hover .project-heading-row");
    expect(highlightRule).toContain(".project-heading-row:focus-within");
    const headingRule = css.match(/\.project-heading\s*\{([^}]*)\}/s)?.[1] || "";
    expect(headingRule).not.toContain("hover:bg-panel-2");
  });
});
