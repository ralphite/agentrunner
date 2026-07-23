// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useStore } from "../store";
import { SettingsWorktrees } from "./SettingsWorktrees";

const workspace = "/Users/test/.local/share/agentrunner/worktrees/a-very-long-unbroken-workspace-name-that-must-wrap-at-mobile-width";
const title = "A very long conversation title with-an-unbroken-suffix-that-must-not-overflow-the-worktree-card";
const select = vi.fn();

beforeEach(() => {
  select.mockClear();
  useStore.setState({
    sessions: [{ id: "session-mobile", status: "idle", turns: 1, workspace, title }],
    renames: {},
    select,
  });
});

afterEach(cleanup);

describe("SettingsWorktrees mobile layout", () => {
  it("wraps long workspace and session labels while preserving session selection", () => {
    const { container } = render(<SettingsWorktrees query="" />);
    const card = container.querySelector(".rs-wt-card")!;
    const head = container.querySelector(".rs-wt-head")!;
    const path = screen.getByText(workspace);
    const count = screen.getByText("1 conversation");
    const session = screen.getByRole("button", { name: title });

    expect([...card.classList]).toEqual(expect.arrayContaining(["min-w-0", "overflow-hidden", "max-[500px]:p-2.5"]));
    expect([...head.classList]).toEqual(expect.arrayContaining(["min-w-0", "max-[500px]:flex-col", "max-[500px]:items-start"]));
    expect([...path.classList]).toEqual(expect.arrayContaining(["min-w-0", "max-w-full", "[overflow-wrap:anywhere]"]));
    expect([...count.classList]).toEqual(expect.arrayContaining(["shrink-0", "whitespace-nowrap"]));
    expect([...session.classList]).toEqual(expect.arrayContaining(["min-w-0", "max-w-full", "w-full", "whitespace-normal", "[overflow-wrap:anywhere]"]));

    fireEvent.click(session);
    expect(select).toHaveBeenCalledWith("session-mobile");
  });

  it("progressively reveals a large shared-store workspace list", () => {
    useStore.setState({
      sessions: Array.from({ length: 45 }, (_, i) => ({
        id: `session-${i}`,
        status: "idle",
        turns: 1,
        workspace: `/workspace/${String(i).padStart(2, "0")}`,
        title: `Session ${i}`,
      })),
      renames: {},
      select,
    });

    const { container } = render(<SettingsWorktrees query="" />);
    expect(container.querySelectorAll(".rs-wt-card")).toHaveLength(40);

    fireEvent.click(screen.getByRole("button", { name: "Show 5 more · 5 remaining" }));
    expect(container.querySelectorAll(".rs-wt-card")).toHaveLength(45);
    expect(screen.queryByRole("button", { name: /remaining/ })).toBeNull();
  });
});
