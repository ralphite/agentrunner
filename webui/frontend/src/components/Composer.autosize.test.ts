// @ts-ignore -- no @types/node in the browser production tsconfig
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const controller = readFileSync(
  `${process.cwd()}/src/features/composer/ComposerController.tsx`,
  "utf8",
);
const css = readFileSync(`${process.cwd()}/src/tw.css`, "utf8");

// The composer card grows with the draft. jsdom has no layout — scrollHeight is
// always 0 there — so these pin the two structural halves of that instead, both
// of which had actually broken:
//
//   1. `grow` was called by hand from three handlers. Dictation appends through
//      `appendText`, which was not one of them, so a dictated sentence sat in a
//      one-row box with its second line sliced through the middle. Height is a
//      function of the draft, so it has to be derived from the draft.
//   2. `grow` also carried `Math.min(scrollHeight, 320)` — a ceiling that
//      disagreed with the stylesheet's own (180px below 901px), so the inline
//      height claimed room the box was never given.
describe("composer autosize", () => {
  it("derives the height from the draft, not from whichever handler set it", () => {
    expect(controller).toMatch(
      /useLayoutEffect\(\(\) => \{\s*if \(taRef\.current\) grow\(taRef\.current\);\s*\}, \[text\]\);/,
    );
    // No handler-side calls left to forget: the effect is the only caller.
    expect(controller.match(/\bgrow\(/g)).toHaveLength(1);
  });

  it("leaves the ceiling to the stylesheet, which knows the width", () => {
    const body = controller.slice(
      controller.indexOf("const grow = "),
      controller.indexOf("const grow = ") + 200,
    );
    expect(body).toContain("el.scrollHeight");
    expect(body).not.toMatch(/Math\.min/);
    expect(css).toContain("max-h-[180px]");
    expect(css).toContain("max-height: min(320px, 38dvh);");
  });
});
