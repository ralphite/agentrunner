// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  switchAgent: vi.fn(async () => ({})),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    switchAgent: mocks.switchAgent,
  },
}));

import { useStore } from "../store";
import { Modals } from "./Modals";
import { recallSpec, rememberSpec } from "./sessionSpecs";

beforeEach(() => {
  mocks.switchAgent.mockClear();
});

afterEach(() => {
  cleanup();
  useStore.setState({ modal: null });
});

describe("session agent YAML editor", () => {
  it("round-trips the session's remembered spec instead of resetting to Dev", async () => {
    const sid = "inc96-agent-editor";
    const current = "name: custom\nmodel: { provider: gemini, id: gemini-flash-latest }\nsystem_prompt: Keep me.\ntools: []\n";
    const changed = current.replace("Keep me.", "Keep my edit.");
    rememberSpec(sid, current);
    useStore.setState({ modal: { kind: "agent", sid } });

    const { container } = render(<Modals />);
    const specEditor = container.querySelectorAll<HTMLTextAreaElement>("textarea")[0];
    expect(specEditor.value).toBe(current);

    fireEvent.change(specEditor, { target: { value: changed } });
    fireEvent.click(screen.getByRole("button", { name: "Switch" }));

    await waitFor(() => expect(mocks.switchAgent).toHaveBeenCalledOnce());
    expect(mocks.switchAgent).toHaveBeenCalledWith(
      sid,
      changed,
      expect.arrayContaining([expect.objectContaining({ name: "worker.yaml" })]),
    );
    expect(recallSpec(sid)).toBe(changed);
  });
});
