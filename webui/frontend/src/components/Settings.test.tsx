// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Settings } from "./Settings";

afterEach(cleanup);

describe("Settings search", () => {
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
    expect(screen.queryByText("Branch prefix")).toBeNull();
  });
});

describe("Settings mobile shell", () => {
  it("keeps navigation compact and gives panel content the only vertical scroller", () => {
    render(<Settings onClose={() => {}} />);

    const dialog = screen.getByRole("dialog", { name: "Settings" });
    const rail = screen.getByRole("complementary");
    const navigation = screen.getByRole("navigation");
    const main = screen.getByRole("main");
    const content = main.lastElementChild as HTMLElement;

    expect(dialog.className).toContain("h-[100dvh]");
    expect(dialog.className).toContain("overflow-hidden");
    expect(rail.className).toContain("max-[720px]:grid");
    expect(rail.className).toContain("max-[720px]:overflow-hidden");
    expect(rail.className).not.toContain("max-[720px]:max-h-[45vh]");
    expect(navigation.className).toContain("max-[720px]:flex-row");
    expect(navigation.className).toContain("max-[720px]:overflow-x-auto");
    expect(main.className).toContain("min-h-0");
    expect(main.className).toContain("overflow-hidden");
    expect(content.className).toContain("min-h-0");
    expect(content.className).toContain("overflow-y-auto");
  });
});
