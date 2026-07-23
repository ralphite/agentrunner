// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Markdown } from "./Markdown";
import { useStore } from "../store";

// The mock stands in for the lazy chunk: render succeeds unless the source
// says otherwise, so both the diagram path and the code-block fallback are
// covered without pulling real mermaid into jsdom.
const renderMock = vi.fn(async (_id: string, src: string) => {
  if (src?.includes("bad-source")) throw new Error("parse error");
  return { svg: '<svg role="img" aria-label="mmd-ok"><text>mmd-ok</text></svg>' };
});
vi.mock("mermaid", () => ({
  default: { initialize: vi.fn(), render: (id: string, src: string) => renderMock(id, src) },
}));

describe("Markdown mermaid blocks", () => {
  beforeEach(() => renderMock.mockClear());
  afterEach(() => {
    cleanup();
    useStore.setState({ theme: "system" });
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("renders a mermaid fence as a diagram once the lazy chunk resolves", async () => {
    render(<Markdown sid="s" text={"```mermaid\ngraph TD; A-->B\n```"} />);
    await waitFor(() => expect(screen.getByText("mmd-ok")).toBeTruthy());
    expect(renderMock).toHaveBeenCalledWith(expect.stringMatching(/^ar-mmd-/), "graph TD; A-->B");
    expect(screen.queryByText("graph TD; A-->B")).toBeNull();
  });

  it("falls back to the plain code block when mermaid cannot render", async () => {
    render(<Markdown sid="s" text={"```mermaid\nbad-source\n```"} />);
    await waitFor(() => expect(renderMock).toHaveBeenCalled());
    expect(screen.getByText("bad-source")).toBeTruthy();
    expect(screen.queryByText("mmd-ok")).toBeNull();
  });

  it("never touches mermaid for ordinary code fences", async () => {
    renderMock.mockClear();
    render(<Markdown sid="s" text={"```go\nfmt.Println(1)\n```"} />);
    await new Promise((r) => setTimeout(r, 10));
    expect(renderMock).not.toHaveBeenCalled();
  });

  it("re-renders an already mounted diagram when the explicit theme changes", async () => {
    useStore.setState({ theme: "dark" });
    render(<Markdown sid="s" text={"```mermaid\ngraph TD; A-->B\n```"} />);
    await waitFor(() => expect(renderMock).toHaveBeenCalledTimes(1));
    const mermaid = (await import("mermaid")).default;
    expect(mermaid.initialize).toHaveBeenLastCalledWith(expect.objectContaining({ theme: "dark" }));

    useStore.setState({ theme: "light" });
    await waitFor(() => expect(renderMock).toHaveBeenCalledTimes(2));
    expect(mermaid.initialize).toHaveBeenLastCalledWith(expect.objectContaining({ theme: "default" }));
  });

  it("re-renders a system-theme diagram when the OS media query changes", async () => {
    let listener: ((event: MediaQueryListEvent) => void) | undefined;
    const media = {
      matches: false,
      media: "(prefers-color-scheme: dark)",
      onchange: null,
      addEventListener: vi.fn((_type: string, fn: (event: MediaQueryListEvent) => void) => {
        listener = fn;
      }),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    } as unknown as MediaQueryList;
    vi.stubGlobal("matchMedia", vi.fn(() => media));
    useStore.setState({ theme: "system" });
    render(<Markdown sid="s" text={"```mermaid\ngraph TD; A-->B\n```"} />);
    await waitFor(() => expect(renderMock).toHaveBeenCalledTimes(1));

    listener?.({ matches: true } as MediaQueryListEvent);
    await waitFor(() => expect(renderMock).toHaveBeenCalledTimes(2));
    const mermaid = (await import("mermaid")).default;
    expect(mermaid.initialize).toHaveBeenLastCalledWith(expect.objectContaining({ theme: "dark" }));
  });
});
