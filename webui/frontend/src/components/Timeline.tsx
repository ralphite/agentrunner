import { useEffect, useRef } from "react";
import type { TimelineItem, ToolItem } from "../timeline";

function AssistantText({ text }: { text: string }) {
  // Minimal markdown: split fenced ``` code blocks from prose.
  const parts = text.split(/(```[\s\S]*?```)/g);
  return (
    <>
      {parts.map((p, i) => {
        if (p.startsWith("```")) {
          const body = p.replace(/^```[^\n]*\n?/, "").replace(/```$/, "");
          return (
            <pre className="code" key={i}>
              {body}
            </pre>
          );
        }
        return <span key={i}>{p}</span>;
      })}
    </>
  );
}

function ToolCard({ t }: { t: ToolItem }) {
  const badgeCls =
    t.status === "done"
      ? "ok"
      : t.status === "error" || t.status === "failed"
        ? "err"
        : t.status === "running"
          ? "run"
          : "";
  return (
    <div className={"tool " + (t.status === "error" || t.status === "failed" ? "error" : t.status === "cancelled" ? "cancelled" : "")}>
      <div className="head">
        <span className="name">🔧 {t.name}</span>
        {t.background && <span className="badge">task</span>}
        <span className={"badge " + badgeCls}>{t.statusText}</span>
        {t.usage && (
          <span className="badge" title="tokens">
            {t.usage.input_tokens}/{t.usage.output_tokens} tok
          </span>
        )}
      </div>
      {t.args !== undefined && t.args !== null && (
        <details>
          <summary>参数</summary>
          <pre>{pretty(t.args)}</pre>
        </details>
      )}
      {t.result !== undefined && (
        <details>
          <summary>结果</summary>
          <pre>{pretty(t.result).slice(0, 20000)}</pre>
        </details>
      )}
      {t.errorMsg && <pre>{t.errorMsg}</pre>}
      {t.partial && (
        <details>
          <summary>部分输出</summary>
          <pre>{t.partial}</pre>
        </details>
      )}
    </div>
  );
}

function pretty(raw: any): string {
  if (raw == null) return "";
  try {
    return JSON.stringify(typeof raw === "string" ? JSON.parse(raw) : raw, null, 2);
  } catch {
    return String(raw);
  }
}

function Item({ it }: { it: TimelineItem }) {
  switch (it.kind) {
    case "turn":
      return <div className="turn">第 {it.gen} 轮</div>;
    case "user":
      return (
        <div className="msg user">
          <div className="bubble">
            {it.text}
            {it.images ? <div className="imgnote">📷 ×{it.images} (CAS ref)</div> : null}
          </div>
          <span className="who">{it.source || "你"}</span>
        </div>
      );
    case "assistant":
      return (
        <div className="msg assistant">
          <div className="avatar a">◆</div>
          <div className="bubble">
            <AssistantText text={it.text} />
          </div>
        </div>
      );
    case "tool":
      return <ToolCard t={it} />;
    case "chip":
      return (
        <div className={"chip " + it.tone}>
          <span>{it.text}</span>
          {it.childSession && (
            <a href={"#" + it.childSession}>打开子会话 ↗</a>
          )}
        </div>
      );
    case "sys":
      return <div className="sys">{it.text}</div>;
  }
}

export function TimelineView({
  items,
  pending,
  typing,
  showSys,
}: {
  items: TimelineItem[];
  pending: { id: number; text: string; images: number }[];
  typing: string;
  showSys: boolean;
}) {
  const visible = showSys ? items : items.filter((it) => it.kind !== "sys");
  const ref = useRef<HTMLDivElement>(null);
  const stick = useRef(true);

  useEffect(() => {
    const el = ref.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  });

  const onScroll = () => {
    const el = ref.current;
    if (!el) return;
    stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  };

  return (
    <div className="timeline" ref={ref} onScroll={onScroll}>
      <div className="tl-inner">
        {visible.map((it) => (
          <Item key={it.key} it={it} />
        ))}
        {typing && (
          <div className="msg assistant">
            <div className="avatar a">◆</div>
            <div className="bubble typing">{typing}</div>
          </div>
        )}
        {pending.map((p) => (
          <div className="msg user" key={"p" + p.id}>
            <div className="bubble pending">
              {p.text}
              {p.images ? <div className="imgnote">📷 ×{p.images}</div> : null}
            </div>
            <span className="who">排队中…</span>
          </div>
        ))}
      </div>
    </div>
  );
}
