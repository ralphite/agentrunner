// A bundle-conscious syntax highlighter for the Markdown renderer (INC-51).
//
// We deliberately do NOT use `rehype-highlight`: it statically imports
// lowlight's `common` grammar set (~35 languages) via `import {common} from
// "lowlight"`, and because it reads `settings.languages || common` that binding
// is live and cannot be tree-shaken — so the full common set ships whether or
// not it is used. Instead we build a lowlight instance off `createLowlight`
// (which runs on `highlight.js/lib/core`, zero languages) and register only the
// languages we care about. This keeps highlight.js to core + an explicit,
// auditable language subset (the "core, per-language" plan of HANDA-PARITY #20).
//
// Security: this only ever emits <span class="hljs-*"> nodes built from the
// tokenised source text — it never introduces raw HTML. The Markdown renderer
// keeps rehype-raw off, so there is no HTML-injection surface.
import { createLowlight } from "lowlight";
import { visit } from "unist-util-visit";
import type { Root, Element, ElementContent } from "hast";

import bash from "highlight.js/lib/languages/bash";
import c from "highlight.js/lib/languages/c";
import cpp from "highlight.js/lib/languages/cpp";
import csharp from "highlight.js/lib/languages/csharp";
import css from "highlight.js/lib/languages/css";
import diff from "highlight.js/lib/languages/diff";
import dockerfile from "highlight.js/lib/languages/dockerfile";
import go from "highlight.js/lib/languages/go";
import ini from "highlight.js/lib/languages/ini";
import java from "highlight.js/lib/languages/java";
import javascript from "highlight.js/lib/languages/javascript";
import json from "highlight.js/lib/languages/json";
import markdown from "highlight.js/lib/languages/markdown";
import python from "highlight.js/lib/languages/python";
import rust from "highlight.js/lib/languages/rust";
import sql from "highlight.js/lib/languages/sql";
import typescript from "highlight.js/lib/languages/typescript";
import xml from "highlight.js/lib/languages/xml";
import yaml from "highlight.js/lib/languages/yaml";

// Each highlight.js grammar carries its own aliases (js, ts, py, sh, html, yml,
// md, rs, …) which lowlight registers automatically. We only add the few that
// assistant fences use but hljs does not ship as aliases.
export const lowlight = createLowlight({
  bash,
  c,
  cpp,
  csharp,
  css,
  diff,
  dockerfile,
  go,
  ini,
  java,
  javascript,
  json,
  markdown,
  python,
  rust,
  sql,
  typescript,
  xml,
  yaml,
});
lowlight.registerAlias({
  typescript: ["tsx"],
  javascript: ["jsx"],
  bash: ["shell", "console", "sh"],
  markdown: ["md"],
  dockerfile: ["docker"],
});

function textOf(node: Element | ElementContent): string {
  if (node.type === "text") return node.value;
  if ("children" in node && node.children) return node.children.map(textOf).join("");
  return "";
}

const langOf = (node: Element): string | undefined => {
  const cls = node.properties?.className;
  const list = Array.isArray(cls) ? cls : cls ? [cls] : [];
  for (const c of list) {
    if (typeof c === "string" && c.startsWith("language-")) return c.slice("language-".length);
  }
  return undefined;
};

// rehypeHighlight: a minimal rehype transform that highlights fenced code
// blocks (`<pre><code class="language-x">`) with the registered subset. Unknown
// or missing languages are left untouched (rendered verbatim), never throwing.
export function rehypeHighlight() {
  return (tree: Root) => {
    visit(tree, "element", (node: Element, _index, parent) => {
      if (node.tagName !== "code") return;
      if (!parent || parent.type !== "element" || (parent as Element).tagName !== "pre") return;
      const lang = langOf(node);
      if (!lang || !lowlight.registered(lang)) return;
      let result: Root;
      try {
        result = lowlight.highlight(lang, textOf(node));
      } catch {
        return;
      }
      const props = (node.properties = node.properties || {});
      const cls = Array.isArray(props.className) ? props.className : props.className ? [String(props.className)] : [];
      if (!cls.includes("hljs")) cls.unshift("hljs");
      props.className = cls;
      if (result.children.length > 0) node.children = result.children as ElementContent[];
    });
  };
}
