import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ArrowsHorizontal, Check, Copy, ImageBroken, TextAlignLeft } from "@phosphor-icons/react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { AR } from "../api";
import { copyText } from "../clipboard";
import { useStore } from "../store";
import { Lightbox } from "./Lightbox";
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

// ─── Inline images (INC-41 RT-1) ────────────────────────────────────────────
// An assistant that produces a screenshot writes it into the workspace and then
// references it from its answer as `![shot](qa/shot.png)`. Without an `img`
// renderer that relative path resolves against the SPA origin and 404s (broken
// image icon), so the picture the turn was *about* never showed. We resolve
// workspace-relative sources through the session file endpoint (the same one the
// document artifact cards use — no new backend surface), constrain the image to
// the prose column, and open the full-size view in the shared Lightbox on click.

// External sources are passed through verbatim: a real URL, an inline data: URI,
// a blob:, or an already-built /api/… path (e.g. an upload preview) needs no
// workspace resolution. Everything else is treated as a path inside the
// session's workspace.
function isExternalSrc(src: string): boolean {
  return /^(https?:|data:|blob:)/i.test(src) || src.startsWith("/api/");
}

// resolveSrc maps a markdown image source to something the browser can fetch.
// A leading "./" or "/" is stripped so both `![a](./x.png)` and `![a](/x.png)`
// mean the same workspace-relative file (the file endpoint rejects absolute host
// paths outright, so a bare leading slash could only ever have been a 400).
export function resolveSrc(sid: string, src: string): string {
  if (isExternalSrc(src)) return src;
  const rel = src.replace(/^\.?\/+/, "");
  if (!sid || !rel) return src;
  return AR.fileURL(sid, rel);
}

