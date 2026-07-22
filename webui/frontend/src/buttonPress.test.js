// @vitest-environment node
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const css = readFileSync(new URL("tw.css", import.meta.url), "utf8");

describe("button pressed-state sizing (INC-89)", () => {
  it("never scales buttons in an active rule", () => {
    expect(css).not.toMatch(/:active\s*\{[^}]*\bscale(?:-|\()/s);
  });
});

describe("sidebar session row highlight extent (INC-92)", () => {
  it("uses the same complete wrapper background for current, hover, and focus", () => {
    expect(css).toMatch(
      /\.project-session-wrap\.current,\s*\.project-session-wrap:hover,\s*\.project-session-wrap:focus-within\s*\{[^}]*bg-panel-2/s,
    );
    const buttonRule = css.match(/\.project-session\s*\{([^}]*)\}/s)?.[1] || "";
    expect(buttonRule).not.toContain("hover:bg-panel-2");
  });
});
