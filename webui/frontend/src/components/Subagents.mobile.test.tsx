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

  it("lets a typed approval wait outrank the broad waiting status", () => {
    render(
      <Subagents
        nodes={[{
          agent: "worker",
          session: "parent-sub-call_1_0-a1",
          report: {
            status: "waiting",
            waiting: { kind: "approval", approval_id: "apr-1", tool: "bash" },
          },
        }]}
        onOpen={vi.fn()}
      />,
    );

    const row = screen.getByRole("button", { name: /worker Needs approval/i });
    expect(row.querySelector(".sa-status")?.textContent).toBe("Needs approval");
    expect(row.querySelector(".sa-dot")?.classList.contains("appr")).toBe(true);
  });
});
