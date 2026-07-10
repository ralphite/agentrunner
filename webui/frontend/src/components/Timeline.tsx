import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { ArrowSquareOut, CaretRight, Check, Circle, Copy, File, ImageSquare, Robot, X } from "@phosphor-icons/react";
import { completedTurnDurations, foldWork, formatWorkDuration, type RenderNode, type TimelineItem, type ToolItem, type WorkFold } from "../timeline";
import { Markdown } from "./Markdown";
import { copyText } from "../clipboard";
import { uploadURL } from "../api";

// absTime renders an event timestamp for hover titles: local, second-precise.
function absTime(ts?: string): string | undefined {
  if (!ts) return undefined;
  const d = new Date(ts);
  return isNaN(d.getTime()) ? undefined : d.toLocaleString();
}

// shortTime renders a message timestamp for the hover action row (Codex shows
// the time there, not as centered feed markers).
function shortTime(ts?: string): string | null {
  if (!ts) return null;
  const d = new Date(ts);
  if (isNaN(d.getTime())) return null;
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

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
function MsgActions({ text, ts, onContinue }: { text: string; ts?: string; onContinue?: () => void }) {
  const [copied, setCopied] = useState(false);
  if (!text) return null;
  const copy = async () => {
    await copyText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  };
  const time = shortTime(ts);
  return (
    <div className="msg-actions">
      <button className="msg-copy" onClick={copy} title="Copy message">
        {copied ? <><Check size={13} /> Copied</> : <><Copy size={13} /> Copy</>}
      </button>
      {onContinue && (
        <button className="msg-copy icon-only" onClick={onContinue} title="Continue in new task" aria-label="Continue in new task">
          <ArrowSquareOut size={15} />
        </button>
      )}
      {time && <span className="msg-time" title={absTime(ts)}>{time}</span>}
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

// ShellDetail renders a bash activity as a Codex-style Shell block:
// "$ command" + captured output + a ✓ Success / ✗ Exit N footer.
function ShellDetail({ t }: { t: ToolItem }) {
  const parse = (raw: any) => {
    if (typeof raw !== "string") return raw;
    try {
      return JSON.parse(raw);
    } catch {
      return raw;
    }
  };
  const args = parse(t.args) || {};
  const r = parse(t.result);
  const cmd: string = args.command || "";
  const stdout = r && typeof r === "object" ? r.stdout || "" : typeof r === "string" ? r : "";
  const stderr = r && typeof r === "object" ? r.stderr || "" : "";
  const exit = r && typeof r === "object" && typeof r.exit_code === "number" ? r.exit_code : undefined;
  const cancelled = t.status === "cancelled";
  const ok = !cancelled && t.status !== "error" && t.status !== "failed" && (exit === undefined || exit === 0);
  const out = [stdout, stderr].filter(Boolean).join(stdout && stderr ? "\n" : "");
  return (
    <div className="shell">
      <div className="shell-hd">Shell</div>
      {cmd && <pre className="shell-cmd">$ {cmd}</pre>}
      {out && <pre className="shell-out">{out.slice(0, 20000)}</pre>}
      {t.partial && <pre className="shell-out">{t.partial}</pre>}
      {t.errorMsg && <pre className="shell-out err">{t.errorMsg}</pre>}
      <div className={"shell-status" + (ok ? "" : " bad")}>
        {ok ? <><Check size={12} /> Success</> : cancelled ? <><Circle size={9} /> Cancelled</> : <><X size={12} /> {exit !== undefined && exit !== 0 ? `Exit ${exit}` : "Failed"}</>}
      </div>
    </div>
  );
}

function ToolCard({ t }: { t: ToolItem }) {
  const { verb, body, mono } = toolLabel(t.name, t.args);
  const isShell = t.name === "bash";
  const hasDetail =
    isShell || t.result !== undefined || t.errorMsg || t.partial || (!body && t.args !== undefined);
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
          {isShell ? (
            <ShellDetail t={t} />
          ) : (
            <>
              {!body && t.args !== undefined && <pre>{pretty(t.args)}</pre>}
              {t.result !== undefined && <pre>{pretty(t.result).slice(0, 20000)}</pre>}
              {t.errorMsg && <pre className="err">{t.errorMsg}</pre>}
              {t.partial && <pre>{t.partial}</pre>}
            </>
          )}
        </div>
      )}
    </details>
  );
}

