// @vitest-environment jsdom

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  inspectStoryFrameDocument,
  QaJourneyPlayback,
  qaJourneyPlaylists,
} from "./QaJourneyPlaylists.stories";

afterEach(cleanup);

describe("QA journey playlists", () => {
  it("keeps every visual QA cluster explicit and non-empty", () => {
    expect(Object.keys(qaJourneyPlaylists)).toEqual([
      "session-delivery",
      "attention-permissions",
      "supervision",
      "scheduled-work",
      "changes-artifacts",
      "navigation-recovery",
    ]);
    for (const playlist of Object.values(qaJourneyPlaylists)) {
      expect(playlist.scenes.length).toBeGreaterThanOrEqual(6);
      for (const scene of playlist.scenes) {
        expect(scene.storyId).toMatch(/^[a-z0-9-]+--[a-z0-9-]+$/);
        expect(scene.qaRefs.length).toBeGreaterThan(0);
        expect(scene.dwellMs).toBeGreaterThanOrEqual(7000);
      }
    }
  });

  it("distinguishes a pending, ready, and failed canonical Story frame", () => {
    const frameDocument = document.implementation.createHTMLDocument();
    const root = frameDocument.createElement("div");
    root.id = "storybook-root";
    frameDocument.body.append(root);
    expect(inspectStoryFrameDocument(frameDocument)).toBe("pending");

    root.append(frameDocument.createElement("main"));
    expect(inspectStoryFrameDocument(frameDocument)).toBe("ready");

    const error = frameDocument.createElement("div");
    error.dataset.storybook = "story-error";
    frameDocument.body.append(error);
    expect(inspectStoryFrameDocument(frameDocument)).toBe("failed");
  });

  it("surfaces the nested Story handshake and a retryable failure", async () => {
    render(
      <QaJourneyPlayback
        autoPlay={false}
        journey="session-delivery"
        playbackPace="instant"
      />,
    );
    const frameStatus = screen.getByRole("status", {
      name: "Demo frame status",
    });
    fireEvent(
      window,
      new MessageEvent("message", {
        data: {
          type: "agentrunner-story-frame",
          storyId: "pages-home--starter-intent-flow",
          frameRevision: "0",
          status: "ready",
        },
        origin: window.location.origin,
      }),
    );
    await waitFor(() => expect(frameStatus.textContent).toBe("Ready"));

    fireEvent(
      window,
      new MessageEvent("message", {
        data: {
          type: "agentrunner-story-frame",
          storyId: "pages-home--starter-intent-flow",
          frameRevision: "0",
          status: "failed",
        },
        origin: window.location.origin,
      }),
    );
    await waitFor(() =>
      expect(frameStatus.textContent).toBe("Failed to load"),
    );
    expect(screen.getByRole("button", { name: "Retry frame" })).toBeTruthy();
  });

  it("keeps displayed checkpoint progress aligned across every playlist", async () => {
    for (const journey of Object.keys(
      qaJourneyPlaylists,
    ) as Array<keyof typeof qaJourneyPlaylists>) {
      const playlist = qaJourneyPlaylists[journey];
      const view = render(
        <QaJourneyPlayback
          autoPlay={false}
          journey={journey}
          playbackPace="instant"
        />,
      );
      const transport = screen.getByRole("region", {
        name: `${playlist.title} QA demo`,
      });
      expect(within(transport).getByRole("status").textContent).toContain(
        `Step 1 / ${playlist.scenes.length}`,
      );
      expect(within(transport).getByRole("status").textContent).toContain(
        playlist.scenes[0].title,
      );

      fireEvent.click(within(transport).getByRole("button", { name: "Next" }));
      await waitFor(() =>
        expect(within(transport).getByRole("status").textContent).toContain(
          `Step 2 / ${playlist.scenes.length}`,
        ),
      );
      fireEvent.click(
        within(transport).getByRole("button", { name: "Reset" }),
      );
      await waitFor(() =>
        expect(within(transport).getByRole("status").textContent).toContain(
          `Step 1 / ${playlist.scenes.length}`,
        ),
      );
      view.unmount();
    }
  });

  it("drives the full shared transport without changing product UI", async () => {
    render(
      <QaJourneyPlayback
        autoPlay={false}
        journey="session-delivery"
        playbackPace="automated"
      />,
    );

    expect(
      screen.getByTitle("Session & Delivery: Choose a starter intent"),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() =>
      expect(
        screen.getByTitle(
          "Session & Delivery: Choose project and run location",
        ),
      ).toBeTruthy(),
    );
    const transport = screen.getByRole("region", {
      name: "Session & Delivery QA demo",
    });
    expect(within(transport).getByRole("status").textContent).toContain(
      "Step 2 / 8",
    );
    expect(within(transport).getByRole("status").textContent).toContain(
      "paused",
    );

    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    await waitFor(() =>
      expect(
        screen.getByTitle("Session & Delivery: Choose a starter intent"),
      ).toBeTruthy(),
    );
    expect(within(transport).getByRole("status").textContent).toContain("idle");
    expect(within(transport).getByRole("status").textContent).toContain(
      "Step 1 / 8",
    );

    fireEvent.change(
      within(transport).getByRole("combobox", { name: "Playback speed" }),
      { target: { value: "2" } },
    );
    expect(
      within(transport).getByRole<HTMLSelectElement>("combobox", {
        name: "Playback speed",
      }).value,
    ).toBe("2");

    fireEvent.click(within(transport).getByRole("button", { name: "Play" }));
    expect(within(transport).getByRole("status").textContent).toContain(
      "running",
    );
    fireEvent.click(within(transport).getByRole("button", { name: "Pause" }));
    await waitFor(() =>
      expect(within(transport).getByRole("status").textContent).toContain(
        "paused",
      ),
    );

    fireEvent.click(within(transport).getByRole("button", { name: "Replay" }));
    await waitFor(() =>
      expect(within(transport).getByRole("status").textContent).toContain(
        "running",
      ),
    );
    fireEvent.click(within(transport).getByRole("button", { name: "Pause" }));

    fireEvent.click(within(transport).getByRole("button", { name: "Reset" }));
    await waitFor(() =>
      expect(within(transport).getByRole("status").textContent).toContain(
        "idle",
      ),
    );
    fireEvent.click(
      within(transport).getByRole("checkbox", { name: "Autoplay" }),
    );
    await waitFor(() =>
      expect(within(transport).getByRole("status").textContent).toContain(
        "running",
      ),
    );
    fireEvent.click(within(transport).getByRole("button", { name: "Pause" }));
  });
});
