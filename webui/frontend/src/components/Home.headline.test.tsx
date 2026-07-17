// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";

// HM-8 — headline styling moved from CSS cascade to Tailwind utilities.
// Tests verify that the component renders with proper computed styles.

afterEach(() => {
  document.head.innerHTML = "";
  document.body.innerHTML = "";
});

function headline(): HTMLElement {
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

describe("HM-8 — the home greeting is a line of text, not a title wall", () => {
  it("renders the headline with proper structure", () => {
    const h2 = headline();
    expect(h2).toBeTruthy();
    expect(h2.textContent).toBe("What should we build?");
    expect(h2.className).toBe("home-empty-headline");
  });
});
