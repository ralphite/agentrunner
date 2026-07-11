// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { Markdown } from "./Markdown";

// INC-51: react-markdown + remark-gfm + rehypeHighlight replaces the old
// hand-rolled renderer. These cover the four things the swap adds/guards:
// GFM tables, highlight.js token spans, the line-wrap toggle, and — the safety
// red line — raw HTML staying escaped (no rehype-raw, no injection surface).
afterEach(cleanup);

describe("Markdown (INC-51)", () => {
  it("renders GFM pipe tables into a bordered .cx-table", () => {
    const { container } = render(<Markdown text={"| Head A | Head B |\n|---|---|\n| r1c1 | r1c2 |\n"} />);
    expect(container.querySelector("table.cx-table")).not.toBeNull();
    expect(Array.from(container.querySelectorAll("th")).map((th) => th.textContent)).toEqual(["Head A", "Head B"]);
    expect(Array.from(container.querySelectorAll("td")).map((td) => td.textContent)).toEqual(["r1c1", "r1c2"]);
  });

  it("syntax-highlights fenced code with highlight.js token spans", () => {
    const { container } = render(<Markdown text={"```js\nconst x = 1;\n```\n"} />);
    const code = container.querySelector("pre.md-hljs code.hljs");
    expect(code).not.toBeNull();
    expect(code?.className).toContain("language-js");
    expect(container.querySelector(".hljs-keyword")?.textContent).toBe("const");
    expect(container.querySelector(".hljs-number")?.textContent).toBe("1");
    // language label surfaces in the header bar
    expect(container.querySelector(".lowercase")?.textContent).toBe("js");
  });

  it("leaves an unregistered language unhighlighted but still a block", () => {
    const { container } = render(<Markdown text={"```wat\nnope\n```\n"} />);
    const pre = container.querySelector("pre.md-hljs");
    expect(pre).not.toBeNull();
    expect(pre?.textContent).toContain("nope");
    expect(container.querySelector(".hljs-keyword")).toBeNull();
  });

  it("toggles line-wrap on the code body (default scroll ↔ wrap)", () => {
    const { container, getByTitle } = render(<Markdown text={"```text\na very long line\n```\n"} />);
    const pre = () => container.querySelector("pre.md-hljs") as HTMLElement;
    // default: horizontal scroll, not wrapped
    expect(pre().className).toContain("overflow-x-auto");
    expect(pre().className).not.toContain("whitespace-pre-wrap");
    fireEvent.click(getByTitle("Wrap long lines"));
    expect(pre().className).toContain("whitespace-pre-wrap");
    expect(pre().className).toContain("break-words");
    // toggling back restores horizontal scroll
    fireEvent.click(getByTitle("Disable line wrap"));
    expect(pre().className).toContain("overflow-x-auto");
    expect(pre().className).not.toContain("whitespace-pre-wrap");
  });

  it("escapes raw HTML — no injection surface (security red line)", () => {
    const { container } = render(<Markdown text={'<img src=x onerror="alert(1)"> <script>alert(2)</script>\n\nsafe **bold**'} />);
    // dangerous constructs never become live DOM …
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("script")).toBeNull();
    expect(Array.from(container.querySelectorAll("*")).some((el) => el.hasAttribute("onerror"))).toBe(false);
    // no live tags leaked into markup — they were escaped, not parsed
    expect(container.innerHTML).not.toContain("<img");
    expect(container.innerHTML).not.toContain("<script");
    // … they survive only as escaped text, and legit markdown still renders
    expect(container.textContent).toContain("<script>alert(2)</script>");
    expect(container.querySelector("strong")?.textContent).toBe("bold");
  });
});
