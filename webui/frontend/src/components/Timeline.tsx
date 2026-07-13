import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import {
  ArrowsInLineVertical,
  ArrowSquareOut,
  CaretDown,
  CaretRight,
  ChatCircle,
  Check,
  CheckCircle,
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
  Share,
  Terminal,
  Warning,
  Wrench,
  X,
} from "@phosphor-icons/react";
import {
  askUserDetail,
  completedTurnDurations,
  editDetail,
  foldRuns,
  foldWork,
  formatWorkDuration,
  globDetail,
  grepDetail,
  groupIcon,
  readDetail,
  semanticDetail,
  spawnDetail,
  toolLabel,
  webFetchDetail,
  type ActivityCategory,
  type DiffLine,
  type FoldRun,
  type RenderNode,
  type TimelineItem,
  type ToolItem,
  type WorkFold,
} from "../timeline";
import { Markdown } from "./Markdown";
import { copyText } from "../clipboard";
import { sessionImageURL, uploadURL } from "../api";
import { Lightbox } from "./Lightbox";

// absTime renders an event timestamp for hover titles: local, second-precise.
function absTime(ts?: string): string | undefined {
  if (!ts) return undefined;
  const d = new Date(ts);
  return isNaN(d.getTime()) ? undefined : d.toLocaleString();
}

// calendarDaysAgo: whole LOCAL calendar days between two instants — not 24h
// chunks. Normalising to local midnight first is what makes "yesterday at
// 11:50 PM" read as 1 day ago rather than 0, and keeps the count right across
// a DST shift (a 23h or 25h day is still one day).
function calendarDaysAgo(d: Date, now: Date): number {
  const midnight = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  return Math.round((midnight(now) - midnight(d)) / 86400000);
}

// shortTime renders a message timestamp for the message action row (Codex shows
// the time there, not as centered feed markers).
//
// TR-2: agent sessions run for hours to days, so the bare "11:31 PM" this used to
// emit was unreadable the moment a thread crossed midnight — a real session
// shows "11:31 PM" above "12:40 AM" with nothing saying those are two different
// days. The only thing a timestamp on a long thread is FOR is locating which
// day something happened, so the label carries a date the moment it isn't
// today's. Ladder:
//   today            → "10:14 PM"
//   1–6 days ago     → "Friday 10:14 PM"   (Codex's form)
//   7+ days ago      → "Jul 3, 10:14 PM"
// The weekday tier stops at 6 on purpose: at exactly 7 days the weekday name
// repeats today's, so "Friday" would be ambiguous between this Friday and last.
// A future ts (clock skew) degrades to the plain time rather than "in 3 days".
//
// `now` is injectable so the tiers can be pinned by tests without depending on
// the wall clock. Locale/timezone come from the reader's environment (Intl).
export function shortTime(ts?: string, now: Date = new Date()): string | null {
  if (!ts) return null;
  const d = new Date(ts);
  if (isNaN(d.getTime())) return null;
  const time = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const days = calendarDaysAgo(d, now);
  if (days <= 0) return time;
  if (days < 7) return `${d.toLocaleDateString([], { weekday: "long" })} ${time}`;
  return `${d.toLocaleDateString([], { month: "short", day: "numeric" })}, ${time}`;
}

// Thumbs renders a group of images as inline previews. `paths` are either local
// upload paths (a message sent from THIS tab) or durable session-blob URLs
// (RT-6) — uploadURL takes both. Clicking a thumbnail opens the group in the
// full-screen Lightbox (W9), where arrow keys page across the group's images.
//
// An image that fails to load drops out (a blob can be GC'd, and a broken-image
// glyph is worse than nothing); when they ALL fail, `fallback` — the honest
// "×N attached" note — takes the group's place. So the text stub is now the
// last resort it was always meant to be, not the default.
function Thumbs({ paths, fallback }: { paths: string[]; fallback?: ReactNode }) {
  const [lightbox, setLightbox] = useState<number | null>(null);
  const [broken, setBroken] = useState<Set<number>>(() => new Set());
  const ok = paths.filter((_, i) => !broken.has(i));
  if (paths.length > 0 && ok.length === 0) return <>{fallback ?? null}</>;
  return (
    <div className="thumbs">
      {paths.map((p, i) =>
        broken.has(i) ? null : (
          <img
            className="thumb"
            key={i}
            src={uploadURL(p)}
            alt=""
            role="button"
            tabIndex={0}
            title="View image"
            onError={() => setBroken((prev) => new Set(prev).add(i))}
            onClick={() => setLightbox(i)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                setLightbox(i);
              }
            }}
          />
        ),
      )}
      {lightbox !== null && (
        <Lightbox images={paths} index={lightbox} onIndex={setLightbox} onClose={() => setLightbox(null)} />
      )}
    </div>
  );
}

