import { describe, expect, it } from "vitest";
import { diffPath, sessionImageURL, uploadURL } from "./api";

describe("diffPath", () => {
  it("keeps Working tree and Last turn as explicit backend scopes", () => {
    expect(diffPath("session-1", "working-tree")).toBe("/sessions/session-1/diff?scope=working-tree");
    expect(diffPath("session-1", "last-turn")).toBe("/sessions/session-1/diff?scope=last-turn");
  });
});

describe("RT-6 — image URLs", () => {
  it("addresses a user attachment by its durable session blob", () => {
    const ref = "sha256-" + "b".repeat(64);
    expect(sessionImageURL("s-1", ref)).toBe(`/api/sessions/s-1/image/${ref}`);
  });

  it("passes an already-served image URL through uploadURL untouched", () => {
    // the Lightbox maps every image through uploadURL; a session blob URL must
    // survive that trip, or opening a restored thumbnail would 404.
    const url = sessionImageURL("s-1", "sha256-" + "c".repeat(64));
    expect(uploadURL(url)).toBe(url);
  });

  it("still maps a local upload path to the uploads route", () => {
    expect(uploadURL("/tmp/runtime/uploads/171-shot.png")).toBe("/api/uploads/171-shot.png");
  });
});
