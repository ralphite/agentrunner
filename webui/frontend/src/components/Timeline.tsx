import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { ArrowSquareOut, Check, Circle, Copy, File, ImageSquare, Robot, X } from "@phosphor-icons/react";
import type { TimelineItem, ToolItem } from "../timeline";
import { Markdown } from "./Markdown";
import { copyText } from "../clipboard";
import { uploadURL } from "../api";

// Thumbs renders locally-known upload paths as inline image previews (the
// journal only stores CAS refs, so these exist for messages sent from this tab).
function Thumbs({ paths }: { paths: string[] }) {
  return (
    <div className="thumbs">
      {paths.map((p, i) => (
        <img className="thumb" key={i} src={uploadURL(p)} alt="" />
      ))}
    </div>
  );
}

// MsgActions is the hover action row under a message (Codex puts Copy / reactions
// there). We ship Copy — the whole message text to the clipboard.
function MsgActions({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  if (!text) return null;
  const copy = async () => {
    await copyText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  };
  return (
    <div className="msg-actions">
      <button className="msg-copy" onClick={copy} title="Copy message">
        {copied ? <><Check size={13} /> Copied</> : <><Copy size={13} /> Copy</>}
      </button>
    </div>
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
      return { verb: "spawn sub-agent", body: a.agent || a.role?.name || a.task || "", mono: false };
    case "send_message":
      return { verb: "message", body: `→ ${a.to || "?"} · ${a.text || ""}`, mono: false };
    case "task_kill":
      return { verb: "kill task", body: a.handle || "", mono: true };
    default:
      return { verb: name, body: a.command || a.path || "", mono: true };
  }
}

function StepIcon({ status }: { status: ToolItem["status"] }) {
  if (status === "running") return <span className="step-ic spin" />;
  if (status === "done") return <span className="step-ic ok"><Check size={12} /></span>;
  if (status === "cancelled") return <span className="step-ic warn"><Circle size={8} /></span>;
  return <span className="step-ic err"><X size={11} /></span>;
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

function Item({ it, sentImages }: { it: TimelineItem; sentImages?: Map<number, string[]> }) {
  switch (it.kind) {
    case "turn":
      return <div className="turn">turn {it.gen}</div>;
    case "user": {
      const thumbs = it.seq !== undefined ? sentImages?.get(it.seq) : undefined;
      return (
        <div className="msg user">
          <div className="msg-col user">
            <div className="bubble">
              {it.text}
              {thumbs && thumbs.length ? (
                <Thumbs paths={thumbs} />
              ) : it.images ? (
                <div className="imgnote"><ImageSquare size={13} /> ×{it.images} attached</div>
              ) : null}
            </div>
            <MsgActions text={it.text} />
          </div>
          <span className="who">{it.source || "you"}</span>
        </div>
      );
    }
    case "assistant":
      return (
        <div className="msg assistant">
          <div className="avatar a"><Robot size={14} weight="bold" /></div>
          <div className="msg-col">
            <div className="bubble">
              <Markdown text={it.text} />
            </div>
            <MsgActions text={it.text} />
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
            <a href={"#" + it.childSession}>open sub-session <ArrowSquareOut size={12} /></a>
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
  sentImages,
  statusLine,
  approvalSlot,
}: {
  items: TimelineItem[];
  pending: { id: number; text: string; imgs: string[]; files: number }[];
  typing: string;
  showSys: boolean;
  sentImages?: Map<number, string[]>;
  statusLine?: ReactNode;
  approvalSlot?: ReactNode;
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
        {visible.length === 0 && statusLine}
        {visible.map((it, index) => (
          <Fragment key={it.key}>
            <Item it={it} sentImages={sentImages} />
            {index === 0 && statusLine}
          </Fragment>
        ))}
        {approvalSlot}
        {typing && (
          <div className="msg assistant">
            <div className="avatar a"><Robot size={14} weight="bold" /></div>
            <div className="bubble typing">{typing}</div>
          </div>
        )}
        {pending.map((p) => (
          <div className="msg user" key={"p" + p.id}>
            <div className="bubble pending">
              {p.text}
              {p.imgs.length ? <Thumbs paths={p.imgs} /> : null}
              {p.files ? <div className="imgnote"><File size={13} /> ×{p.files} attached</div> : null}
            </div>
            <span className="who">queued…</span>
          </div>
        ))}
      </div>
    </div>
  );
}
