import type { ReactNode } from "react";

// A small, dependency-free Markdown renderer for assistant messages — Codex
// renders rich markdown and plain <span> text looked flat next to it. Safe by
// construction: it builds React nodes from text (never dangerouslySetInnerHTML),
// so there is no HTML-injection surface. Covers the common cases: fenced code,
// headings, bullet/numbered lists, blockquotes, **bold**, *italic*, `code`,
// [links](url). Anything it doesn't recognise falls through as literal text.

function inline(text: string, keyBase = 0): ReactNode[] {
  const nodes: ReactNode[] = [];
  const re = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*]+\*)|(\[[^\]]+\]\([^)\s]+\))/g;
  let last = 0;
  let m: RegExpExecArray | null;
  let k = keyBase;
  while ((m = re.exec(text))) {
    if (m.index > last) nodes.push(text.slice(last, m.index));
    const tok = m[0];
    if (tok.startsWith("`")) nodes.push(<code key={k++}>{tok.slice(1, -1)}</code>);
    else if (tok.startsWith("**")) nodes.push(<strong key={k++}>{tok.slice(2, -2)}</strong>);
    else if (tok.startsWith("*")) nodes.push(<em key={k++}>{tok.slice(1, -1)}</em>);
    else {
      const lm = tok.match(/\[([^\]]+)\]\(([^)\s]+)\)/);
      if (lm)
        nodes.push(
          <a key={k++} href={lm[2]} target="_blank" rel="noreferrer">
            {lm[1]}
          </a>,
        );
      else nodes.push(tok);
    }
    last = re.lastIndex;
  }
  if (last < text.length) nodes.push(text.slice(last));
  return nodes;
}

function Blocks({ text }: { text: string }) {
  const lines = text.split("\n");
  const out: ReactNode[] = [];
  let i = 0;
  let key = 0;
  const isUl = (l: string) => /^\s*[-*]\s+/.test(l);
  const isOl = (l: string) => /^\s*\d+\.\s+/.test(l);
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
      continue;
    }
    const h = line.match(/^(#{1,6})\s+(.*)$/);
    if (h) {
      out.push(
        <div className={"md-h md-h" + h[1].length} key={key++}>
          {inline(h[2])}
        </div>,
      );
      i++;
      continue;
    }
    if (line.trim().startsWith(">")) {
      const quote: string[] = [];
      while (i < lines.length && lines[i].trim().startsWith(">")) {
        quote.push(lines[i].replace(/^\s*>\s?/, ""));
        i++;
      }
      out.push(
        <blockquote className="md-quote" key={key++}>
          {inline(quote.join(" "))}
        </blockquote>,
      );
      continue;
    }
    if (isUl(line) || isOl(line)) {
      const ordered = isOl(line);
      const items: ReactNode[] = [];
      while (i < lines.length && (isUl(lines[i]) || isOl(lines[i]))) {
        items.push(<li key={items.length}>{inline(lines[i].replace(/^\s*(?:[-*]|\d+\.)\s+/, ""))}</li>);
        i++;
      }
      out.push(
        ordered ? (
          <ol className="md-list" key={key++}>
            {items}
          </ol>
        ) : (
          <ul className="md-list" key={key++}>
            {items}
          </ul>
        ),
      );
      continue;
    }
    const para: string[] = [];
    while (i < lines.length && lines[i].trim() && !/^(#{1,6})\s/.test(lines[i]) && !isUl(lines[i]) && !isOl(lines[i]) && !lines[i].trim().startsWith(">")) {
      para.push(lines[i]);
      i++;
    }
    out.push(
      <p className="md-p" key={key++}>
        {inline(para.join(" "))}
      </p>,
    );
  }
  return <>{out}</>;
}

export function Markdown({ text }: { text: string }) {
  // Fenced code blocks are split out first so their contents are never parsed
  // as markdown.
  const segs = text.split(/(```[\s\S]*?```)/g);
  return (
    <div className="md">
      {segs.map((seg, i) => {
        if (seg.startsWith("```")) {
          const nl = seg.indexOf("\n");
          const lang = seg.slice(3, nl < 0 ? undefined : nl).trim();
          const body = nl < 0 ? "" : seg.slice(nl + 1).replace(/```\s*$/, "");
          return (
            <pre className="code md-code" key={i} data-lang={lang || undefined}>
              {body}
            </pre>
          );
        }
        return <Blocks key={i} text={seg} />;
      })}
    </div>
  );
}
