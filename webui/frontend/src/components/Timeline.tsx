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

// toolLabel turns a raw tool call into a Codex-style step line:
// "$ ls -la", "read notes.txt", "edit main.go".
function toolLabel(name: string, args: any): { verb: string; body: string; mono: boolean } {
  let a: any = args;
  if (typeof args === "string") {
    try {
      a = JSON.parse(args);
    } catch {
      a = {};
    }
  }
  a = a || {};
  switch (name) {
    case "bash":
      return { verb: "$", body: a.command || "", mono: true };
    case "read_file":
      return { verb: "read", body: a.path || a.file || "", mono: true };
    case "write_file":
      return { verb: "write", body: a.path || a.file || "", mono: true };
    case "edit_file":
      return { verb: "edit", body: a.path || a.file || "", mono: true };
    case "spawn_agent":
      return { verb: "spawn sub-agent", body: a.agent || a.task || "", mono: false };
    case "task_kill":
      return { verb: "kill task", body: a.handle || "", mono: true };
    default:
      return { verb: name, body: a.command || a.path || "", mono: true };
  }
}

function StepIcon({ status }: { status: ToolItem["status"] }) {
  if (status === "running") return <span className="step-ic spin" />;
  if (status === "done") return <span className="step-ic ok">✓</span>;
  if (status === "cancelled") return <span className="step-ic warn">◦</span>;
  return <span className="step-ic err">✕</span>;
}

function ToolCard({ t }: { t: ToolItem }) {
  const { verb, body, mono } = toolLabel(t.name, t.args);
  const hasDetail =
    t.result !== undefined || t.errorMsg || t.partial || (!body && t.args !== undefined);
  return (
    <details className={"step" + (t.status === "error" || t.status === "failed" ? " error" : "")}>
      <summary>
        <StepIcon status={t.status} />
        <span className="step-verb">{verb}</span>
        <span className={"step-body" + (mono ? " mono" : "")}>{body}</span>
        {t.background && <span className="step-tag">background</span>}
        {t.usage && (
          <span className="step-tok" title="tokens">
            {t.usage.input_tokens + t.usage.output_tokens} tok
          </span>
        )}
      </summary>
      {hasDetail && (
        <div className="step-detail">
          {!body && t.args !== undefined && <pre>{pretty(t.args)}</pre>}
          {t.result !== undefined && <pre>{pretty(t.result).slice(0, 20000)}</pre>}
          {t.errorMsg && <pre className="err">{t.errorMsg}</pre>}
          {t.partial && <pre>{t.partial}</pre>}
        </div>
      )}
    </details>
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
      return <div className="turn">turn {it.gen}</div>;
    case "user":
      return (
        <div className="msg user">
          <div className="bubble">
            {it.text}
            {it.images ? <div className="imgnote">📷 ×{it.images} (CAS ref)</div> : null}
          </div>
          <span className="who">{it.source || "you"}</span>
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
            <a href={"#" + it.childSession}>open sub-session ↗</a>
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
  // Codex shows a continuous activity feed — no "turn N" dividers, no raw
  // system events. Those stay behind the developer toggle.
  const visible = showSys ? items : items.filter((it) => it.kind !== "sys" && it.kind !== "turn");
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
            <span className="who">queued…</span>
          </div>
        ))}
      </div>
    </div>
  );
}
