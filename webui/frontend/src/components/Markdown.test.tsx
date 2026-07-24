// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { Markdown, resolveSrc } from "./Markdown";

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

  it("keeps header actions visible for a long language name without wrap points", () => {
    const lang = "supercalifragilisticexpialidociouslanguageidentifierabcdefghijklmnopqrstuvwxyz0123456789";
    const { container, getByTitle } = render(<Markdown text={`\`\`\`${lang}\nconst value = 1;\n\`\`\`\n`} />);
    const pre = container.querySelector("pre.md-hljs") as HTMLElement;
    const header = pre.previousElementSibling as HTMLElement;
    const label = header.querySelector("span") as HTMLElement;
    const actions = getByTitle("Copy code").parentElement as HTMLElement;

    expect(label.textContent).toBe(lang);
    expect(header.className).toContain("min-w-0");
    expect(label.className).toContain("min-w-0");
    expect(label.className).toContain("flex-1");
    expect(label.className).toContain("truncate");
    expect(label.title).toBe(lang);
    expect(actions.className).toContain("shrink-0");
    expect(actions.contains(getByTitle("Wrap long lines"))).toBe(true);
  });

  it("renders inline and display math with KaTeX without exposing delimiters", () => {
    const { container } = render(<Markdown text={"Inline $E = mc^2$.\n\n$$\n\\int_0^1 x^2 dx = \\frac{1}{3}\n$$"} />);
    expect(container.querySelector(".katex")).not.toBeNull();
    expect(container.querySelector(".katex-display")).not.toBeNull();
    expect(container.textContent).not.toContain("$E = mc^2$");
    expect(container.textContent).not.toContain("$$");
    expect(container.querySelector(".katex-mathml math")).not.toBeNull();
  });

  it("copies the exact raw fenced-code text", async () => {
    const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });

    try {
      const raw = 'const sentinel = "COPY_WRAP_SCROLL_SENTINEL";';
      const { getByTitle } = render(<Markdown text={`\`\`\`js\n${raw}\n\`\`\`\n`} />);
      fireEvent.click(getByTitle("Copy code"));

      await waitFor(() => expect(writeText).toHaveBeenCalledWith(raw));
      expect(getByTitle("Copy code").textContent).toContain("Copied");
    } finally {
      if (originalClipboard) Object.defineProperty(navigator, "clipboard", originalClipboard);
      else delete (navigator as unknown as { clipboard?: Clipboard }).clipboard;
    }
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

// INC-41 RT-1: an assistant that writes a screenshot into the workspace and then
// references it (`![shot](qa/shot.png)`) must see it inline in the thread — the
// relative path has to be resolved through the session-file endpoint, not left
// to 404 against the SPA origin.
describe("Markdown inline images (INC-41 RT-1)", () => {
  const SID = "s-abc";

  it("resolves a workspace-relative source through the session file endpoint", () => {
    const { container } = render(<Markdown sid={SID} text={"![shot](qa/runs/shot.png)"} />);
    const img = container.querySelector("img.md-img") as HTMLImageElement;
    expect(img).not.toBeNull();
    expect(img.getAttribute("src")).toBe("/api/sessions/s-abc/file?path=qa%2Fruns%2Fshot.png");
    expect(img.getAttribute("alt")).toBe("shot");
  });

  it("normalises ./ and / prefixed sources to the same workspace file", () => {
    expect(resolveSrc(SID, "./a.png")).toBe(resolveSrc(SID, "a.png"));
    expect(resolveSrc(SID, "/a.png")).toBe(resolveSrc(SID, "a.png"));
  });

  it("passes absolute/data/api sources through untouched", () => {
    for (const src of ["https://example.com/a.png", "http://example.com/a.png", "data:image/png;base64,AAA", "/api/uploads/x.png"]) {
      expect(resolveSrc(SID, src)).toBe(src);
    }
  });

  it("lays an image-only paragraph out as a grid, and keeps prose as a paragraph", () => {
    const { container } = render(<Markdown sid={SID} text={"![a](a.png)\n![b](b.png)\n\ntext ![c](c.png) more"} />);
    const grid = container.querySelector(".md-img-grid");
    expect(grid).not.toBeNull();
    expect(grid?.querySelectorAll("img.md-img").length).toBe(2);
    // the mixed prose+image paragraph stays a paragraph (image still rendered)
    const p = container.querySelector("p.md-p");
    expect(p?.querySelector("img.md-img")).not.toBeNull();
    expect(p?.textContent).toContain("more");
  });

  it("opens the lightbox on click, with the whole answer's images as the group", () => {
    const { container } = render(<Markdown sid={SID} text={"![a](a.png)\n![b](b.png)"} />);
    const imgs = container.querySelectorAll("img.md-img");
    fireEvent.click(imgs[1]);
    const lb = document.querySelector(".lightbox");
    expect(lb).not.toBeNull();
    // group of two → counter shows the clicked one as #2
    expect(lb?.querySelector(".lb-count")?.textContent).toBe("2 / 2");
    expect((lb?.querySelector("img.lb-img") as HTMLImageElement).src).toContain("path=b.png");
    // Real keyboard input originates at the focused element and propagates
    // through document, where FocusScope captures Escape. Dispatching directly
    // on window skips that real browser path.
    expect(document.activeElement).toBe(lb);
    fireEvent.keyDown(document.activeElement as HTMLElement, { key: "Escape" });
    expect(document.querySelector(".lightbox")).toBeNull();
  });

  it("degrades a failed load to a filename link — no broken image glyph", () => {
    const { container } = render(<Markdown sid={SID} text={"![](qa/missing.png)"} />);
    const img = container.querySelector("img.md-img") as HTMLImageElement;
    fireEvent.error(img);
    expect(container.querySelector("img.md-img")).toBeNull();
    const link = container.querySelector("a.md-img-fallback") as HTMLAnchorElement;
    expect(link).not.toBeNull();
    expect(link.textContent).toContain("missing.png");
    expect(link.getAttribute("href")).toBe("/api/sessions/s-abc/file?path=qa%2Fmissing.png");
  });
});
