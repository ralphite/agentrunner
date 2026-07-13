// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

import { ApprovalCard } from "./ApprovalCard";

afterEach(cleanup);

const approval = {
  id: "apr-mobile",
  tool: "bash",
  args: {
    command: "printf '%s\\n' a-very-long-unbroken-command-token-that-must-stay-inside-the-card",
  },
  gates: [],
};

describe("ApprovalCard mobile decision flow", () => {
  it("keeps long command and workspace text inside a shrinkable card", () => {
    const { container } = render(
      <ApprovalCard
        approval={approval}
        readonly={false}
        workspace="/Users/test/.local/share/agentrunner/worktrees/a-very-long-unbroken-workspace-name-that-must-wrap"
        onDecide={vi.fn()}
        onError={vi.fn()}
      />,
    );

    const card = screen.getByRole("region", { name: "Approval required" });
    const subject = container.querySelector(".approval-subject");
    const subjectCode = subject?.querySelector("code");
    const workspace = container.querySelector(".approval-ws");

    expect(card.classList.contains("min-w-0")).toBe(true);
    expect(card.classList.contains("overflow-hidden")).toBe(true);
    expect(subject?.classList.contains("min-w-0")).toBe(true);
    expect(subjectCode?.classList.contains("flex-1")).toBe(true);
    expect(subjectCode?.classList.contains("[overflow-wrap:anywhere]")).toBe(true);
    expect(workspace?.classList.contains("basis-full")).toBe(true);
    expect(workspace?.classList.contains("whitespace-normal")).toBe(true);
    expect(workspace?.classList.contains("[overflow-wrap:anywhere]")).toBe(true);
  });

  it("puts the primary approval first and gives it the full mobile row", () => {
    const { container } = render(
      <ApprovalCard approval={approval} readonly={false} onDecide={vi.fn()} onError={vi.fn()} />,
    );

    const actions = container.querySelector(".approval-actions");
    const buttons = Array.from(actions?.querySelectorAll("button") || []);

    expect(buttons.map((button) => button.textContent)).toEqual(["Approve once", "Always allow", "Deny"]);
    expect(buttons[0].classList.contains("primary")).toBe(true);
    expect(buttons[0].classList.contains("flex-[1_1_100%]")).toBe(true);
    expect(buttons[1].classList.contains("subtle")).toBe(true);
  });

  it("stacks the denial reason above reachable secondary and destructive actions", async () => {
    const onDecide = vi.fn().mockResolvedValue(undefined);
    const { container } = render(
      <ApprovalCard approval={approval} readonly={false} onDecide={onDecide} onError={vi.fn()} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Deny" }));

    const reason = screen.getByPlaceholderText("Reason (optional)");
    const denyFlow = container.querySelector(".deny-reason");
    expect(denyFlow?.classList.contains("w-full")).toBe(true);
    expect(denyFlow?.classList.contains("min-w-0")).toBe(true);
    expect(denyFlow?.classList.contains("flex-col")).toBe(true);
    expect(reason.classList.contains("w-full")).toBe(true);

    fireEvent.change(reason, { target: { value: "Not needed" } });
    fireEvent.click(screen.getByRole("button", { name: "Deny" }));

    await waitFor(() => expect(onDecide).toHaveBeenCalledWith("apr-mobile", "deny", "Not needed", false));
  });
});