// groupLabel summarizes one consecutive run of tool activity the way Codex
// does ("Edited files, read files, ran commands") — distinct categories in
// first-appearance order.
export function groupLabel(tools: ToolItem[]): string {
  const cats: string[] = [];
  const add = (c: string) => {
    if (!cats.includes(c)) cats.push(c);
  };
  for (const t of tools) {
    switch (t.name) {
      case "bash":
        add("ran commands");
        break;
      case "read_file":
        add("read files");
        break;
      case "grep":
      case "glob":
      case "semantic_search":
        add("searched files");
        break;
      case "write_file":
      case "edit_file":
        add("edited files");
        break;
      case "web_fetch":
        add("fetched the web");
        break;
      case "spawn_agent":
        add("started sub-agents");
        break;
      case "send_message":
        add("messaged agents");
        break;
      case "ask_user":
        add("asked you");
        break;
      case "progress_update":
      case "goal_status":
      case "goal_complete":
        add("tracked progress");
        break;
      default:
        add("used " + t.name.replace(/_/g, " "));
    }
  }
  const s = cats.join(", ");
  return s.charAt(0).toUpperCase() + s.slice(1);
}

// ActivityGroup: level-2 disclosure for a run of consecutive tool calls.
// A single call skips the wrapper and renders its row directly.
function ActivityGroup({ tools }: { tools: ToolItem[] }) {
  if (tools.length === 1) return <ToolCard t={tools[0]} />;
  const failed = tools.some((t) => t.status === "error" || t.status === "failed");
  return (
    <details className={"act-group" + (failed ? " error" : "")}>
      <summary>
        <span className="act-ic"><CaretRight size={12} className="act-caret" /></span>
        <span className="act-label">{groupLabel(tools)}</span>
        <span className="act-count">{tools.length}</span>
      </summary>
      <div className="act-body">
        {tools.map((t) => (
          <ToolCard t={t} key={t.key} />
        ))}
      </div>
    </details>
  );
}

