import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// S68 · DARK-TOKEN-PARITY guard. The stylesheet keeps two hand-mirrored dark
// palettes — the system path (`@media (prefers-color-scheme: dark)` around
// `:root:not([data-theme])`) and the explicit path (`:root[data-theme="dark"]`).
// Nothing but this test keeps them in sync: the S-series sidebar tokens once
// landed only in the media block, and every explicit-dark user got light-theme
// ink on a near-black sidebar (2026-07-29 live screenshot). A token declared in
// either dark block must exist in both.

const css = readFileSync(join(__dirname, "tw.css"), "utf8");

// Extract the body of the first block opened by `marker`, brace-balanced.
function blockAfter(marker: string): string {
  const at = css.indexOf(marker);
  expect(at, `marker not found: ${marker}`).toBeGreaterThanOrEqual(0);
  const open = css.indexOf("{", at);
  let depth = 1;
  let i = open + 1;
  while (depth > 0 && i < css.length) {
    if (css[i] === "{") depth += 1;
    else if (css[i] === "}") depth -= 1;
    i += 1;
  }
  return css.slice(open + 1, i - 1);
}

const declaredTokens = (block: string): Set<string> =>
  new Set([...block.matchAll(/(--[a-z][a-z0-9-]*)\s*:/g)].map((m) => m[1]));

describe("dark token parity (S68)", () => {
  const media = blockAfter("@media (prefers-color-scheme: dark)");
  const attr = blockAfter(':root[data-theme="dark"]');
  const a = declaredTokens(media);
  const b = declaredTokens(attr);

  it("every media-dark token also exists in the explicit-dark block", () => {
    const missing = [...a].filter((t) => !b.has(t)).sort();
    expect(missing, `missing from :root[data-theme="dark"]: ${missing.join(", ")}`).toEqual([]);
  });

  it("every explicit-dark token also exists in the media-dark block", () => {
    const missing = [...b].filter((t) => !a.has(t)).sort();
    expect(missing, `missing from the @media dark block: ${missing.join(", ")}`).toEqual([]);
  });

  it("both dark blocks are non-trivial (the parser found the real palettes)", () => {
    expect(a.size).toBeGreaterThan(30);
    expect(b.size).toBeGreaterThan(30);
  });
});
