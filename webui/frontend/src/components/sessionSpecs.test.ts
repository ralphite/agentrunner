// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";

beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  vi.resetModules();
});

describe("device-wide last model choice", () => {
  it("survives a reload so the next new session opens on it", async () => {
    const first = await import("./sessionSpecs");
    first.rememberLastModel({
      provider: "gemini",
      model: "gemini-pro-latest",
      effort: "high",
    });

    vi.resetModules();
    const reloaded = await import("./sessionSpecs");

    expect(reloaded.recallLastModel()).toEqual({
      provider: "gemini",
      model: "gemini-pro-latest",
      effort: "high",
    });
  });

  it("stays separate from the per-session record, which keeps its own choice", async () => {
    const state = await import("./sessionSpecs");
    state.rememberModel("session-a", {
      provider: "anthropic",
      model: "claude-sonnet-5",
      effort: "medium",
    });
    state.rememberLastModel({
      provider: "gemini",
      model: "gemini-pro-latest",
      effort: "high",
    });

    expect(state.recallModel("session-a")?.model).toBe("claude-sonnet-5");
    expect(state.recallLastModel()?.model).toBe("gemini-pro-latest");
  });

  it("ignores a partial record instead of seeding a half-set choice", async () => {
    const state = await import("./sessionSpecs");
    state.rememberLastModel({ provider: "gemini", model: "", effort: "high" });

    expect(state.recallLastModel()).toBeUndefined();
  });
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
