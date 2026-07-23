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

  it("does not repeat a short single-line command inside its detail", () => {
    const command = "printf TH10-AR-STDERR >&2; exit 23";
    const { container, getByRole } = render(
      <TimelineView items={[bashTool(command, "TH10-AR-STDERR")]} pending={[]} typing="" showSys />,
    );

    fireEvent.click(container.querySelector("details.step > summary") as HTMLElement);
    expect(container.querySelector(".step-body")?.textContent).toBe(command);
    expect(container.querySelector(".shell-cmd")).toBeNull();
    expect(container.querySelector(".shell-out")?.textContent).toContain("TH10-AR-STDERR");
    expect(container.querySelector(".shell-footer")?.contains(getByRole("button", { name: "Copy command and result" }))).toBe(true);
    expect(container.querySelector(".shell")?.textContent).not.toContain("Shell");
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

  it("includes the decisive exit status when copying a failed command", async () => {
    const failed: ToolItem = {
      ...bashTool("echo START; exit 7", "START\n"),
      status: "failed",
      statusText: "failed",
      result: { stdout: "START\n", exit_code: 7 },
    };
    const { container, getByRole } = render(
      <TimelineView items={[failed]} pending={[]} typing="" showSys />,
    );

    fireEvent.click(container.querySelector("details.step > summary") as HTMLElement);
    expect(container.querySelector(".shell-status")?.textContent).toContain("Exit 7");
    fireEvent.click(getByRole("button", { name: "Copy command and result" }));

    await vi.waitFor(() => expect(copyText).toHaveBeenCalledOnce());
    expect(copyText).toHaveBeenCalledWith("$ echo START; exit 7\nSTART\nExit 7");
  });

  it.each([
    ["cancelled", "Cancelled"],
    ["failed", "Failed"],
  ] as const)("copies the %s terminal state when no exit code exists", async (status, label) => {
    const terminal: ToolItem = {
      ...bashTool("long-running-command", "partial\n"),
      status,
      statusText: status,
      result: { stdout: "partial\n" },
    };
    const { container, getByRole } = render(
      <TimelineView items={[terminal]} pending={[]} typing="" showSys />,
    );

    fireEvent.click(container.querySelector("details.step > summary") as HTMLElement);
    expect(container.querySelector(".shell-status")?.textContent).toContain(label);
    fireEvent.click(getByRole("button", { name: "Copy command and result" }));

    await vi.waitFor(() => expect(copyText).toHaveBeenCalledOnce());
    expect(copyText).toHaveBeenCalledWith(`$ long-running-command\npartial\n${label}`);
  });
});
