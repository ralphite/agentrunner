// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useStore } from "../store";
import { Toasts } from "./Toasts";

describe("Toasts details disclosure (G36 余项)", () => {
  afterEach(() => {
    cleanup();
    useStore.setState({ toasts: [] });
  });

  it("keeps stderr out of the sentence but one tap away", () => {
    useStore.getState().toast("worktree apply failed", "error", "error: patch failed\nexit status 1");
    render(<Toasts />);
    expect(screen.getByText("worktree apply failed")).toBeTruthy();
    const summary = screen.getByText("Details");
    fireEvent.click(summary);
    expect(screen.getByText(/exit status 1/)).toBeTruthy();
    // Toggling the disclosure must not dismiss the toast.
    expect(useStore.getState().toasts.length).toBe(1);
  });

  it("renders no disclosure without details", () => {
    useStore.getState().toast("plain failure");
    render(<Toasts />);
    expect(screen.queryByText("Details")).toBeNull();
  });
});
