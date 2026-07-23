// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Settings } from "./Settings";

afterEach(cleanup);

describe("Settings search", () => {
  it("opens the undirected Settings entry on General", () => {
    render(<Settings onClose={() => {}} />);

    expect(screen.getByRole("heading", { name: "General" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "General" }).getAttribute("aria-current")).toBe("true");
  });

  it("shows a complete section when its name matches", () => {
    render(<Settings onClose={() => {}} />);

    fireEvent.change(screen.getByRole("textbox", { name: "Search settings" }), {
      target: { value: "git" },
    });

    expect(screen.getByRole("heading", { name: "Git" })).toBeTruthy();
    expect(screen.getByText("Commit message template")).toBeTruthy();
    expect(screen.queryByText(/No Git settings match/)).toBeNull();
  });

  it("keeps row-level filtering for a setting keyword", () => {
    render(<Settings onClose={() => {}} />);

    fireEvent.change(screen.getByRole("textbox", { name: "Search settings" }), {
      target: { value: "commit" },
    });

    expect(screen.getByText("Commit message template")).toBeTruthy();
    expect(screen.queryByText("Theme")).toBeNull();
  });
});

describe("Settings mobile shell", () => {
  it("stacks mobile chrome, wraps tabs, and gives panel content the only scroller", () => {
    render(<Settings onClose={() => {}} />);

    const dialog = screen.getByRole("dialog", { name: "Settings" });
    const rail = screen.getByRole("complementary");
    const navigation = screen.getByRole("navigation");
    const main = screen.getByRole("main");
    const content = main.lastElementChild as HTMLElement;
    const back = screen.getByRole("button", { name: "Back to app" });
    const done = screen.getByRole("button", { name: "Close settings" });

    expect(dialog.className).toContain("h-[100dvh]");
    expect(dialog.className).toContain("overflow-hidden");
    expect(rail.className).toContain("max-[720px]:grid-cols-1");
    expect(rail.className).toContain("max-[720px]:overflow-hidden");
    expect(navigation.className).toContain("max-[720px]:flex-row");
    expect(navigation.className).toContain("max-[720px]:flex-wrap");
    expect(navigation.className).toContain("max-[720px]:overflow-visible");
    expect(navigation.className).not.toContain("max-[720px]:overflow-x-auto");
    expect(main.firstElementChild?.className).toContain("max-[720px]:hidden");
    expect(back.className).toContain("hidden");
    expect(back.className).toContain("max-[720px]:inline-flex");
    expect(done.closest("header")?.className).toContain("max-[720px]:hidden");
    expect(main.className).toContain("min-h-0");
    expect(main.className).toContain("overflow-hidden");
    expect(content.className).toContain("min-h-0");
    expect(content.className).toContain("overflow-y-auto");
  });
});