// MsgActions is the action row under a message (Codex puts Copy / reactions
// there). We ship an icon-only Copy (whole message text), a Share that reuses the
// copy-link mechanism (the current hash route already deep-links this session), and
// — on the final assistant answer of a satisfied run — an inline "Goal achieved
// in N" verdict. Thumbs up/down are deliberately omitted: there is no feedback
// endpoint to wire them to, so they'd be dead controls (deferred until one lands).
//
// TH-21: the row is HOVER-ONLY on every message except the thread's last
// assistant answer, which keeps it at rest — that is the one row Codex draws
// persistently. Both switches are CSS, keyed off the `.msg-last` class this file
// puts on that message (see the TH-21 block in tw.css):
//   • the row itself: opacity 0 at rest on every `:not(.msg-last)` message;
//   • the timestamp: hidden on `.msg-last`, because the gold master's persistent
//     row is `⧉ 👍 👎 ↗ │ ⊘ Goal achieved in 3h 47m 26s` and carries no time.
// So one row shape is rendered for every message and the sheet decides what of
// it is visible where — no branchy JSX, and the tier ladder (shortTime) keeps
// producing a real label on the rows that do show one (the hover-revealed ones).
function MsgActions({
  text,
  ts,
  onContinue,
  goalVerdict,
}: {
  text: string;
  ts?: string;
  onContinue?: () => void;
  goalVerdict?: { elapsed: string } | null;
}) {
  const [copied, setCopied] = useState(false);
  const [shared, setShared] = useState(false);
  if (!text) return null;
  const copy = async () => {
    await copyText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  };
  // Share = copy a deep link to this session. The router keys off the URL hash, so
  // the current location already targets this session — no backend needed.
  const share = async () => {
    await copyText(location.href);
    setShared(true);
    setTimeout(() => setShared(false), 1200);
  };
  const time = shortTime(ts);
  return (
    <div className="msg-actions">
      <button className="msg-copy icon-only" onClick={copy} title="Copy message" aria-label="Copy message">
        {copied ? <Check size={15} /> : <Copy size={15} />}
      </button>
      <button className="msg-copy icon-only" onClick={share} title="Copy link to this session" aria-label="Copy link to this session">
        {shared ? <Check size={15} /> : <Share size={15} />}
      </button>
      {onContinue && (
        <button className="msg-copy icon-only" onClick={onContinue} title="Continue in new session" aria-label="Continue in new session">
          <ArrowSquareOut size={15} />
        </button>
      )}
      {goalVerdict && (
        <>
          {/* Divider + verdict — Codex appends the goal outcome to the final
              answer's action row. The divider is styled in tw.css (NOT
              inline): at rest the icons are hidden, and a separator with nothing
              to separate must collapse with them (TH-1). */}
          <span className="msg-actions-div" aria-hidden="true" />
          <span className="msg-goal-verdict">
            <CheckCircle size={15} weight="fill" /> Goal achieved in {goalVerdict.elapsed}
          </span>
        </>
      )}
      {time && <span className="msg-time" title={absTime(ts)}>{time}</span>}
    </div>
  );
}

function StepIcon({ status }: { status: ToolItem["status"] }) {
  if (status === "running") return <span className="step-ic spin shrink-0" />;
  if (status === "done") return <span className="step-ic ok shrink-0"><Check size={12} /></span>;
  if (status === "cancelled") return <span className="step-ic warn shrink-0"><Circle size={8} /></span>;
  return <span className="step-ic err shrink-0"><X size={11} /></span>;
}

