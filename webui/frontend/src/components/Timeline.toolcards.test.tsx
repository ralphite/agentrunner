// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { TimelineView } from "./Timeline";
import type { ToolItem } from "../timeline";

const { copyText } = vi.hoisted(() => ({
  copyText: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("../clipboard", () => ({ copyText }));

afterEach(() => {
  cleanup();
  copyText.mockClear();
});

function bashTool(command: string, stdout: string): ToolItem {
  return {
    kind: "tool",
    key: "tool-1",
    name: "bash",
    args: { command },
    background: false,
    status: "done",
    statusText: "done",
    result: { stdout, exit_code: 0 },
  };
}

describe("Timeline tool cards on narrow screens", () => {
  it("keeps a multiline command quiet while collapsed and preserves the full detail", () => {
    const command = `cat << 'EOF' > cli/cli.go\n${"const longToken = 'x';\n".repeat(400)}EOF`;
    const result = `ok\n${"unbroken-result-token-".repeat(80)}`;
    const { container, getByRole } = render(
      <TimelineView
        items={[bashTool(command, result)]}
        pending={[]}
        typing=""
        showSys
      />,
    );

    const card = container.querySelector("details.step") as HTMLDetailsElement;
    const summary = card.querySelector("summary") as HTMLElement;
    const body = card.querySelector(".step-body") as HTMLElement;
    expect(card.open).toBe(false);
    expect(summary.textContent).toContain("cat << 'EOF' > cli/cli.go");
    expect(summary.textContent).not.toContain("const longToken");
    expect(summary.className).toContain("min-w-0");
    expect(body.className).toContain("truncate");
    expect(body.getAttribute("title")).toBe(command);
    expect(card.querySelector(".step-caret")).not.toBeNull();

    fireEvent.click(summary);
    expect(card.open).toBe(true);
    expect(container.querySelector(".shell-cmd")?.textContent).toContain("const longToken");
    expect(container.querySelector(".shell-out")?.textContent).toContain("unbroken-result-token");
    expect(container.querySelector(".shell-cmd")?.className).toContain("break-words");
    expect(container.querySelector(".shell-out")?.className).toContain("max-h-[240px]");
    expect(getByRole("button", { name: "Copy command and result" })).toBeTruthy();
  });

  it("copies the complete command and result rather than the collapsed preview", async () => {
    const command = `printf '%s' '${"a".repeat(500)}'`;
    const result = "full-result-" + "b".repeat(500);
    const { container, getByRole } = render(
      <TimelineView
        items={[bashTool(command, result)]}
        pending={[]}
        typing=""
        showSys
      />,
    );

    fireEvent.click(container.querySelector("details.step > summary") as HTMLElement);
    fireEvent.click(getByRole("button", { name: "Copy command and result" }));

    await vi.waitFor(() => expect(copyText).toHaveBeenCalledOnce());
    expect(copyText).toHaveBeenCalledWith(`$ ${command}\n${result}`);
  });
});
