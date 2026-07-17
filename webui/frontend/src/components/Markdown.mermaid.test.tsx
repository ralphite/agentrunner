// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Markdown } from "./Markdown";

// The mock stands in for the lazy chunk: render succeeds unless the source
// says otherwise, so both the diagram path and the code-block fallback are
// covered without pulling real mermaid into jsdom.
const renderMock = vi.fn(async (_id: string, src: string) => {
  if (src.includes("bad-source")) throw new Error("parse error");
  return { svg: '<svg role="img" aria-label="mmd-ok"><text>mmd-ok</text></svg>' };
});
vi.mock("mermaid", () => ({
  default: { initialize: vi.fn(), render: (id: string, src: string) => renderMock(id, src) },
}));

describe("Markdown mermaid blocks", () => {
  afterEach(cleanup);

  it("renders a mermaid fence as a diagram once the lazy chunk resolves", async () => {
    render(<Markdown sid="s" text={"```mermaid\ngraph TD; A-->B\n```"} />);
    await waitFor(() => expect(screen.getByText("mmd-ok")).toBeTruthy());
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
});