// ShellDetail renders a bash activity as a Codex-style Shell block:
// "$ command" + captured output + a ✓ Success / ✗ Exit N footer.
function ShellDetail({ t }: { t: ToolItem }) {
  const [copied, setCopied] = useState(false);
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
  const copyBody = [cmd && `$ ${cmd}`, out, t.partial, t.errorMsg].filter(Boolean).join("\n");
  const copy = async () => {
    await copyText(copyBody);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  };
  return (
    <div className="shell min-w-0 max-w-full">
      <div className="shell-hd">
        <span>Shell</span>
        {copyBody && (
          <button
            type="button"
            className="msg-copy icon-only"
            onClick={copy}
            title="Copy command and result"
            aria-label="Copy command and result"
          >
            {copied ? <Check size={15} /> : <Copy size={15} />}
          </button>
        )}
      </div>
      {cmd && (
        <pre className="shell-cmd max-h-[240px] min-w-0 max-w-full overflow-auto whitespace-pre-wrap break-words p-3">
          $ {cmd}
        </pre>
      )}
      {out && (
        <pre className="shell-out max-h-[240px] min-w-0 max-w-full overflow-auto whitespace-pre-wrap break-words">
          {out.slice(0, 20000)}
        </pre>
      )}
      {t.partial && (
        <pre className="shell-out max-h-[240px] min-w-0 max-w-full overflow-auto whitespace-pre-wrap break-words">
          {t.partial}
        </pre>
      )}
      {t.errorMsg && (
        <pre className="shell-out err max-h-[240px] min-w-0 max-w-full overflow-auto whitespace-pre-wrap break-words">
          {t.errorMsg}
        </pre>
      )}
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
      {d.prompt && <div className="cx-td-prompt">{d.prompt}</div>}
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
  const firstLine = body.trim().split("\n", 1)[0] || "";
  const summaryBody = firstLine.length > 160 ? `${firstLine.slice(0, 159).trimEnd()}…` : firstLine;
  const hasMoreBody = body.trim().length > firstLine.length;
  const isShell = t.name === "bash";
  const hasDetail =
    isShell ||
    STRUCTURED_TOOLS.has(t.name) ||
    t.result !== undefined ||
    !!t.errorMsg ||
    !!t.partial ||
    (!body && t.args !== undefined);
  return (
    <details className={"step group min-w-0 max-w-full overflow-hidden" + (t.status === "error" || t.status === "failed" ? " error" : "")}>
      <summary className="flex min-w-0 items-start gap-2">
        <StepIcon status={t.status} />
        <span className="step-verb shrink-0">{verb}</span>
        <span className={"step-body min-w-0 flex-1 truncate" + (mono ? " mono" : "")} title={body || undefined}>
          {summaryBody}{hasMoreBody && !summaryBody.endsWith("…") ? " …" : ""}
        </span>
        {t.background && <span className="step-tag shrink-0">background</span>}
        {t.usage && (
          <span className="step-tok shrink-0" title="tokens">
            {t.usage.input_tokens + t.usage.output_tokens} tok
          </span>
        )}
        {hasDetail && (
          <CaretRight
            size={13}
            className="step-caret mt-[2px] shrink-0 text-dim transition-transform group-open:rotate-90"
          />
        )}
      </summary>
      {hasDetail && (
        <div className="step-detail mt-2 min-w-0 max-w-full overflow-hidden">
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
      case "exit_plan_mode":
        add("tracked progress");
        break;
      case "read_notes":
      case "artifacts_list":
      case "artifacts_read":
        add("read notes");
        break;
      case "publish_artifact":
      case "publish_note":
        add("published results");
        break;
      case "handoff_agent":
        add("handed off work");
        break;
      case "skill":
        add("ran skills");
        break;
      case "schedule_next":
      case "finish_series":
        add("scheduled work");
        break;
      case "kill":
      case "output":
        add("managed background work");
        break;
      default:
        // RT-3: never spell an internal tool name at the user. A tool we don't
        // know (skill-provided, future) is summarized, not identified.
        add("used tools");
    }
  }
  const s = cats.join(", ");
  return s.charAt(0).toUpperCase() + s.slice(1);
}

// ActivityGroup: level-2 disclosure for a run of tool calls. The run's chips
// (approval audit, goal checks, compaction) ride inside it, in order — they are
// part of the step list, not separators of it (RT-4) — and so does the planning
// narration the model spoke while doing this work (FOLD-RUN). The summary counts
// and labels the TOOLS: "Ran commands ×3", never "×3" over a pile of approvals.
function ActivityGroup({ run, sentImages }: { run: FoldRun; sentImages?: Map<number, string[]> }) {
  const { tools, members } = run;
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
        {members.map((m) => (
          <Item it={m} sentImages={sentImages} key={m.key} />
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
// information (six of them in a row on an approval-heavy session). Ladder:
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
  // TR-6: a fold with no children is a dead row — no caret, not clickable, and
  // nothing behind it if it were. A real session showed three of them in a row
  // ("Worked for 2s / 1s / 3s"), each one pure noise claiming there is work to
  // look at. A turn whose work detail is empty says nothing worth a line.
  if (!expandable) return null;
  const label = workedLabel(fold);

  // Group the step list (timeline.foldRuns): each run is one activity row, and
  // carries its chips AND its narration inside it, in order.
  //
  // FOLD-RUN: the wrapper used to require 2+ tools, so a run of one tool spilled
  // its members straight into the fold body — and since Gemini narrates between
  // every tool call, that was 33 of the 39 steps here, each one a full-width bare
  // row trailing a raw block of thinking (6585px of fold). A run that carries
  // prose therefore aggregates even when it holds a single step: the row says
  // WHAT was done, and one click shows the step and what the model was thinking
  // when it did it. A lone step with nothing around it still needs no wrapper —
  // it renders as itself, so a one-command turn stays one readable line.
  const rows: ReactNode[] = [];
  if (open) {
    for (const run of foldRuns(fold.children)) {
      const prose = run.members.some((m) => m.kind === "assistant");
      if (run.tools.length > 1 || (run.tools.length === 1 && prose)) {
        rows.push(<ActivityGroup run={run} sentImages={sentImages} key={"g" + run.key} />);
      } else {
        for (const m of run.members) rows.push(<Item it={m} sentImages={sentImages} key={m.key} />);
      }
    }
  }

  return (
    <div className={"worked" + (open ? " open" : "")}>
      <button type="button" className="worked-row" onClick={onToggle} aria-expanded={open}>
        {label}
        <CaretRight size={15} className="worked-caret" />
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

function Item({ it, sentImages, onContinue, goalVerdict, last }: { it: TimelineItem; sentImages?: Map<number, string[]>; onContinue?: () => void; goalVerdict?: { elapsed: string } | null; last?: boolean }) {
  switch (it.kind) {
    case "turn":
      return <div className="turn">turn {it.gen}</div>;
    case "user": {
      // Two ways to see the images this message carried, in order of immediacy:
      //  1. sentImages — the upload paths of a message THIS tab just sent (they
      //     render before the journal even comes back);
      //  2. RT-6: the durable blobs the journal points at, addressed by CAS ref.
      // (2) is what makes an attachment survive a reload or a second tab; the
      // "×N attached" stub is now only the fallback for a blob that's gone.
      const sent = it.seq !== undefined ? sentImages?.get(it.seq) : undefined;
      const blobs =
        it.sessionId && it.imageRefs?.length
          ? it.imageRefs.map((ref) => sessionImageURL(it.sessionId!, ref))
          : undefined;
      const thumbs = sent && sent.length ? sent : blobs;
      const peer = !!it.peerSession;
      const hasText = !!it.text.trim();
      const hasAttach = (thumbs && thumbs.length) || it.images;
      const attachNote = it.images ? (
        <div className="imgnote"><ImageSquare size={13} /> ×{it.images} attached</div>
      ) : null;
      return (
        <div className={"msg user" + (peer ? " peer" : "")} title={absTime(it.ts)} tabIndex={0}>
          <div className="msg-col user">
            <div className="bubble">
              {hasText ? (
                <CollapsibleUserText text={it.text} />
              ) : !hasAttach ? (
                // An empty prompt would otherwise render as a bare blank bubble
                // (R4-10) — label it instead of showing an empty blob.
                <span className="dim">(empty message)</span>
              ) : null}
              {thumbs && thumbs.length ? <Thumbs paths={thumbs} fallback={attachNote} /> : attachNote}
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
      // N4: an assistant answer renders as prose, not a chat bubble — no robot
      // avatar. (The bubble border removal is handled in tw.css.)
      //
      // TH-21: `.msg-last` marks the thread's final assistant answer. It is the
      // ONE message whose action row Codex keeps at rest, so the class is what
      // (a) exempts the row from the hover-only rule and (b) drops its timestamp
      // — both in tw.css. The absolute time stays on every `.msg`'s
      // `title`, so hovering a message still tells you when it landed.
      return (
        <div
          className={"msg assistant" + (last ? " msg-last" : "")}
          title={absTime(it.ts)}
          tabIndex={last ? undefined : 0}
        >
          <div className="msg-col">
            <div className="bubble">
              <Markdown text={it.text} />
            </div>
            <MsgActions text={it.text} ts={it.ts} onContinue={onContinue} goalVerdict={goalVerdict} />
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
    case "compact":
      return (
        <div className="compact-divider">
          <span className="compact-divider-label">
            <ArrowsInLineVertical size={14} /> {it.text}
          </span>
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

// TH-4: the runtime emits one chip per state write, so a single "agent changed"
// action that touches spec + model lands as the SAME chip text twice in a row
// ("Agent changed · dev · gemini-flash-latest" ×2, wrapping onto two lines).
// Codex aggregates a repeated activity row instead of stuttering it. Adjacent
// chips that say exactly the same thing (same tone/link/role) collapse into one
// carrying a "×N" multiplicity — the first chip's identity (key, ts, link,
// fold/activity role) is kept, so folding, click targets and ordering are
// untouched. Non-adjacent repeats (a real second occurrence later in the run)
// still render on their own: they are separated by work the reader can see.
export function mergeAdjacentChips(items: TimelineItem[]): TimelineItem[] {
  const out: TimelineItem[] = [];
  let runCount = 0; // how many raw chips the trailing merged chip stands for
  for (const it of items) {
    const prev = out[out.length - 1];
    if (
      it.kind === "chip" &&
      prev &&
      prev.kind === "chip" &&
      runCount > 0 &&
      prev.tone === it.tone &&
      prev.childSession === it.childSession &&
      !!prev.fold === !!it.fold &&
      !!prev.activity === !!it.activity &&
      baseChipText(prev.text) === it.text
    ) {
      runCount += 1;
      out[out.length - 1] = { ...prev, text: `${it.text} ×${runCount}` };
      continue;
    }
    runCount = it.kind === "chip" ? 1 : 0;
    out.push(it);
  }
  return out;
}

// Strips the "×N" suffix mergeAdjacentChips itself appended, so a third repeat
// still matches the chip it has already been merged into.
function baseChipText(text: string): string {
  return text.replace(/ ×\d+$/, "");
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
  goalVerdict,
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
  /** When the run ended satisfied, the elapsed to show as an inline "Goal
   *  achieved in N" verdict on the final assistant answer's action row (fix 3).
   *  Undefined/null while the goal is unsettled or wasn't achieved. */
  goalVerdict?: { elapsed: string } | null;
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
  // developer (showSys) view stays flat and raw — including chip repeats, which
  // are only aggregated (TH-4) in the reader's view.
  const nodes: RenderNode[] = showSys ? visible : foldWork(mergeAdjacentChips(visible), durations, active);

  // The goal verdict rides the FINAL assistant answer only (fix 3) — a settled
  // run's last word. Assistant answers are turn boundaries, so they sit at the
  // top level of `nodes`, never folded into WorkedFold work.
  //
  // TH-21 reuses the same key: the final assistant answer is also the only
  // message that keeps its action row at rest (`.msg-last`).
  const lastAssistantKey = (() => {
    for (let i = nodes.length - 1; i >= 0; i--) {
      if (nodes[i].kind === "assistant") return nodes[i].key;
    }
    return undefined;
  })();

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

  // TR-1: has a user message already opened a turn above this one? Re-derived on
  // every render (the map below is a fresh closure), so it never carries state
  // across renders — it's a cursor over `nodes`, not view state.
  let seenUser = false;

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
            <span>This session hasn't started. Send a message below to begin.</span>
          </div>
        )}
        {nodes.map((it) => {
          // TR-1: a 1px rule across the full prose column closes each turn.
          // Codex draws it, and without it an 86-turn thread is one undifferen-
          // tiated scroll — the turn is the only navigation unit a long thread
          // has. It rides ABOVE each user message rather than below the previous
          // turn's last node, because a turn can end in an error, an approval
          // stall or a changes card, while the user message that opens the next
          // one is the single landmark every turn is guaranteed to have. The
          // first user message opens nothing, so it gets no rule.
          const sep = it.kind === "user" && seenUser;
          if (it.kind === "user") seenUser = true;
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
              {sep && <div className="turn-sep" role="separator" />}
              {showSys && durations.has(it.key) && (
                <div className="worked-row static">Worked for {formatWorkDuration(durations.get(it.key)!)}</div>
              )}
              <Item
                it={it}
                sentImages={sentImages}
                onContinue={it.kind === "assistant" ? onContinue : undefined}
                goalVerdict={it.kind === "assistant" && it.key === lastAssistantKey ? goalVerdict : undefined}
                last={it.kind === "assistant" && it.key === lastAssistantKey}
              />
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
