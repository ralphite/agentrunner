// @vitest-environment jsdom
import { describe, expect, it } from "vitest";

// @ts-ignore -- no @types/node in this project's tsconfig
import { readFileSync } from "node:fs";

// @ts-ignore -- `process` is a vitest-only reference (vitest runs from webui/frontend)
const src: string = readFileSync(`${process.cwd()}/src/components/Home.tsx`, "utf8");
const partsSrc: string = readFileSync(`${process.cwd()}/src/components/HomeParts.tsx`, "utf8");

describe("home greeting structure", () => {
  it("keeps the compact empty-state chain that Tailwind styles", () => {
    expect(src).toContain('<div className="home home-welcome home-empty-state">');
    expect(src).toContain('<div className="hero max-[680px]:[@media(max-height:560px)]:py-2">');
    expect(src).toContain('<div className="home-empty">');
    expect(src).toContain('<h2 className="home-empty-headline">');
  });

  it("keeps suggestion cards and the project-aware repository span", () => {
    expect(src).toContain('className="home-empty-cards max-[680px]:gap-1.5"');
    expect(partsSrc).toContain(
      'className="home-empty-card max-[680px]:min-h-[76px] max-[680px]:gap-1 max-[680px]:px-2.5 max-[680px]:py-2"',
    );
    expect(src).toContain('home-empty-repo');
    expect(src).toContain('decoration-dotted');
  });

  it("keeps the send action on-screen after a starter fills the mobile composer", () => {
    expect(src).toContain("max-[480px]:[&_.cx-optimize]:hidden");
    expect(src).toContain('variant="home"');
  });

  it("uses Codex's two-stage starter intent instead of keeping cards beside a long canned prompt", () => {
    expect(src).toContain('seed: "Explore"');
    expect(src).toContain('"Explore and learn how a feature works"');
    expect(partsSrc).toContain('className="home-intent-suggestion"');
    expect(src).toContain("const showStarterCards = !draft.trim() && !activeSuggestion");
    expect(src).toContain("onDraftChange={handleDraftChange}");
  });

  it("fits the empty composer inside a 390x500 viewport without changing normal-height mobile", () => {
    expect(src).toContain("max-[680px]:[@media(max-height:560px)]:[&_.cx-input-wrap]:pt-1.5");
    expect(src).toContain("max-[680px]:[@media(max-height:560px)]:[&_.cx-input-wrap_textarea]:min-h-8");
  });
});
