// @vitest-environment node
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const css = readFileSync(new URL("tw.css", import.meta.url), "utf8");

describe("button pressed-state sizing (INC-89)", () => {
  it("never scales buttons in an active rule", () => {
    expect(css).not.toMatch(/:active\s*\{[^}]*\bscale(?:-|\()/s);
  });
});
