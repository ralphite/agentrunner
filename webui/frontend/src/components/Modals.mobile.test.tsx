// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

import { useStore } from "../store";
import { Modals } from "./Modals";

afterEach(() => {
  cleanup();
  useStore.setState({ modal: null });
  document.documentElement.style.removeProperty("--app-vvh");
});

describe("mobile modal shell", () => {
  it("keeps one scroll region with fixed chrome and a full-size close target", () => {
    useStore.setState({
      modal: {
        kind: "confirm",
        title: "Remove worktree?",
        body: "This intentionally long copy exercises the shared modal shell.",
        confirmLabel: "Remove",
        onConfirm: vi.fn(),
      },
    });

    const { container } = render(<Modals />);
    const dialog = screen.getByRole("dialog", { name: "Remove worktree?" });
    const backdrop = container.querySelector(".backdrop")!;
    const header = container.querySelector(".mhead")!;
    const body = container.querySelector(".mbody")!;
    const footer = container.querySelector(".mfoot")!;
    const close = screen.getByRole("button", { name: "Close dialog" });

    expect(backdrop.className).toContain("h-[var(--app-vvh,100dvh)]");
    expect(backdrop.className).toContain("overflow-hidden");
    expect(dialog.className).toContain("flex-col");
    expect(dialog.className).toContain("max-[640px]:max-h-[calc(var(--app-vvh,100dvh)-1rem)]");
    expect(header.className).toContain("shrink-0");
    expect(body.className).toContain("overflow-y-auto");
    expect(body.className).toContain("min-h-0");
    expect(footer.className).toContain("shrink-0");
    expect(footer.className).toContain("max-[640px]:flex-wrap");
    expect(close.className).toContain("h-11");
    expect(close.className).toContain("w-11");
  });

  it("renders structured capability details inside the same scroll region", () => {
    useStore.setState({
      modal: {
        kind: "confirm",
        title: "Turn on Full Access?",
        body: "The agent can act without asking, including:",
        confirmLabel: "Turn on Full Access",
        details: [
          { icon: "files", title: "Files and folders", body: "Read or delete files anywhere" },
          { icon: "terminal", title: "Terminal commands", body: "Run commands and install software" },
          { icon: "internet", title: "Internet access", body: "Access websites and send data" },
        ],
        note: "This increases prompt-injection risk.",
        onConfirm: vi.fn(),
      },
    });

    const { container } = render(<Modals />);
    expect(screen.getByText("Files and folders")).toBeTruthy();
    expect(screen.getByText("Terminal commands")).toBeTruthy();
    expect(screen.getByText("Internet access")).toBeTruthy();
    expect(screen.getByText("This increases prompt-injection risk.")).toBeTruthy();
    expect(container.querySelectorAll(".confirm-detail")).toHaveLength(3);
    expect(container.querySelector(".confirm-details")?.closest(".mbody")).toBeTruthy();
  });
});
