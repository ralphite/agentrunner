// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useStore } from "../store";
import { SettingsConfiguration } from "./SettingsConfiguration";

describe("SettingsConfiguration mobile layout", () => {
  beforeEach(() => {
    useStore.setState({
      health: {
        version: "agentrunner-development-version-with-a-long-suffix",
        daemonUp: true,
        daemonManaged: false,
        daemonExternal: true,
        manageRequested: false,
        runtimeDir: "/Users/example/.local/share/agentrunner/runtime-with-a-long-unbroken-segment",
        daemonLogPath: "/Users/example/.local/share/agentrunner/log/agentrunner-daemon.log",
        sandboxBackend: "seatbelt",
        sandboxDetected: true,
      },
    });
  });

  afterEach(cleanup);

  it("stacks keys above selectable, wrapping values and separates policy copy", () => {
    render(<SettingsConfiguration query="" />);

    const version = screen.getByText("agentrunner-development-version-with-a-long-suffix");
    expect(version.parentElement?.className).toContain("flex-col");
    expect(version.className).toContain("select-text");
    expect(version.className).toContain("[overflow-wrap:anywhere]");

    const policy = screen.getByText("Approval policy & sandbox");
    expect(policy.parentElement?.className).toContain("gap-x-2");
    expect(screen.getByText(/Approval mode is per-session/).className).toContain("mt-1.5");
  });

  it("surfaces the real sandbox backend state instead of a todo", () => {
    render(<SettingsConfiguration query="" />);
    expect(screen.getByText(/seatbelt — detected/)).toBeTruthy();
    expect(screen.queryByText("Not surfaced")).toBeNull();
  });

  it("states fail-closed when the backend is missing", () => {
    useStore.setState((s) => ({
      health: { ...s.health!, sandboxBackend: "bubblewrap", sandboxDetected: false },
    }));
    render(<SettingsConfiguration query="" />);
    expect(screen.getByText(/bubblewrap — not detected; execute-class commands fail closed/)).toBeTruthy();
  });
});
