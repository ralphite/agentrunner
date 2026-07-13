// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useStore } from "../store";
import { SettingsArchived } from "./SettingsArchived";

afterEach(cleanup);

describe("SettingsArchived mobile layout", () => {
  it("contains long project and session content while preserving both actions", () => {
    const select = vi.fn();
    const toggleArchive = vi.fn();
    const onClose = vi.fn();
    const title = "A very long archived session title that must not push actions outside a 390px viewport";
    useStore.setState({
      sessions: [{ id: "archived-1", status: "waiting_for_approval", turns: 2, title, workspace: "/very/long/project/path/agentrunner" }],
      archived: ["archived-1"],
      renames: {},
      select,
      toggleArchive,
    });

    const { container } = render(<SettingsArchived query="" onClose={onClose} />);
    const header = container.querySelector(".rs-archive-group > header")!;
    const row = container.querySelector(".rs-archive-row")!;
    const open = screen.getByRole("button", { name: `Open ${title}` });
    const sessionTitle = screen.getByText(title);

    expect(header.className).toContain("grid-cols-[auto_minmax(0,1fr)_auto]");
    expect(row.className).toContain("max-[520px]:grid-cols-[minmax(0,1fr)_auto]");
    expect(open.className).toContain("grid-cols-[minmax(0,1fr)_auto]");
    expect(sessionTitle.className).toContain("truncate");

    fireEvent.click(open);
    expect(select).toHaveBeenCalledWith("archived-1");
    expect(onClose).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Unarchive" }));
    expect(toggleArchive).toHaveBeenCalledWith("archived-1");
  });
});
