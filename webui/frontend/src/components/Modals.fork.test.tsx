// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  barriers: vi.fn(async () => [
    "bar-t1",
    ...Array.from({ length: 124 }, (_, i) => `bar-t${i + 2}`),
    "bar-final",
  ]),
  fork: vi.fn(async () => ({ sid: "20260723-080000-fork-title" })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    barriers: mocks.barriers,
    fork: mocks.fork,
  },
}));

import { useStore } from "../store";
import { Modals } from "./Modals";

const SID = "20260713-030529-build-a-production-like-go-cli-a404";

beforeEach(() => {
  mocks.barriers.mockClear();
  mocks.fork.mockClear();
  useStore.setState({
    modal: { kind: "fork", sid: SID },
    sessions: [{ id: SID, title: "Initialize Taskledger CLI Skeleton", workspace: "/tmp/taskledger" } as any],
    refreshSessions: vi.fn(async () => {}),
    select: vi.fn(),
    toast: vi.fn(),
    openModal: (modal: any) => useStore.setState({ modal }),
  } as any);
});

afterEach(() => {
  cleanup();
  useStore.setState({ modal: null });
});

describe("complex checkpoint continuation", () => {
  it("keeps 125 internal steps behind disclosure and labels them truthfully", async () => {
    render(<Modals />);

    expect(await screen.findByText("Latest — end of the conversation")).toBeTruthy();
    expect(screen.queryByRole("combobox")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Choose an earlier checkpoint" }));
    const picker = screen.getByRole("combobox") as HTMLSelectElement;
    expect(picker.options).toHaveLength(126);
    expect(screen.getByRole("option", { name: "After agent step 125" })).toBeTruthy();
    expect(screen.queryByRole("option", { name: "After turn 125" })).toBeNull();

    fireEvent.change(picker, { target: { value: "bar-t60" } });
    fireEvent.click(screen.getByRole("button", { name: /^Continue$/ }));
    await waitFor(() => expect(mocks.fork).toHaveBeenCalledWith(SID, "bar-t60", ""));
  });
});