const basename = (p: string) => (p.split(/[?#]/)[0].split("/").pop() || p);

// ─── Inlined-image registry (INC-41 TH-9) ───────────────────────────────────
// The same screenshot used to be painted THREE times in one screen: once inline
// in the answer (here), once as an artifact thumbnail card, and once more as a
// grey filename row inside "Edited N files" — ~600px of a 723px thread viewport
// spent re-showing one picture. Codex shows a produced artifact exactly once.
//
// The answer's prose is the authoritative place for an image the agent chose to
// show, so the inline renderer wins and the thumbnail card stands down. The
// artifact card lives in a sibling subtree (SessionView mounts ChangesOutcome
// next to the Timeline, not under it), so there is no prop path between them:
// mounted inline images register here, and ChangesOutcome subscribes.
//
// Ref-counted, because a streaming answer re-renders and the same path can be
// mounted by more than one message — a path counts as "already inlined" only
// while at least one <img.md-img> for it is actually on screen.
const inlineCounts = new Map<string, Map<string, number>>(); // sid → path → mounted count
const inlineListeners = new Set<() => void>();
let inlineVersion = 0;

// workspacePath normalizes a markdown image source to the workspace-relative
// path the diff summary keys files by ("./qa/shot.png" and "/qa/shot.png" and
// "qa/shot.png" are one file). External sources (http/data/blob/already-built
// /api/…) are not workspace files and never match a changed file → null.
export function workspacePath(src: string): string | null {
  if (!src || isExternalSrc(src)) return null;
  const rel = src.split(/[?#]/)[0].replace(/^\.?\/+/, "");
  return rel || null;
}

function registerInline(sid: string, src: string): () => void {
  const rel = workspacePath(src);
  if (!sid || !rel) return () => {};
  let paths = inlineCounts.get(sid);
  if (!paths) {
    paths = new Map();
    inlineCounts.set(sid, paths);
  }
  paths.set(rel, (paths.get(rel) || 0) + 1);
  inlineVersion++;
  inlineListeners.forEach((fn) => fn());
  return () => {
    const live = inlineCounts.get(sid);
    if (!live) return;
    const n = (live.get(rel) || 0) - 1;
    if (n > 0) live.set(rel, n);
    else live.delete(rel);
    if (!live.size) inlineCounts.delete(sid);
    inlineVersion++;
    inlineListeners.forEach((fn) => fn());
  };
}

// useSyncExternalStore's three parts, exported for ChangesOutcome.
export function subscribeInlinedImages(fn: () => void): () => void {
  inlineListeners.add(fn);
  return () => {
    inlineListeners.delete(fn);
  };
}

export function inlinedImagesVersion(): number {
  return inlineVersion;
}

// inlinedImagePaths: workspace-relative paths this session currently shows
// inline in its answers.
export function inlinedImagePaths(sid: string): Set<string> {
  return new Set(inlineCounts.get(sid)?.keys() ?? []);
}

// MdImage is one inline image. A load failure degrades to a single filename
// link rather than leaving a broken-image glyph in the middle of the answer —
// the agent may have referenced a path it never actually wrote, or the file may
// have been moved since the turn ended, and the reader deserves to see which.
function MdImage({ sid, src, alt, onOpen }: { sid: string; src: string; alt: string; onOpen: (src: string) => void }) {
  const [failed, setFailed] = useState(false);
  const url = resolveSrc(sid, src);
  // While this image is mounted, the turn's artifact row must not repeat it
  // (TH-9). A failed load still counts: the fallback link below names the file,
  // so a thumbnail card of the same broken file would add nothing but noise.
  useEffect(() => registerInline(sid, src), [sid, src]);
  if (failed)
    return (
      <a className="md-img-fallback" href={url} target="_blank" rel="noreferrer" title={src}>
        <ImageBroken size={13} /> {alt || basename(src)}
      </a>
    );
  return (
    <img
      className="md-img"
      src={url}
      data-src={src}
      alt={alt}
      title={alt || src}
      loading="lazy"
      onError={() => setFailed(true)}
      onClick={() => onOpen(src)}
    />
  );
}

// A paragraph whose entire content is images renders as an image row instead of
// prose: one image stays single-column, several lay out as a wrapping grid
// (Codex renders multi-screenshot answers the same way).
function imageOnlyParagraph(node: unknown): boolean {
  const kids = (node as { children?: { type?: string; tagName?: string; value?: string }[] } | undefined)?.children;
  if (!Array.isArray(kids)) return false;
  const meaningful = kids.filter((c) => !(c.type === "text" && !(c.value || "").trim()));
  return meaningful.length > 0 && meaningful.every((c) => c.type === "element" && c.tagName === "img");
}

const baseComponents: Components = {
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
  p: ({ children, node }) =>
    imageOnlyParagraph(node) ? <div className="md-img-grid">{children}</div> : <p className="md-p">{children}</p>,
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

// `sid` defaults to the session the thread belongs to (the router's current
// session) so callers keep the original <Markdown text={…} /> surface; it stays
// overridable for tests and for any future off-thread render.
export function Markdown({ text, sid }: { text: string; sid?: string }) {
  const currentSid = useStore((s) => s.currentSid);
  const ctxSid = sid ?? currentSid ?? "";
  const root = useRef<HTMLDivElement>(null);
  // The lightbox group is the images actually rendered in this answer, read off
  // the DOM at click time (document order == reading order). That avoids a
  // registration dance between the img renderer and this component, and it can
  // never drift out of sync with what the reader is looking at.
  const [group, setGroup] = useState<{ srcs: string[]; index: number } | null>(null);
  const open = (src: string) => {
    const imgs = Array.from(root.current?.querySelectorAll<HTMLImageElement>("img.md-img") || []);
    const srcs = imgs.map((el) => el.dataset.src || el.src);
    const index = Math.max(0, srcs.indexOf(src));
    setGroup({ srcs: srcs.length ? srcs : [src], index });
  };

  const components = useMemo<Components>(
    () => ({
      ...baseComponents,
      img: ({ src, alt }) => <MdImage sid={ctxSid} src={typeof src === "string" ? src : ""} alt={alt || ""} onOpen={open} />,
    }),
    // `open` closes over refs/setState only — stable across renders.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [ctxSid],
  );

  return (
    <div className="md cx-md" ref={root}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={components}>
        {text}
      </ReactMarkdown>
      {group && (
        <Lightbox
          images={group.srcs}
          index={group.index}
          resolve={(s) => resolveSrc(ctxSid, s)}
          onIndex={(i) => setGroup((g) => (g ? { ...g, index: i } : g))}
          onClose={() => setGroup(null)}
        />
      )}
    </div>
  );
}
