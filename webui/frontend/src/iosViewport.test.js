import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const root = new URL("..", import.meta.url);

describe("iOS viewport contract", () => {
  it("keeps keyboard focus app-like without disabling pinch zoom", () => {
    const html = readFileSync(new URL("index.html", root), "utf8");
    const css = readFileSync(new URL("src/tw.css", root), "utf8");

    expect(html).toContain("interactive-widget=resizes-content");
    expect(html).not.toMatch(/(?:maximum-scale|user-scalable)\s*=/);
    expect(css).toContain("@supports (-webkit-touch-callout: none)");
    expect(css).toContain("@media (any-pointer: coarse)");
    expect(css).toMatch(/:is\(input,\s*textarea,\s*select\)\s*\{[\s\S]*font-size:\s*max\(16px,\s*1em\)/);
  });
});
