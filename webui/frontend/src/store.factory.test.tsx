// @vitest-environment jsdom
import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AppStoreProvider, createAppStore, useStore } from "./store";
import { createStoryAppServices } from "./storybook/appServices";
import type { Session } from "./types";

const session = (id: string): Session => ({
  id,
  status: "completed",
  title: id,
  turns: 1,
});

function StoreProbe({ label }: { label: string }) {
  const showSys = useStore((state) => state.showSys);
  const toggleSys = useStore((state) => state.toggleSys);
  return <button onClick={toggleSys}>{label}:{String(showSys)}</button>;
}

describe("createAppStore", () => {
  it("isolates provider state, storage, navigation, and id sequences", () => {
    const leftHarness = createStoryAppServices({
      local: {
        "arwebui.archived": JSON.stringify(["left-archived"]),
        "arwebui.pinned": JSON.stringify(["left-pinned"]),
      },
    });
    const rightHarness = createStoryAppServices();
    const left = createAppStore(leftHarness.services);
    const right = createAppStore(rightHarness.services);

    render(
      <>
        <AppStoreProvider store={left}><StoreProbe label="left" /></AppStoreProvider>
        <AppStoreProvider store={right}><StoreProbe label="right" /></AppStoreProvider>
      </>,
    );

    expect(left.getState().archived).toEqual(["left-archived"]);
    expect(left.getState().pinned).toEqual(["left-pinned"]);
    expect(right.getState().archived).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "left:false" }));
    expect(screen.getByRole("button", { name: "left:true" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "right:false" })).toBeTruthy();

    act(() => {
      left.getState().togglePin("new-left");
      left.getState().newSessionForProject("/tmp/left/");
      right.getState().newSessionForProject("/tmp/right");
    });
    expect(JSON.parse(leftHarness.local.getItem("arwebui.pinned") ?? "[]")).toEqual([
      "new-left",
      "left-pinned",
    ]);
    expect(rightHarness.local.getItem("arwebui.pinned")).toBeNull();
    expect(leftHarness.navigation.hash()).toBe("");
    expect(left.getState().newSessionProject).toEqual({
      workspace: "/tmp/left",
      requestId: 1,
    });
    expect(right.getState().newSessionProject?.requestId).toBe(1);
  });

  it("coalesces refreshes per store without coupling independent stories", async () => {
    let releaseLeft!: (sessions: Session[]) => void;
    let releaseRight!: (sessions: Session[]) => void;
    const leftRequest = vi.fn(
      () => new Promise<Session[]>((resolve) => { releaseLeft = resolve; }),
    );
    const rightRequest = vi.fn(
      () => new Promise<Session[]>((resolve) => { releaseRight = resolve; }),
    );
    const leftHarness = createStoryAppServices({
      api: { ...createStoryAppServices().services.api, sessions: leftRequest },
    });
    const rightHarness = createStoryAppServices({
      api: { ...createStoryAppServices().services.api, sessions: rightRequest },
    });
    const left = createAppStore(leftHarness.services);
    const right = createAppStore(rightHarness.services);

    const leftFirst = left.getState().refreshSessions();
    const leftSecond = left.getState().refreshSessions();
    const rightFirst = right.getState().refreshSessions();
    expect(leftRequest).toHaveBeenCalledTimes(1);
    expect(rightRequest).toHaveBeenCalledTimes(1);

    releaseLeft([session("left")]);
    releaseRight([session("right")]);
    await Promise.all([leftFirst, leftSecond, rightFirst]);
    expect(left.getState().sessions.map(({ id }) => id)).toEqual(["left"]);
    expect(right.getState().sessions.map(({ id }) => id)).toEqual(["right"]);
  });

  it("never runs the retired rename migration unless production opts in", async () => {
    const rename = vi.fn(async () => ({}));
    const safeHarness = createStoryAppServices({
      api: { ...createStoryAppServices().services.api, rename },
      local: { "arwebui.renames": JSON.stringify({ legacy: "Legacy title" }) },
    });
    createAppStore(safeHarness.services);
    await Promise.resolve();
    expect(rename).not.toHaveBeenCalled();
    expect(safeHarness.local.getItem("arwebui.renames")).not.toBeNull();

    const migrationHarness = createStoryAppServices({
      api: { ...createStoryAppServices().services.api, rename },
      local: { "arwebui.renames": JSON.stringify({ legacy: " Legacy title " }) },
    });
    createAppStore(migrationHarness.services, { migrateLegacyRenames: true });
    await vi.waitFor(() => expect(rename).toHaveBeenCalledWith("legacy", "Legacy title"));
    await vi.waitFor(() => expect(migrationHarness.local.getItem("arwebui.renames")).toBeNull());
  });
});
