import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import {
  ArrowsInLineVertical,
  ArrowSquareOut,
  CaretDown,
  CaretRight,
  ChatCircle,
  Check,
  Circle,
  Copy,
  File,
  FileText,
  Globe,
  ImageSquare,
  Lightning,
  ListChecks,
  MagnifyingGlass,
  PencilSimple,
  Question,
  Robot,
  Terminal,
  Warning,
  Wrench,
  X,
} from "@phosphor-icons/react";
import {
  askUserDetail,
  completedTurnDurations,
  editDetail,
  foldWork,
  formatWorkDuration,
  globDetail,
  grepDetail,
  groupIcon,
  readDetail,
  semanticDetail,
  spawnDetail,
  webFetchDetail,
  type ActivityCategory,
  type DiffLine,
  type RenderNode,
  type TimelineItem,
  type ToolItem,
  type WorkFold,
} from "../timeline";
import { Markdown } from "./Markdown";
import { copyText } from "../clipboard";
import { uploadURL } from "../api";
import { Lightbox } from "./Lightbox";
import "../styles.conv.css";

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
// Clicking a thumbnail opens the group in the full-screen Lightbox (W9), where
// arrow keys page across the group's images.
function Thumbs({ paths }: { paths: string[] }) {
  const [lightbox, setLightbox] = useState<number | null>(null);
  return (
    <div className="thumbs">
      {paths.map((p, i) => (
        <img
          className="thumb"
          key={i}
          src={uploadURL(p)}
          alt=""
          role="button"
          tabIndex={0}
          title="View image"
          onClick={() => setLightbox(i)}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              setLightbox(i);
            }
          }}
        />
      ))}
      {lightbox !== null && (
        <Lightbox images={paths} index={lightbox} onIndex={setLightbox} onClose={() => setLightbox(null)} />
      )}
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

// ---- A1: category icon for an aggregated activity row -----------------------
// One icon per group (from timeline.groupIcon), distinct from the per-step
// StepIcon (which carries run/ok/error state).
function CategoryIcon({ cat, size = 14 }: { cat: ActivityCategory; size?: number }) {
  switch (cat) {
    case "bash":
      return <Terminal size={size} />;
    case "read":
      return <FileText size={size} />;
    case "edit":
      return <PencilSimple size={size} />;
    case "search":
      return <MagnifyingGlass size={size} />;
    case "web":
      return <Globe size={size} />;
    case "spawn":
      return <Robot size={size} />;
    case "message":
      return <ChatCircle size={size} />;
    case "ask":
      return <Question size={size} />;
    case "progress":
      return <ListChecks size={size} />;
    default:
      return <Wrench size={size} />;
  }
}

// ---- A2: tool-specific detail renderers --------------------------------------
// Each replaces the raw <pre>{JSON}</pre> with a Codex-style structured view.
// Unknown tools fall back to JSONDetail.
const STRUCTURED_TOOLS = new Set([
  "read_file",
  "read_notes",
  "write_file",
  "edit_file",
  "grep",
  "glob",
  "semantic_search",
  "spawn_agent",
  "web_fetch",
  "ask_user",
]);

function MiniDiff({ rows, more }: { rows: DiffLine[]; more: number }) {
  return (
    <div className="cx-minidiff">
      {rows.map((r, i) => (
        <div className={"cx-dl " + r.kind} key={i}>
          <span className="cx-dl-sign">{r.kind === "add" ? "+" : r.kind === "del" ? "-" : " "}</span>
          <span className="cx-dl-text">{r.text || " "}</span>
        </div>
      ))}
      {more > 0 && (
        <div className="cx-dl-more">
          … {more} more line{more === 1 ? "" : "s"}
        </div>
      )}
    </div>
  );
}

