// @vitest-environment jsdom
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useStore } from "../store";
import { RunView } from "./RunView";

const LONG_COMMAND =
  "printf mobile-run-log-overflow-check-/Users/yadong/.local/share/agentrunner/qa-workspaces/qa-inc49-runview-mobile/0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ";

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  close = vi.fn();
  addEventListener = vi.fn();

  constructor(readonly url: string) {
    FakeEventSource.instances.push(this);
  }

  emit(data: string) {
    this.onmessage?.({ data } as MessageEvent<string>);
  }
}

beforeEach(() => {
  FakeEventSource.instances = [];
  (globalThis as any).EventSource = FakeEventSource;
  useStore.setState({
    runs: [
      {
        id: "run-mobile",
        kind: "submit",
        label: "Run exactly one bash command with an intentionally long mobile title",
        workspace: "/tmp/run-mobile",
        status: "running",
        startedAt: "2026-07-13T06:49:10Z",
      } as any,
    ],
  });
});

afterEach(cleanup);

describe("RunView mobile run log", () => {
  it("keeps topbar actions visible and wraps long log content without page-level horizontal scrolling", () => {
    const { container } = render(<RunView runId="run-mobile" />);

    const topbar = container.querySelector(".topbar")!;
    const navSlot = topbar.querySelector(".run-topbar-nav-slot")!;
    const title = topbar.querySelector(".sid")!;
    const status = screen.getByText("running");
    const kind = screen.getByText("submit run");
    const stop = screen.getByRole("button", { name: "Stop run" });

    expect(navSlot).toBe(topbar.firstElementChild);
    expect(navSlot.classList.contains("max-[900px]:block")).toBe(true);
    expect(title.classList.contains("min-w-0")).toBe(true);
    expect(title.classList.contains("flex-1")).toBe(true);
    expect(title.classList.contains("truncate")).toBe(true);
    expect(title.getAttribute("title")).toContain("intentionally long mobile title");
    expect(status.classList.contains("shrink-0")).toBe(true);
    expect(kind.classList.contains("shrink-0")).toBe(true);
    expect(stop.querySelector("svg")).not.toBeNull();

    act(() => {
      FakeEventSource.instances[0].emit(
        JSON.stringify({ kind: "tool_call", tool: "bash", args: { command: LONG_COMMAND } }),
      );
    });

    const log = screen.getByRole("log", { name: "Run output" });
    const row = container.querySelector(".runline")!;
    const rowText = row.querySelector(".rt")!;

    expect(log.classList.contains("overflow-x-hidden")).toBe(true);
    expect(log.classList.contains("overflow-y-auto")).toBe(true);
    expect(row.classList.contains("grid-cols-[104px_minmax(0,1fr)]")).toBe(true);
    expect(rowText.textContent).toContain(LONG_COMMAND);
    expect(rowText.classList.contains("min-w-0")).toBe(true);
    expect(rowText.classList.contains("whitespace-pre-wrap")).toBe(true);
    expect(rowText.classList.contains("break-words")).toBe(true);
  });

  it("keeps the run id as the title without empty metadata pills when the registry no longer has the run", () => {
    useStore.setState({ runs: [] });

    const { container } = render(<RunView runId="run-missing-after-restart" />);
    const title = container.querySelector(".topbar .sid")!;

    expect(title.textContent).toBe("run-missing-after-restart");
    expect(title.getAttribute("title")).toBe("run-missing-after-restart");
    expect(container.querySelector(".topbar .pill")).toBeNull();
    expect(container.querySelector(".topbar .readonly-tag")).toBeNull();
    expect(screen.queryByRole("button", { name: "Stop run" })).toBeNull();
  });
});
