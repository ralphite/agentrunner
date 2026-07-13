// @vitest-environment jsdom
import { describe, expect, it } from "vitest";

// @ts-ignore -- no @types/node in this project's tsconfig
import { readFileSync } from "node:fs";

// @ts-ignore -- `process` is a vitest-only reference (vitest runs from webui/frontend)
const src: string = readFileSync(`${process.cwd()}/src/components/Home.tsx`, "utf8");

describe("home greeting structure", () => {
  it("keeps the compact empty-state chain that Tailwind styles", () => {
    expect(src).toContain('<div className="home home-welcome home-empty-state">');
    expect(src).toContain('<div className="hero">');
    expect(src).toContain('<div className="home-empty">');
    expect(src).toContain('<h2 className="home-empty-headline">');
  });

  it("keeps suggestion cards and the project-aware repository span", () => {
    expect(src).toContain('className="home-empty-cards"');
    expect(src).toContain('className="home-empty-card"');
    expect(src).toContain('className="home-empty-repo"');
  });

  it("keeps the send action on-screen after a starter fills the mobile composer", () => {
    expect(src).toContain('className="home-composer w-full max-[480px]:[&_.cx-optimize]:hidden"');
    expect(src).toContain('<Composer variant="home"');
  });
});
