// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BackgroundProcessRow } from "./SupervisionParts";
import { SubagentItem } from "./Subagents";

afterEach(cleanup);

// A kill affordance must reach exactly one running unit and never appear on
// work that already finished — a button that stops nothing is worse than no
// button (an earlier attempt shipped one wired to a transport that no longer
// existed and had to be reverted).
describe("kill affordances", () => {
  it("stops one background command by its handle", async () => {
    const onKill = vi.fn();
    render(
      <BackgroundProcessRow
        work={{ handle: "call_1_0", tool: "bash", detail: "sleep 300" }}
        onKill={onKill}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /^Stop / }));
    expect(onKill).toHaveBeenCalledWith("call_1_0");
  });

  it("offers no stop button when the surface is read-only", () => {
    render(
      <BackgroundProcessRow work={{ handle: "call_1_0", tool: "bash", detail: "" }} />,
    );
    expect(screen.queryByRole("button", { name: /^Stop / })).toBeNull();
  });

  it("stops a RUNNING subagent by its session, and opening still works", async () => {
    const onKill = vi.fn();
    const onOpen = vi.fn();
    render(
      <SubagentItem
        node={{ agent: "worker", session: "sess-child", status: "running" }}
        onOpen={onOpen}
        onKill={onKill}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /Stop agent worker/ }));
    expect(onKill).toHaveBeenCalledWith("sess-child");
    expect(onOpen).not.toHaveBeenCalled();
  });

  it("does not offer to stop a subagent that already finished", () => {
    render(
      <SubagentItem
        node={{ agent: "worker", session: "sess-child", status: "completed" }}
        onOpen={vi.fn()}
        onKill={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: /Stop agent/ })).toBeNull();
  });
});