// WorkedFold: the turn-level "Worked for N ⌄" disclosure holding all work
// detail of a settled turn (W2/W3). Consecutive tool calls aggregate into
// ActivityGroups; chips and planning narration render between them.
function WorkedFold({ fold, sentImages }: { fold: WorkFold; sentImages?: Map<number, string[]> }) {
  const [open, setOpen] = useState(false);
  const expandable = fold.children.length > 0;
  const label = fold.durationMs !== undefined ? `Worked for ${formatWorkDuration(fold.durationMs)}` : "Worked";

  // group consecutive tools; pass through everything else in order
  const rows: ReactNode[] = [];
  if (open) {
    let run: ToolItem[] = [];
    const flushRun = () => {
      if (run.length) {
        rows.push(<ActivityGroup tools={run} key={"g" + run[0].key} />);
        run = [];
      }
    };
    for (const it of fold.children) {
      if (it.kind === "tool") {
        run.push(it);
      } else {
        flushRun();
        rows.push(<Item it={it} sentImages={sentImages} key={it.key} />);
      }
    }
    flushRun();
  }

  return (
    <div className={"worked" + (open ? " open" : "")}>
      <button
        type="button"
        className="worked-row"
        onClick={expandable ? () => setOpen(!open) : undefined}
        disabled={!expandable}
        aria-expanded={open}
      >
        {label}
        {expandable && <CaretRight size={15} className="worked-caret" />}
      </button>
      {open && <div className="worked-body">{rows}</div>}
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

function runtimeLabel(source: string, text: string): string {
  if (source === "agent") return "Agent message";
  if (source === "parent") return "Parent instruction";
  if (source === "control") return "Control message";
  if (source === "program" && /<goal>|goal/i.test(text)) return "Goal continuation";
  return "Runtime message";
}

// CollapsibleUserText folds tall user messages (long pastes) to ~10 rendered
// lines with a Show more/less toggle (INC-36). Measured against the rendered
// box rather than counting "\n", so a few very long wrapped lines fold too;
// a ResizeObserver re-checks when the column width changes. Folding is view
// state only — MsgActions copy always gets the full text.
const COLLAPSE_TOLERANCE_PX = 4; // don't offer a toggle that reveals one clipped pixel
function CollapsibleUserText({ text }: { text: string }) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [tall, setTall] = useState(false);
  const [open, setOpen] = useState(false);
  useEffect(() => {
    setOpen(false);
    const el = ref.current;
    if (!el) return;
    const check = () => setTall(el.scrollHeight > el.clientHeight + COLLAPSE_TOLERANCE_PX);
    check();
    const ro = new ResizeObserver(check);
    ro.observe(el);
    return () => ro.disconnect();
  }, [text]);
  return (
    <>
      <div ref={ref} className={"utext" + (open ? "" : " clamped")}>{text}</div>
      {(tall || open) && (
        <button type="button" className="ushow" onClick={() => setOpen(!open)}>
          {open ? "Show less" : "Show more"}
        </button>
      )}
    </>
  );
}

function Item({ it, sentImages, onContinue }: { it: TimelineItem; sentImages?: Map<number, string[]>; onContinue?: () => void }) {
  switch (it.kind) {
    case "turn":
      return <div className="turn">turn {it.gen}</div>;
    case "user": {
      const thumbs = it.seq !== undefined ? sentImages?.get(it.seq) : undefined;
      const peer = !!it.peerSession;
      return (
        <div className={"msg user" + (peer ? " peer" : "")} title={absTime(it.ts)}>
          <div className="msg-col user">
            <div className="bubble">
              <CollapsibleUserText text={it.text} />
              {thumbs && thumbs.length ? (
                <Thumbs paths={thumbs} />
              ) : it.images ? (
                <div className="imgnote"><ImageSquare size={13} /> ×{it.images} attached</div>
              ) : null}
            </div>
            <MsgActions text={it.text} ts={it.ts} />
          </div>
          <span className="who">
            {peer ? <>from {it.source} · <a href={"#" + it.peerSession}>open</a></> : it.source || "you"}
          </span>
        </div>
      );
    }
    case "assistant":
      return (
        <div className="msg assistant" title={absTime(it.ts)}>
          <div className="avatar a"><Robot size={14} weight="bold" /></div>
          <div className="msg-col">
            <div className="bubble">
              <Markdown text={it.text} />
            </div>
            <MsgActions text={it.text} ts={it.ts} onContinue={onContinue} />
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
    case "runtime":
      return (
        <details className="runtime-event">
          <summary>{runtimeLabel(it.source, it.text)}</summary>
          <div className="runtime-event-body"><Markdown text={it.text} /></div>
        </details>
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
  active = false,
  onContinue,
  outcomeSlot,
}: {
  items: TimelineItem[];
  pending: { id: number; text: string; imgs: string[]; files: number }[];
  typing: string;
  showSys: boolean;
  sentImages?: Map<number, string[]>;
  statusLine?: ReactNode;
  approvalSlot?: ReactNode;
  active?: boolean;
  onContinue?: () => void;
  outcomeSlot?: ReactNode;
}) {
  // Codex shows a continuous activity feed — no "turn N" dividers, no raw
  // system events. Those stay behind the developer toggle.
  const visible = showSys
    ? items
    : items.filter((it) => it.kind !== "sys" && it.kind !== "turn" && it.kind !== "runtime");
  const ref = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const durations = completedTurnDurations(visible, active);
  // W2: settled turns collapse their work behind "Worked for N ⌄"; the
  // developer (showSys) view stays flat and raw.
  const nodes: RenderNode[] = showSys ? visible : foldWork(visible, durations, active);

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
        {nodes.map((it) => {
          if (it.kind === "fold") return <WorkedFold fold={it} sentImages={sentImages} key={it.key} />;
          return (
            <Fragment key={it.key}>
              {showSys && durations.has(it.key) && (
                <div className="worked-row static">Worked for {formatWorkDuration(durations.get(it.key)!)}</div>
              )}
              <Item it={it} sentImages={sentImages} onContinue={it.kind === "assistant" ? onContinue : undefined} />
            </Fragment>
          );
        })}
        {statusLine}
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
              <CollapsibleUserText text={p.text} />
              {p.imgs.length ? <Thumbs paths={p.imgs} /> : null}
              {p.files ? <div className="imgnote"><File size={13} /> ×{p.files} attached</div> : null}
            </div>
            <span className="who">queued…</span>
          </div>
        ))}
        {!active && !typing && pending.length === 0 && outcomeSlot}
      </div>
    </div>
  );
}
