// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";

// HM-8 (and HM-4 before it) — the home headline's size is a CASCADE fact, not a
// declaration fact: `styles.css` carries two `.hero h2` rules (0,1,1) of its own,
// and the home sheet's headline rule only paints if it out-specifies them. HM-4
// shipped a headline block that lost that fight and sat dead for rounds — nobody
// noticed, because a test that merely greps the home sheet for `font-size: 23px`
// passes just as happily on dead code.
//
// So this file loads BOTH sheets into jsdom in the app's import order, rebuilds
// the element chain Home renders, and reads the *computed* value off the real
// cascade. If someone adds a heavier `.hero h2` rule to styles.css, this goes red.
//
// @ts-ignore -- no @types/node in this project's tsconfig
import { readFileSync } from "node:fs";

// @ts-ignore -- `process` is a vitest-only reference (vitest runs from webui/frontend)
const cwd: string = process.cwd();
const base: string = readFileSync(`${cwd}/src/styles.css`, "utf8");
const home: string = readFileSync(`${cwd}/src/styles.home.css`, "utf8");
const src: string = readFileSync(`${cwd}/src/components/Home.tsx`, "utf8");

afterEach(() => {
  document.head.innerHTML = "";
  document.body.innerHTML = "";
});

// The fixture below hard-codes the class chain Home renders; if Home is
// restructured, the cascade assertions would silently start testing a DOM the app
// no longer ships. Pin the chain to the component source.
function headline(): HTMLElement {
  for (const css of [base, home]) {
    const style = document.createElement("style");
    style.textContent = css;
    document.head.appendChild(style);
  }
  document.body.innerHTML = `
    <div class="home home-welcome home-empty-state">
      <div class="hero">
        <div class="home-empty">
          <h2 class="home-empty-headline">What should we build?</h2>
        </div>
      </div>
    </div>`;
  return document.querySelector<HTMLElement>(".home-empty-headline")!;
}

// A `@media` block's body, so the phone sizes can be pinned too — jsdom's
// getComputedStyle ignores media queries entirely, so the ≤520 override is the
// one value that has to be read out of the sheet text.
function mediaBlock(query: string): string {
  const at = home.indexOf(query);
  expect(at, `${query} not found in styles.home.css`).toBeGreaterThan(-1);
  return home.slice(at, home.indexOf("\n}", at));
}

describe("HM-8 — the home greeting is a line of text, not a title wall", () => {
  it("paints the headline at Codex's 23px/400 through the real cascade", () => {
    const cs = getComputedStyle(headline());
    // Golden (qa/codex-reference/codex-new-task-home.jpg): ~10.0 logical px per
    // character at weight 400. We were painting 30px/500 — a third too wide, and
    // heavier than everything else on a page whose job is to be typed into.
    expect(cs.fontSize).toBe("23px");
    expect(cs.fontWeight).toBe("400");
    expect(cs.letterSpacing).toBe("-0.2px");
  });

  it("keeps winning over the two `.hero h2` rules in styles.css", () => {
    // Both of styles.css's headline rules are still there (this test is only
    // meaningful while they are) and both are out-specified, not deleted.
    expect(base).toMatch(/\.hero h2 \{/);
    expect(home).toMatch(/\.home-empty \.home-empty-headline \{/);
    // A bare `.home-empty-headline` (0,1,0) would tie-and-lose to `.hero h2`
    // (0,1,1) — that was HM-4's dead code. Never again.
    expect(home).not.toMatch(/^\.home-empty-headline \{/m);
  });

  it("scales down, not up, on a 390px phone", () => {
    // styles.css drops `.hero h2` to 24px at its own breakpoint; ours must land
    // under that, not over it (it used to be 25px — bigger than the fallback).
    expect(mediaBlock("@media (max-width: 520px)")).toMatch(
      /\.home-empty \.home-empty-headline \{ font-size: 20px;/,
    );
  });

  it("shrinks the headline without touching the suggestion cards (HM-7 / QA-45)", () => {
    // The point of HM-8 is the RATIO: the cards already sit at the golden's
    // density and the composer is QA-45's deliberate roomy bottom-pinned input.
    // Only the headline gives ground.
    expect(home).toMatch(/\.home-empty-card \{[^}]*min-height: 84px;/);
    expect(home).toMatch(/\.home-empty-cards \{[^}]*max-width: 588px;/);
  });

  it("renders the chain the cascade above assumes", () => {
    expect(src).toContain('<div className="home-empty">');
    expect(src).toContain('<h2 className="home-empty-headline">');
    expect(src).toContain('<div className="hero">');
  });
});
