// @vitest-environment jsdom

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { childAnswerRequests, Subagents, type InspectNode } from "./Subagents";

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
    expect(button.classList.contains("sa-row")).toBe(true);
    expect(button.querySelector(".sa-status")?.classList.contains("whitespace-normal")).toBe(true);
    expect(button.textContent).toContain("123 steps");
    expect(button.textContent).toContain("195k tok");
    expect(button.textContent).toContain("open");
    expect(container.querySelector('[data-depth="4"]')?.classList.contains("ml-12")).toBe(true);
    expect(container.querySelector('[data-depth="5"]')?.classList.contains("ml-12")).toBe(true);

    fireEvent.click(button);
    expect(onOpen).toHaveBeenCalledWith("leaf-session");
  });

  it("uses each real delegation task as the primary identity", () => {
    const nodes: InspectNode[] = [
      { call_id: "call-a", agent: "worker", session: "child-a", status: "running" },
      { call_id: "call-b", agent: "worker", session: "child-b", status: "waiting" },
      { call_id: "call-c", agent: "worker", session: "child-c", reason: "completed" },
    ];
    const { container } = render(
      <Subagents
        nodes={nodes}
        delegations={[
          { call_id: "call-a", assigned_to: "child-a", description: "Audit keyboard focus across the sidebar." },
          { call_id: "call-b", assigned_to: "child-b", description: "Review narrow-screen header layout." },
          { call_id: "call-c", assigned_to: "child-c", description: "Compare menu actions with Codex." },
        ]}
        onOpen={vi.fn()}
      />,
    );

    expect(
      [...container.querySelectorAll(".sa-name")].map((item) => item.textContent),
    ).toEqual([
      "Audit keyboard focus across the sidebar.",
      "Review narrow-screen header layout.",
      "Compare menu actions with Codex.",
    ]);
    expect(screen.getByRole("button", { name: /Audit keyboard focus.*worker · Running/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Review narrow-screen.*worker · Ready/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Compare menu actions.*worker · Completed/i })).toBeTruthy();
    expect(container.querySelector('[title*="child-a"]')).toBeNull();
  });

  it("strips the workspace preamble from a child task identity", () => {
    render(
      <Subagents
        nodes={[{
          agent: "worker",
          session: "child-a",
          task:
            "[workspace note] Do not expose this path.\n\nInspect the responsive navigation.\nVerify focus restoration after closing it.",
          status: "running",
        }]}
        onOpen={vi.fn()}
      />,
    );

    expect(
      screen.getByText(
        "Inspect the responsive navigation. Verify focus restoration after closing it.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/Do not expose this path/)).toBeNull();
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

  it("surfaces nested structured asks as typed answer attention", () => {
    const nodes: InspectNode[] = [{
      agent: "lead-worker",
      session: "parent-sub-lead-a1",
      report: {
        children: [{
          agent: "release-reviewer",
          session: "parent-sub-lead-a1-sub-ask-a1",
          report: {
            status: "waiting",
            waiting: {
              kind: "input",
              ask_questions: [{
                question: "Choose the release channel",
                options: [{ label: "Stable" }, { label: "Beta" }],
              }],
            },
          },
        }],
      },
    }];
    render(<Subagents nodes={nodes} onOpen={vi.fn()} />);

    const row = screen.getByRole("button", { name: /release-reviewer Needs answer/i });
    expect(row.querySelector(".sa-dot")?.classList.contains("appr")).toBe(true);
    expect(childAnswerRequests(nodes)).toEqual([{
      agent: "release-reviewer",
      session: "parent-sub-lead-a1-sub-ask-a1",
    }]);
  });
});
