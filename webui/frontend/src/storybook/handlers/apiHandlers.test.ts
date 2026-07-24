// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";
import {
  buildEnvelope,
  buildInspect,
  buildProjectMeta,
  fixtureDefaults,
} from "../fixtures";
import { createStoryApiHandlers } from "./apiHandlers";

const activeServers: ReturnType<typeof setupServer>[] = [];

afterEach(() => {
  for (const server of activeServers.splice(0)) server.close();
});

function startHarness(harness: ReturnType<typeof createStoryApiHandlers>) {
  const server = setupServer(...harness.handlers);
  server.listen({ onUnhandledRequest: "error" });
  activeServers.push(server);
  return server;
}

const apiURL = (path: string) => new URL(path, window.location.href).href;

describe("Storybook API handlers", () => {
  it("keeps mutable request state private to each factory instance", async () => {
    const first = createStoryApiHandlers();
    startHarness(first);

    const update = await fetch(apiURL("/api/projects"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        workspace: fixtureDefaults.workspace,
        displayName: "Changed in first harness",
      }),
    });
    expect(update.ok).toBe(true);
    expect(first.snapshot().projects[fixtureDefaults.workspace].displayName)
      .toBe("Changed in first harness");

    activeServers.pop()!.close();

    const second = createStoryApiHandlers({
      projects: {
        [fixtureDefaults.workspace]: buildProjectMeta({
          displayName: "Second harness",
        }),
      },
    });
    startHarness(second);

    const response = await fetch(apiURL("/api/projects"));
    expect(await response.json()).toMatchObject({
      [fixtureDefaults.workspace]: { displayName: "Second harness" },
    });
    expect(first.snapshot().projects[fixtureDefaults.workspace].displayName)
      .toBe("Changed in first harness");
  });

  it("supports the runtime and session page's common GET/POST shapes", async () => {
    const harness = createStoryApiHandlers();
    startHarness(harness);
    const sid = harness.snapshot().sessions[0].id;

    const urls = [
      "/api/health",
      "/api/agents",
      "/api/sessions?limit=40&offset=0",
      "/api/runs",
      "/api/projects",
      `/api/sessions/${sid}/events?after=0`,
      `/api/sessions/${sid}/inspect`,
      `/api/sessions/${sid}/ps`,
      `/api/sessions/${sid}/queue`,
      `/api/sessions/${sid}/diff?scope=working-tree`,
    ];
    for (const url of urls) {
      const response = await fetch(apiURL(url));
      expect(response.ok, url).toBe(true);
    }

    const send = await fetch(apiURL(`/api/sessions/${sid}/send`), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: "A fixture-only follow-up", delivery: "queue" }),
    });
    expect(send.ok).toBe(true);
    const events = harness.snapshot().events[sid];
    expect(events[events.length - 1]).toMatchObject({
      type: "input_received",
      payload: {
        text: "A fixture-only follow-up",
      },
    });
  });

  it("restores a clean cloned seed on reset", async () => {
    const harness = createStoryApiHandlers();
    startHarness(harness);
    const sid = harness.snapshot().sessions[0].id;

    await fetch(apiURL(`/api/sessions/${sid}/rename`), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: "Changed title" }),
    });
    expect(harness.snapshot().sessions[0].title).toBe("Changed title");

    harness.reset();

    expect(harness.snapshot().sessions[0].title)
      .toBe("Prepare the component demo");
  });

  it("seeds a new session with its durable opening user message", async () => {
    const harness = createStoryApiHandlers({ sessions: [], runs: [] });
    startHarness(harness);

    const response = await fetch(apiURL("/api/sessions"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        workspace: fixtureDefaults.workspace,
        message: "Build the Core Session demo",
      }),
    });
    expect(response.ok).toBe(true);

    const { sid } = await response.json() as { sid: string };
    expect(harness.snapshot().events[sid]).toEqual([
      expect.objectContaining({
        seq: 1,
        type: "input_received",
        payload: expect.objectContaining({
          source: "user",
          text: "Build the Core Session demo",
        }),
      }),
    ]);
  });

  it("exposes deterministic scenario mutation without leaking live state", () => {
    const harness = createStoryApiHandlers();
    const sid = harness.snapshot().sessions[0].id;
    const event = buildEnvelope({
      seq: 99,
      type: "assistant_message",
      payload: { message: { parts: [{ text: "Scenario completed." }] } },
    });

    harness.appendEvents(sid, [event]);
    harness.updateSession(sid, { status: "completed", turns: 3 });
    harness.setInspect(sid, buildInspect({ mode: "completed" }));

    const first = harness.snapshot();
    expect(first.events[sid][first.events[sid].length - 1]).toEqual(event);
    expect(first.sessions[0]).toMatchObject({ status: "completed", turns: 3 });
    expect(first.inspect[sid]).toMatchObject({ mode: "completed" });

    first.events[sid].push(buildEnvelope({ seq: 100 }));
    expect(harness.snapshot().events[sid]).toHaveLength(first.events[sid].length - 1);
    expect(() => harness.appendEvents("missing", [event]))
      .toThrow(/unknown session/);
  });

  it("leaves unknown API paths for preview's strict policy", async () => {
    const harness = createStoryApiHandlers();
    const fallback = http.all("/api/*", () =>
      HttpResponse.json({ caughtBy: "strict-fallback" }, { status: 418 }));
    const server = setupServer(...harness.handlers, fallback);
    server.listen({ onUnhandledRequest: "error" });
    activeServers.push(server);

    const response = await fetch(apiURL(
      `/api/sessions/${harness.snapshot().sessions[0].id}/not-a-real-action`,
    ), { method: "POST" });

    expect(response.status).toBe(418);
    expect(await response.json()).toEqual({ caughtBy: "strict-fallback" });
  });
});
