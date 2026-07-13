// @vitest-environment jsdom

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Subagents, type InspectNode } from "./Subagents";

describe("Subagents mobile layout", () => {
  it("stacks long identity and metadata while capping deep indentation", () => {
    const leaf: InspectNode = {
      agent: "agent-with-a-very-long-name",
      session: "leaf-session",
      status: "max_generation_steps",
      gen_steps: 123,
      usage: { billed: 195_000 },
    };
    const nodes = [5, 4, 3, 2, 1].reduce<InspectNode[]>((children, depth) => [{ call_id: `level-${depth}`, report: { children } }], [leaf]);
    const onOpen = vi.fn();
    const { container } = render(<Subagents nodes={nodes} onOpen={onOpen} />);

    const button = screen.getByRole("button", { name: /agent-with-a-very-long-name/i });
    expect([...button.classList]).toEqual(expect.arrayContaining(["max-[520px]:grid", "max-[520px]:grid-cols-[auto_minmax(0,1fr)_auto]"]));
    expect(button.querySelector(".sa-status")?.classList.contains("truncate")).toBe(true);
    expect(button.textContent).toContain("123 steps195k tokopen");
    expect(container.querySelector('[data-depth="4"]')?.classList.contains("ml-12")).toBe(true);
    expect(container.querySelector('[data-depth="5"]')?.classList.contains("ml-12")).toBe(true);

    fireEvent.click(button);
    expect(onOpen).toHaveBeenCalledWith("leaf-session");
  });
});
