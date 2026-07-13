import { describe, expect, it } from "vitest";

// @ts-ignore -- no @types/node in the browser production tsconfig
import { readFileSync } from "node:fs";

// @ts-ignore -- vitest runs this contract from webui/frontend
const src = readFileSync(`${process.cwd()}/src/App.tsx`, "utf8");

describe("mobile navigation breakpoint", () => {
  it("uses the same 900px boundary as the sidebar drawer CSS", () => {
    expect(src).toContain('const MOBILE_NAV_QUERY = "(max-width: 900px)"');
    expect(src.match(/matchMedia\(MOBILE_NAV_QUERY\)/g)).toHaveLength(4);
    expect(src).not.toContain("max-width: 680px");
  });
});
