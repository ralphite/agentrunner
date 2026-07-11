import { useState, type ReactNode } from "react";
import { ArrowsHorizontal, Check, Copy, TextAlignLeft } from "@phosphor-icons/react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { copyText } from "../clipboard";
import { rehypeHighlight } from "./highlight";

// Markdown renders assistant/runtime message bodies with react-markdown. It
// replaces the earlier hand-rolled parser (which had no tables, no syntax
// highlighting, no line-wrap control) while keeping the same public surface:
// <Markdown text={…} />. remark-gfm adds GitHub-flavoured tables, task lists,
// strikethrough and autolinks; rehypeHighlight (./highlight) adds highlight.js
// syntax colouring for a registered language subset.
//
// SECURITY (red line): react-markdown escapes raw HTML by default — we do NOT
// add rehype-raw. Any `<script>`/`<img onerror=…>` in the text is rendered as
// literal characters, never as live DOM, so there is no HTML-injection surface.

// CodeBlock renders a fenced block Codex-style: a slim header bar carrying the
// language label (left) and Wrap + Copy controls (right), then the highlighted
// code body. `raw` is the verbatim source used for copying; `children` are the
// already-highlighted <span> nodes from rehypeHighlight. The Wrap toggle flips
// the body between horizontal scroll (default) and soft-wrapping long lines.
function CodeBlock({ raw, lang, className, children }: { raw: string; lang?: string; className?: string; children: ReactNode }) {
  const [copied, setCopied] = useState(false);
  const [wrap, setWrap] = useState(false);
  const copy = async () => {
    await copyText(raw);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  };
  return (
    <div className="my-[2px] border border-line rounded-[12px] overflow-hidden bg-panel-2 max-w-full">
      <div className="flex items-center justify-between pt-[5px] pr-[6px] pb-[5px] pl-[12px] border-b border-line bg-panel">
        <span className="text-[11px] text-dim lowercase tracking-[0.02em] font-mono">{lang || "text"}</span>
        <div className="flex items-center gap-[2px]">
          <button
            className="inline-flex items-center gap-[4px] border border-transparent bg-transparent text-dim text-[11px] px-[8px] py-[3px] rounded-[6px] cursor-pointer transition-colors duration-[120ms] hover:bg-panel-2 hover:text-ink hover:border-line aria-pressed:text-ink aria-pressed:border-line"
            onClick={() => setWrap((w) => !w)}
            title={wrap ? "Disable line wrap" : "Wrap long lines"}
            aria-pressed={wrap}
            type="button"
          >
            {wrap ? <TextAlignLeft size={12} /> : <ArrowsHorizontal size={12} />} Wrap
          </button>
          <button
            className="inline-flex items-center gap-[4px] border border-transparent bg-transparent text-dim text-[11px] px-[8px] py-[3px] rounded-[6px] cursor-pointer transition-colors duration-[120ms] hover:bg-panel-2 hover:text-ink hover:border-line"
            onClick={copy}
            title="Copy code"
            type="button"
          >
            {copied ? (
              <>
                <Check size={12} /> Copied
              </>
            ) : (
              <>
                <Copy size={12} /> Copy
              </>
            )}
          </button>
        </div>
      </div>
      <pre className={"md-hljs m-0 px-[12px] py-[10px] font-mono text-[12.5px] leading-[1.5] text-ink" + (wrap ? " whitespace-pre-wrap break-words" : " whitespace-pre overflow-x-auto")}>
        <code className={className}>{children}</code>
      </pre>
    </div>
  );
}

// A fenced code block always arrives as <pre><code>…</code></pre>; we intercept
// `pre` (the only place block code appears) so no-language blocks are handled
// too, and read the child <code>'s language + highlighted content from it.
function preRenderer(props: { children?: ReactNode; node?: unknown }): ReactNode {
  const codeEl = Array.isArray(props.children) ? props.children[0] : props.children;
  const cp = (codeEl && typeof codeEl === "object" && "props" in codeEl ? (codeEl as { props: Record<string, unknown> }).props : {}) as {
    className?: string;
    children?: ReactNode;
  };
  const cls = cp.className || "";
  const lang = (/language-([\w-]+)/.exec(cls) || [])[1];
  // Raw source for the Copy button: walk the hast <pre> node's text so we copy
  // the verbatim code, not the tokenised spans.
  const raw = hastText(props.node).replace(/\n$/, "");
  return (
    <CodeBlock raw={raw} lang={lang} className={cls}>
      {cp.children}
    </CodeBlock>
  );
}

// hastText collects the concatenated text of a hast node subtree.
function hastText(node: unknown): string {
  if (!node || typeof node !== "object") return "";
  const n = node as { type?: string; value?: string; children?: unknown[] };
  if (n.type === "text") return n.value || "";
  if (Array.isArray(n.children)) return n.children.map(hastText).join("");
  return "";
}

const components: Components = {
  // Block chrome (header, copy, wrap) is owned by CodeBlock; `pre` becomes just
  // the mount point. Inline code falls through to the default <code> so the
  // existing `.md code` chip styling applies.
  pre: preRenderer,
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noreferrer">
      {children}
    </a>
  ),
  table: ({ children }) => (
    <div className="overflow-x-auto border border-line rounded-[8px] max-w-full">
      <table className="cx-table">{children}</table>
    </div>
  ),
  // Reuse the existing markdown class names so styles.css / styles.conv.css keep
  // styling these elements exactly as before (visual parity with the old parser).
  p: ({ children }) => <p className="md-p">{children}</p>,
  h1: ({ children }) => <div className="md-h md-h1">{children}</div>,
  h2: ({ children }) => <div className="md-h md-h2">{children}</div>,
  h3: ({ children }) => <div className="md-h md-h3">{children}</div>,
  h4: ({ children }) => <div className="md-h md-h4">{children}</div>,
  h5: ({ children }) => <div className="md-h md-h5">{children}</div>,
  h6: ({ children }) => <div className="md-h md-h6">{children}</div>,
  ul: ({ children }) => <ul className="md-list">{children}</ul>,
  ol: ({ children }) => <ol className="md-list">{children}</ol>,
  blockquote: ({ children }) => <blockquote className="md-quote">{children}</blockquote>,
};

export function Markdown({ text }: { text: string }) {
  return (
    <div className="md cx-md">
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  );
}
