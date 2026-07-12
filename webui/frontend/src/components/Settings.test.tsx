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
