import { describe, expect, it } from "vitest";
import { normalizeRoute } from "./routeHash";

describe("normalizeRoute", () => {
  it("maps the path-ish home links to the home route", () => {
    expect(normalizeRoute("/")).toBe("");
    expect(normalizeRoute("")).toBe("");
  });

  it("maps '/scheduled' to the scheduled page", () => {
    expect(normalizeRoute("/scheduled")).toBe("scheduled");
    expect(normalizeRoute("/scheduled/")).toBe("scheduled");
    expect(normalizeRoute("scheduled")).toBe("scheduled");
  });

  it("keeps resolving the '/s/<sid>' deep-link form", () => {
    expect(normalizeRoute("/s/20260710-abc")).toBe("20260710-abc");
    expect(normalizeRoute("s/20260710-abc")).toBe("20260710-abc");
    expect(normalizeRoute("20260710-abc")).toBe("20260710-abc");
  });

  it("leaves run ids untouched", () => {
    expect(normalizeRoute("run:r-42")).toBe("run:r-42");
    expect(normalizeRoute("/run:r-42")).toBe("run:r-42");
  });
});
