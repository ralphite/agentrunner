import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const css = readFileSync(new URL("tw.css", import.meta.url), "utf8");

describe("Scheduled detail responsive geometry (G56)", () => {
  it("keeps list context in a bounded desktop master/detail split", () => {
    const shell = css.match(/\.scheduled-shell\s*\{[^}]*\}/)?.[0];
    const master = css.match(/\.scheduled-shell\.has-detail > \.scheduled-page\s*\{[^}]*\}/)?.[0];
    const detail = css.match(/\.schedule-detail\s*\{[^}]*\}/)?.[0];
    expect(shell).toContain("container-name: scheduled-shell");
    expect(shell).toContain("overflow-hidden");
    expect(master).toContain("w-[44%]");
    expect(master).toContain("min-w-[340px]");
    expect(detail).toContain("min-w-0");
    expect(detail).toContain("flex-1");
  });

  it("turns detail into the whole destination below 760px", () => {
    const narrow = css.match(/@container scheduled-shell \(max-width: 760px\) \{[\s\S]*?\n  \}/)?.[0];
    expect(narrow).toContain(".scheduled-shell.has-detail > .scheduled-page { @apply hidden; }");
    expect(narrow).toContain(".schedule-detail { @apply border-l-0; }");
    expect(narrow).toContain(".schedule-detail-back span { @apply inline; }");
    expect(narrow).toContain(".schedule-detail-close { @apply hidden; }");
  });

  it("gives long prompt/config content its own vertical scroll surface", () => {
    const scroll = css.match(/\.schedule-detail-scroll\s*\{[^}]*\}/)?.[0];
    expect(scroll).toContain("overflow-y-auto");
    expect(scroll).toContain("min-h-0");
  });
});
