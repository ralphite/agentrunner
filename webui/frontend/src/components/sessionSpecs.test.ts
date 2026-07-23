// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";

beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  vi.resetModules();
});

describe("per-tab composer text drafts (INC-98.4l)", () => {
  it("restores a session draft after the module reloads", async () => {
    const first = await import("./sessionSpecs");
    first.rememberDraft("session-a", "第一行\nsecond line");

    vi.resetModules();
    const reloaded = await import("./sessionSpecs");

    expect(reloaded.recallDraft("session-a")).toBe("第一行\nsecond line");
  });

  it("removes a cleared draft so a later reload cannot resurrect it", async () => {
    const first = await import("./sessionSpecs");
    first.rememberDraft("session-a", "do not resurrect");
    first.rememberDraft("session-a", "");

    vi.resetModules();
    const reloaded = await import("./sessionSpecs");

    expect(reloaded.recallDraft("session-a")).toBe("");
  });

  it("keeps session and Home drafts isolated without writing cross-tab localStorage", async () => {
    const state = await import("./sessionSpecs");
    state.rememberDraft("session-a", "session draft");
    state.rememberDraft("~home", "home draft");

    expect(state.recallDraft("session-a")).toBe("session draft");
    expect(state.recallDraft("~home")).toBe("home draft");
    expect(localStorage.length).toBe(0);
  });
});