function ReadDetailView({ t }: { t: ToolItem }) {
  const d = readDetail(t.args, t.result);
  return (
    <div className="flex flex-col gap-[6px] px-[10px] py-[8px] text-[12.5px]">
      <div className="cx-td-head">
        <FileText size={13} className="cx-td-ic" />
        <span className="cx-td-path">{d.path}</span>
        {d.range && <span className="cx-td-meta">{d.range}</span>}
        {d.lineCount != null && (
          <span className="cx-td-meta">
            {d.lineCount} line{d.lineCount === 1 ? "" : "s"}
            {d.truncated ? " (truncated)" : ""}
          </span>
        )}
      </div>
    </div>
  );
}

function EditDetailView({ t }: { t: ToolItem }) {
  const d = editDetail(t.name, t.args, t.result);
  return (
    <div className="flex flex-col gap-[6px] px-[10px] py-[8px] text-[12.5px]">
      <div className="cx-td-head">
        <PencilSimple size={13} className="cx-td-ic" />
        <span className="cx-td-path">{d.path}</span>
        {d.note && <span className="cx-td-meta">{d.note}</span>}
      </div>
      {d.rows.length > 0 && <MiniDiff rows={d.rows} more={d.more} />}
    </div>
  );
}

function GrepDetailView({ t }: { t: ToolItem }) {
  const d = grepDetail(t.args, t.result);
  return (
    <div className="flex flex-col gap-[6px] px-[10px] py-[8px] text-[12.5px]">
      <div className="cx-td-head">
        <MagnifyingGlass size={13} className="cx-td-ic" />
        <code className="cx-td-pattern">{d.pattern}</code>
        <span className="cx-td-meta">
          {d.matchCount} match{d.matchCount === 1 ? "" : "es"}
          {d.fileCount ? ` in ${d.fileCount} file${d.fileCount === 1 ? "" : "s"}` : ""}
          {d.scanned != null ? ` · ${d.scanned} scanned` : ""}
          {d.truncated ? " · truncated" : ""}
        </span>
      </div>
      {d.byFile.length > 0 && (
        <div className="cx-grep-files">
          {d.byFile.map((f) => (
            <div className="cx-grep-file" key={f.path}>
              <div className="cx-grep-fname">{f.path}</div>
              {f.hits.slice(0, 8).map((h, i) => (
                <div className="cx-grep-hit" key={i}>
                  <span className="cx-grep-ln">{h.line ?? ""}</span>
                  <span className="cx-grep-tx">{(h.text || "").split("\n")[0]}</span>
                </div>
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function GlobDetailView({ t }: { t: ToolItem }) {
  const d = globDetail(t.args, t.result);
  const shown = d.paths.slice(0, 40);
  return (
    <div className="flex flex-col gap-[6px] px-[10px] py-[8px] text-[12.5px]">
      <div className="cx-td-head">
        <MagnifyingGlass size={13} className="cx-td-ic" />
        <code className="cx-td-pattern">{d.pattern}</code>
        <span className="cx-td-meta">
          {d.paths.length} path{d.paths.length === 1 ? "" : "s"}
          {d.truncated ? " · truncated" : ""}
        </span>
      </div>
      {shown.length > 0 && (
        <div className="cx-path-list">
          {shown.map((p, i) => (
            <div className="cx-path" key={i}>
              {p}
            </div>
          ))}
          {d.paths.length > shown.length && <div className="cx-dl-more">… {d.paths.length - shown.length} more</div>}
        </div>
      )}
    </div>
  );
}

function SemanticDetailView({ t }: { t: ToolItem }) {
  const d = semanticDetail(t.args, t.result);
  const shown = d.hits.slice(0, 12);
  return (
    <div className="flex flex-col gap-[6px] px-[10px] py-[8px] text-[12.5px]">
      <div className="cx-td-head">
        <MagnifyingGlass size={13} className="cx-td-ic" />
        <span className="cx-td-meta">query</span>
        <span className="cx-td-path">{d.query}</span>
      </div>
      {shown.length > 0 && (
        <div className="cx-path-list">
          {shown.map((h, i) => (
            <div className="cx-path" key={i}>
              {h.path}
              {h.line ? `:${h.line}` : ""}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function SpawnDetailView({ t }: { t: ToolItem }) {
  const d = spawnDetail(t.args, t.result);
  return (
    <div className="flex flex-col gap-[6px] px-[10px] py-[8px] text-[12.5px]">
      <div className="cx-td-head">
        <Robot size={13} className="cx-td-ic" />
        <span className="cx-td-path">{d.agent || "sub-agent"}</span>
        {d.reason && <span className="cx-td-meta">{d.reason}</span>}
        {d.childSession && (
          <a className="cx-td-link" href={"#" + d.childSession}>
            open sub-session <ArrowSquareOut size={11} />
          </a>
        )}
      </div>
      {d.task && <div className="cx-td-task">{d.task}</div>}
    </div>
  );
}

function WebDetailView({ t }: { t: ToolItem }) {
  const d = webFetchDetail(t.args, t.result);
  return (
    <div className="flex flex-col gap-[6px] px-[10px] py-[8px] text-[12.5px]">
      <div className="cx-td-head">
        <Globe size={13} className="cx-td-ic" />
        {d.url ? (
          <a className="cx-td-link" href={d.url} target="_blank" rel="noreferrer">
            {d.url} <ArrowSquareOut size={11} />
          </a>
        ) : (
          <span className="cx-td-path">web_fetch</span>
        )}
      </div>
      <div className="cx-td-sub">
        {d.title && <span className="cx-td-path">{d.title}</span>}
        {d.bytes != null && <span className="cx-td-meta">{d.bytes} bytes</span>}
        {d.untrusted && (
          <span className="cx-td-tag warn">
            <Warning size={11} /> untrusted
          </span>
        )}
      </div>
    </div>
  );
}

function AskDetailView({ t }: { t: ToolItem }) {
  const d = askUserDetail(t.args);
  return (
    <div className="flex flex-col gap-[6px] px-[10px] py-[8px] text-[12.5px]">
      <div className="cx-td-head">
        <Question size={13} className="cx-td-ic" />
        <span className="cx-td-path">{d.question}</span>
      </div>
    </div>
  );
}

function JSONDetail({ t, body }: { t: ToolItem; body: string }) {
  return (
    <>
      {!body && t.args !== undefined && <pre>{pretty(t.args)}</pre>}
      {t.result !== undefined && <pre>{pretty(t.result).slice(0, 20000)}</pre>}
    </>
  );
}

// ToolDetail routes a tool activity to its Codex-style detail renderer. bash
// keeps the Shell block; the rest use structured views; unknown tools fall back
// to pretty JSON. An error/partial footer is appended for the non-bash paths.
function ToolDetail({ t, body }: { t: ToolItem; body: string }) {
  if (t.name === "bash") return <ShellDetail t={t} />;
  let view: ReactNode;
  switch (t.name) {
    case "read_file":
    case "read_notes":
      view = <ReadDetailView t={t} />;
      break;
    case "write_file":
    case "edit_file":
      view = <EditDetailView t={t} />;
      break;
    case "grep":
      view = <GrepDetailView t={t} />;
      break;
    case "glob":
      view = <GlobDetailView t={t} />;
      break;
    case "semantic_search":
      view = <SemanticDetailView t={t} />;
      break;
    case "spawn_agent":
      view = <SpawnDetailView t={t} />;
      break;
    case "web_fetch":
      view = <WebDetailView t={t} />;
      break;
    case "ask_user":
      view = <AskDetailView t={t} />;
      break;
    default:
      view = <JSONDetail t={t} body={body} />;
  }
  return (
    <>
      {view}
      {t.errorMsg && <pre className="cx-td-err">{t.errorMsg}</pre>}
      {t.partial && t.name !== "bash" && <pre className="cx-td-partial">{t.partial}</pre>}
    </>
  );
}

function ToolCard({ t }: { t: ToolItem }) {
  const { verb, body, mono } = toolLabel(t.name, t.args);
  const isShell = t.name === "bash";
  const hasDetail =
    isShell ||
    STRUCTURED_TOOLS.has(t.name) ||
    t.result !== undefined ||
    !!t.errorMsg ||
    !!t.partial ||
    (!body && t.args !== undefined);
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
          <ToolDetail t={t} body={body} />
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
        <span className="act-cat"><CategoryIcon cat={groupIcon(tools)} /></span>
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

// ---- CX-2: the collapsed head must never be information-free -----------------
// foldSpanMs measures a fold from its own children when the turn carried no
// duration (see workedLabel). Only bubbles and runtime injections carry a `ts`
// — tool activities and chips don't — so this recovers a span for folds that
// hold planning narration, and returns undefined for pure tool/chip folds.
function foldSpanMs(fold: WorkFold): number | undefined {
  let lo = Infinity;
  let hi = -Infinity;
  for (const c of fold.children) {
    const ts = (c as { ts?: string }).ts;
    if (!ts) continue;
    const at = new Date(ts).getTime();
    if (!Number.isFinite(at)) continue;
    if (at < lo) lo = at;
    if (at > hi) hi = at;
  }
  const span = hi - lo;
  return Number.isFinite(span) && span > 0 ? span : undefined;
}

// workedLabel: the text of a collapsed turn head. A turn only gets a duration
// when it reached a final assistant answer (timeline.completedTurnDurations) —
// so a turn that was cut into segments by top-level approval chips, or that
// ended in an error/approval stall instead of an answer, has durationMs
// undefined. That used to degrade to a bare "Worked ›" carrying zero
// information (six of them in a row on an approval-heavy task). Ladder:
// stored duration → span measured off the fold's own children → step count.
export function workedLabel(fold: WorkFold): string {
  const ms = fold.durationMs ?? foldSpanMs(fold);
  if (ms !== undefined) return `Worked for ${formatWorkDuration(ms)}`;
  const steps = fold.children.filter((c) => c.kind === "tool").length;
  if (steps > 0) return `Worked · ${steps} step${steps === 1 ? "" : "s"}`;
  const n = fold.children.length;
  if (n > 0) return `Worked · ${n} item${n === 1 ? "" : "s"}`;
  return "Worked · no activity";
}

// WorkedFold: the turn-level "Worked for N ⌄" disclosure holding all work
// detail of a settled turn (W2/W3). Consecutive tool calls aggregate into
// ActivityGroups; chips and planning narration render between them.
function WorkedFold({
  fold,
  sentImages,
  open,
  onToggle,
}: {
  fold: WorkFold;
  sentImages?: Map<number, string[]>;
  open: boolean;
  onToggle: () => void;
}) {
  const expandable = fold.children.length > 0;
  const label = workedLabel(fold);

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
        onClick={expandable ? onToggle : undefined}
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
      const hasText = !!it.text.trim();
      const hasAttach = (thumbs && thumbs.length) || it.images;
      return (
        <div className={"msg user" + (peer ? " peer" : "")} title={absTime(it.ts)}>
          <div className="msg-col user">
            <div className="bubble">
              {hasText ? (
                <CollapsibleUserText text={it.text} />
              ) : !hasAttach ? (
                // An empty prompt would otherwise render as a bare blank bubble
                // (R4-10) — label it instead of showing an empty blob.
                <span className="dim">(empty message)</span>
              ) : null}
              {thumbs && thumbs.length ? (
                <Thumbs paths={thumbs} />
              ) : it.images ? (
                <div className="imgnote"><ImageSquare size={13} /> ×{it.images} attached</div>
              ) : null}
            </div>
            {it.sentAsGoal && (
              <div className="cx-goal-note">
                <Lightning size={12} weight="fill" /> Sent as goal
              </div>
            )}
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
      // A3: compaction (and any activity-marked chip) renders as a Codex
      // activity row — icon + label — rather than a bubble chip.
      if (it.activity)
        return (
          <div className="cx-activity-row">
            <span className="cx-activity-ic"><ArrowsInLineVertical size={14} /></span>
            <span className="cx-activity-label">{it.text}</span>
          </div>
        );
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
  loading = false,
}: {
  items: TimelineItem[];
  pending: { id: number; text: string; imgs: string[]; files: number; delivery?: "steer" | "queue" }[];
  typing: string;
  showSys: boolean;
  sentImages?: Map<number, string[]>;
  statusLine?: ReactNode;
  approvalSlot?: ReactNode;
  active?: boolean;
  onContinue?: () => void;
  outcomeSlot?: ReactNode;
  /** The first events fetch for this session hasn't returned yet (INC-41 L1). */
  loading?: boolean;
}) {
  // Codex shows a continuous activity feed — no "turn N" dividers, no raw
  // system events. Those stay behind the developer toggle.
  const visible = showSys
    ? items
    : items.filter((it) => it.kind !== "sys" && it.kind !== "turn" && it.kind !== "runtime");
  const ref = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  // W10: floating jump-to-bottom control once the reader scrolls up (Codex
  // shows the same affordance on long threads).
  const [showJump, setShowJump] = useState(false);
  // A7: fold open state lives here (not inside WorkedFold) keyed by a stable
  // content id, so a poll that reshuffles fold render keys never collapses a
  // fold the reader already opened.
  const [openFolds, setOpenFolds] = useState<Set<string>>(() => new Set());
  const toggleFold = (id: string) =>
    setOpenFolds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  // Pass the FULL item list (incl. hidden "turn" markers) so the duration is
  // measured from generation_started, not the user message (R4-6). Keys land
  // on assistant items that also exist in `visible`, so foldWork still matches.
  const durations = completedTurnDurations(items, active);
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
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    stick.current = nearBottom;
    setShowJump(!nearBottom);
  };

  const jumpToBottom = () => {
    const el = ref.current;
    if (!el) return;
    stick.current = true;
    el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  };

  // Nothing to show yet — but WHY decides what we render (INC-41 L1): while the
  // first fetch is still in flight the timeline is merely unknown, so claiming
  // "No messages yet" on a session with a long history is a lie the reader sees
  // for ~1s before it snaps away. Skeleton while loading; the empty state only
  // once we know the session really is empty (R4-11).
  const blank = nodes.length === 0 && !typing && pending.length === 0 && !statusLine && !approvalSlot;
  const isEmpty = blank && !loading;

  return (
    <div className="timeline" ref={ref} onScroll={onScroll}>
      <div className="tl-inner">
        {blank && loading && (
          <div className="tl-skeleton" role="status" aria-label="Loading conversation">
            <div className="tl-skel-row user">
              <span className="tl-skel-bubble" />
            </div>
            <div className="tl-skel-row">
              <span className="tl-skel-avatar" />
              <span className="tl-skel-bubble" />
            </div>
            <div className="tl-skel-row">
              <span className="tl-skel-avatar" />
              <span className="tl-skel-bubble" />
            </div>
          </div>
        )}
        {isEmpty && (
          <div className="tl-empty">
            <ChatCircle size={26} weight="light" />
            <b>No messages yet</b>
            <span>This task hasn't started. Send a message below to begin.</span>
          </div>
        )}
        {nodes.map((it) => {
          if (it.kind === "fold") {
            const foldId = it.children[0]?.key ?? it.key;
            return (
              <WorkedFold
                fold={it}
                sentImages={sentImages}
                open={openFolds.has(foldId)}
                onToggle={() => toggleFold(foldId)}
                key={it.key}
              />
            );
          }
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
            <div className="bubble typing" role="status" aria-label="Thinking" />
          </div>
        )}
        {pending.map((p) => (
          <div className="msg user" key={"p" + p.id}>
            <div className={"bubble pending" + (p.delivery === "steer" ? " steer" : "")}>
              <CollapsibleUserText text={p.text} />
              {p.imgs.length ? <Thumbs paths={p.imgs} /> : null}
              {p.files ? <div className="imgnote"><File size={13} /> ×{p.files} attached</div> : null}
            </div>
            <span className="who">{p.delivery === "steer" ? "steering…" : "queued…"}</span>
          </div>
        ))}
        {!active && !typing && pending.length === 0 && outcomeSlot}
      </div>
      {showJump && (
        <button type="button" className="tl-jump" onClick={jumpToBottom} title="Jump to latest" aria-label="Jump to latest">
          <CaretDown size={16} />
        </button>
      )}
    </div>
  );
}
