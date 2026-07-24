import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import {
  ArrowsInLineVertical,
  ArrowSquareOut,
  ArrowUpRight,
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
  Terminal,
  Warning,
  Wrench,
  X,
} from "@phosphor-icons/react";
import { useAppServices } from "../app/appServices";
import {
  askUserDetail,
  completedTurnDurations,
  editDetail,
  foldRuns,
  foldWork,
  type RetriedItem,
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
  type BubbleItem,
  type DiffLine,
  type FoldRun,
  type RenderNode,
  type TimelineItem,
  type ToolItem,
  type WorkFold,
} from "../timeline";
import { Spinner } from "../ui/Spinner";
import { Markdown } from "./Markdown";
import { copyText } from "../clipboard";
import { sessionImageURL, uploadURL } from "../api";
import { IconButton } from "../ui/IconButton";
import { Lightbox } from "./Lightbox";
import {
  useTimelineScrollController,
  type TimelineScrollController,
} from "../features/timeline/useTimelineScrollController";

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
export function Thumbs({ paths, fallback }: { paths: string[]; fallback?: ReactNode }) {
  const [lightbox, setLightbox] = useState<number | null>(null);
  const [broken, setBroken] = useState<Set<number>>(() => new Set());
  const ok = paths.filter((_, i) => !broken.has(i));
  if (paths.length > 0 && ok.length === 0) return <>{fallback ?? null}</>;
  return (
    <div className="thumbs">
      {paths.map((p, i) =>
        broken.has(i) ? null : (
          <button
            className="thumb-button"
            key={i}
            type="button"
            title="View image"
            aria-label={`View image ${i + 1} of ${paths.length}`}
            onClick={() => setLightbox(i)}
          >
            <img
              className="thumb"
              src={uploadURL(p)}
              alt=""
              aria-hidden="true"
              onError={() => setBroken((prev) => new Set(prev).add(i))}
            />
          </button>
        ),
      )}
      {lightbox !== null && (
        <Lightbox images={paths} index={lightbox} onIndex={setLightbox} onClose={() => setLightbox(null)} />
      )}
    </div>
  );
}

// MsgActions exposes only the content-level action that belongs to this message:
// Copy. Session deep links remain a router / browser-bookmark capability, not a
// repeated action under every message; fork / continue lives in Advanced.
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
export function MsgActions({ text, ts, onContinue }: { text: string; ts?: string; onContinue?: () => Promise<void> }) {
  const { clock } = useAppServices();
  const [copied, setCopied] = useState(false);
  const [continuing, setContinuing] = useState(false);
  const [continueError, setContinueError] = useState("");
  if (!text && !onContinue) return null;
  const copy = async () => {
    await copyText(text);
    setCopied(true);
    clock.setTimeout(() => setCopied(false), 1200);
  };
  const time = shortTime(ts);
  return (
    <div className="msg-actions" aria-live="polite">
      {text && (
        <IconButton
          size="sm"
          variant="ghost"
          className="msg-copy"
          onClick={copy}
          title={copied ? "Copied message" : "Copy message"}
          aria-label={copied ? "Copied message" : "Copy message"}
        >
          {copied ? <Check size={15} /> : <Copy size={15} />}
        </IconButton>
      )}
      {onContinue && (
        <IconButton
          size="sm"
          variant="ghost"
          className="msg-copy"
          loading={continuing}
          onClick={async () => {
            setContinuing(true);
            setContinueError("");
            try {
              await onContinue();
            } catch (e: any) {
              setContinueError(
                e?.message || "Couldn't continue from this message",
              );
            } finally {
              setContinuing(false);
            }
          }}
          title="Continue in new session"
          aria-label={
            continuing ? "Continuing in new session" : "Continue in new session"
          }
        >
          <ArrowUpRight size={15} />
        </IconButton>
      )}
      {continueError && <span className="sr-only">{continueError}</span>}
      {time && <span className="msg-time" title={absTime(ts)}>{time}</span>}
    </div>
  );
}

function StepIcon({ status }: { status: ToolItem["status"] }) {
  if (status === "running") return <Spinner className="step-ic shrink-0" size="sm" aria-hidden="true" />;
  if (status === "done") return <span className="step-ic ok shrink-0"><Check size={12} /></span>;
  if (status === "cancelled") return <span className="step-ic warn shrink-0"><Circle size={8} /></span>;
  return <span className="step-ic err shrink-0"><X size={11} /></span>;
}

// ShellDetail renders a bash activity as a compact Codex-style transcript.
// The outer summary already carries a short one-line command, so the detail
// repeats only long/multiline commands, then output + terminal state + Copy.
export function ShellDetail({ t }: { t: ToolItem }) {
  const { clock } = useAppServices();
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
  const commandAlreadyVisible = !cmd.includes("\n") && cmd.trim().length <= 160;
  const cancelled = t.status === "cancelled";
  const ok = !cancelled && t.status !== "error" && t.status !== "failed" && (exit === undefined || exit === 0);
  const statusText = ok ? "Success" : cancelled ? "Cancelled" : exit !== undefined && exit !== 0 ? `Exit ${exit}` : "Failed";
  const out = [stdout, stderr].filter(Boolean).join(stdout && stderr ? "\n" : "");
  // A failed command's exit status is part of its result, not decorative UI.
  // Omitting it from Copy made `exit 7` indistinguishable from a successful
  // command that printed the same stdout. Keep the established quiet success
  // payload, but preserve the decisive terminal state for every non-success.
  const copyOut = !ok ? out.replace(/\n+$/, "") : out;
  const copyBody = [cmd && `$ ${cmd}`, copyOut, t.partial, t.errorMsg, ok ? "" : statusText].filter(Boolean).join("\n");
  const copy = async () => {
    await copyText(copyBody);
    setCopied(true);
    clock.setTimeout(() => setCopied(false), 1200);
  };
  return (
    <div className="shell min-w-0 max-w-full">
      {cmd && !commandAlreadyVisible && (
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
      <div className="shell-footer">
        <div className={"shell-status" + (ok ? "" : " bad")}>
          {ok ? <><Check size={12} /> {statusText}</> : cancelled ? <><Circle size={9} /> {statusText}</> : <><X size={12} /> {statusText}</>}
        </div>
        {copyBody && (
          <IconButton
            size="sm"
            variant="ghost"
            className="msg-copy"
            onClick={copy}
            title={copied ? "Copied command and result" : "Copy command and result"}
            aria-label={copied ? "Copied command and result" : "Copy command and result"}
          >
            {copied ? <Check size={15} /> : <Copy size={15} />}
          </IconButton>
        )}
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
  "keyword_search",
  "semantic_search", // legacy journals (pre-rename)
  "spawn_agent",
  "web_fetch",
  "ask_user",
]);

export function MiniDiff({ rows, more }: { rows: DiffLine[]; more: number }) {
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

export function ReadDetailView({ t }: { t: ToolItem }) {
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

export function EditDetailView({ t }: { t: ToolItem }) {
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

export function GrepDetailView({ t }: { t: ToolItem }) {
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

export function GlobDetailView({ t }: { t: ToolItem }) {
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

export function SemanticDetailView({ t }: { t: ToolItem }) {
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

export function SpawnDetailView({ t }: { t: ToolItem }) {
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

export function WebDetailView({ t }: { t: ToolItem }) {
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

export function AskDetailView({ t }: { t: ToolItem }) {
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

export function JSONDetail({ t, body }: { t: ToolItem; body: string }) {
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
export function ToolDetail({ t, body }: { t: ToolItem; body: string }) {
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
    case "keyword_search":
    case "semantic_search": // legacy journals (pre-rename)
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

export function ToolCard({ t }: { t: ToolItem }) {
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
      case "keyword_search":
      case "semantic_search": // legacy journals (pre-rename)
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
export function ActivityGroup({ run, sentImages, onContinue }: { run: FoldRun; sentImages?: Map<number, string[]>; onContinue?: (item: BubbleItem) => Promise<void> }) {
  const { tools, members } = run;
  const failed = tools.some((t) => t.status === "error" || t.status === "failed");
  return (
    <details className={"act-group" + (failed ? " error" : "")}>
      <summary>
        <span className="act-ic"><CaretRight size={12} className="act-caret" /></span>
        <span className="act-cat"><CategoryIcon cat={groupIcon(tools)} /></span>
        <span className="act-label">{groupLabel(tools)}</span>
        <span className="act-count" aria-label={`${tools.length} activities`}>×{tools.length}</span>
      </summary>
      <div className="act-body">
        {members.map((m) => (
          <Item it={m} sentImages={sentImages} onContinue={onContinue} key={m.key} />
        ))}
      </div>
    </details>
  );
}

// ---- CX-2: the collapsed head must never be information-free -----------------
// foldSpanMs measures a fold from its own children when the turn carried no
// duration (see workedLabel). Bubbles, runtime injections, tool activities
// (THREAD-2: ToolItem now carries its activity_started `ts`) and dated outcome
// chips all contribute a `ts`, so this recovers a span for folds that hold
// planning narration or bare tool work alike; only folds whose children are
// entirely undated return undefined. In practice branch ② of foldElapsedMs
// (startMs..endMs) almost always fires first, so this last-resort span is
// rarely reached — but including tool ts keeps it correct when it is.
function foldSpanMs(fold: WorkFold): number | undefined {
  let lo = Infinity;
  let hi = -Infinity;
  const take = (raw?: string) => {
    if (!raw) return;
    const at = new Date(raw).getTime();
    if (!Number.isFinite(at)) return;
    if (at < lo) lo = at;
    if (at > hi) hi = at;
  };
  for (const c of fold.children) {
    take((c as { ts?: string }).ts);
    // THREAD-2-SINGLESTEP · a tool also carries its END instant (endTs). A
    // single-step interrupted turn holds ONE tool: its start `ts` and its
    // completion `endTs` are the only dated instants, so both must count or
    // the span collapses to 0 and the head degrades to "Worked · 1 step".
    if (c.kind === "tool") take((c as { endTs?: string }).endTs);
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
// THREAD-2 · the real elapsed to show in the head, in priority order:
//  1. durationMs — a settled turn (user prompt → final answer). Unchanged.
//  2. startMs..endMs — an interrupted / stalled turn's real work-span, dated
//     from its first generation_started to its last activity (the same elapsed
//     the terminal alert shows as "00:34"). This is NOT fabricated: both
//     bounds are real journal timestamps.
//  3. foldSpanMs — a last-resort span measured off the fold's own dated
//     children (planning narration), for folds carrying neither of the above.
// Only when NONE yields a positive span does the head fall back to a step count.
function foldElapsedMs(fold: WorkFold): number | undefined {
  if (fold.durationMs !== undefined) return fold.durationMs;
  if (fold.startMs !== undefined && fold.endMs !== undefined) {
    const span = fold.endMs - fold.startMs;
    if (span > 0) return span;
  }
  return foldSpanMs(fold);
}

export function workedLabel(fold: WorkFold): string {
  const ms = foldElapsedMs(fold);
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
export function WorkedFold({
  fold,
  sentImages,
  open,
  onToggle,
  onContinue,
}: {
  fold: WorkFold;
  sentImages?: Map<number, string[]>;
  open: boolean;
  onToggle: () => void;
  onContinue?: (item: BubbleItem) => Promise<void>;
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
        rows.push(<ActivityGroup run={run} sentImages={sentImages} onContinue={onContinue} key={"g" + run.key} />);
      } else {
        for (const m of run.members) rows.push(<Item it={m} sentImages={sentImages} onContinue={onContinue} key={m.key} />);
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

// RetriedFold (INC-84): the block a Retry superseded — original message plus
// its failed turn — as one collapsed, expandable row. Mirrors WorkedFold's
// chrome; sys/turn markers inside the buried block stay hidden even expanded
// (they are journal plumbing, not content).
export function RetriedFold({
  fold,
  sentImages,
  open,
  onToggle,
  onContinue,
}: {
  fold: RetriedItem;
  sentImages?: Map<number, string[]>;
  open: boolean;
  onToggle: () => void;
  onContinue?: (item: BubbleItem) => Promise<void>;
}) {
  const children = fold.children.filter((c) => c.kind !== "sys" && c.kind !== "turn");
  if (!children.length) return null;
  return (
    <div className={"worked retried" + (open ? " open" : "")}>
      <button type="button" className="worked-row" onClick={onToggle} aria-expanded={open} title="This attempt failed and was retried; the journal keeps it — expand to review.">
        Failed attempt · retried
        <CaretRight size={15} className="worked-caret" />
      </button>
      {open && (
        <div className="worked-body">
          {children.map((m) => (
            <Item it={m} sentImages={sentImages} onContinue={onContinue} key={m.key} />
          ))}
        </div>
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
export function CollapsibleUserText({ text }: { text: string }) {
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

export function Item({ it, sentImages, last, deferActions, onContinue }: { it: TimelineItem; sentImages?: Map<number, string[]>; last?: boolean; deferActions?: boolean; onContinue?: (item: BubbleItem) => Promise<void> }) {
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
      const hasAttach = (thumbs && thumbs.length) || it.images || it.files;
      const attachNote = it.images ? (
        <div className="imgnote"><ImageSquare size={13} /> ×{it.images} attached</div>
      ) : null;
      return (
        <div className={"msg user" + (peer ? " peer" : "")} title={absTime(it.ts)} tabIndex={0}>
          <span className="who">
            {peer ? <>from {it.source} · <a href={"#" + it.peerSession}>open</a></> : it.source || "you"}
          </span>
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
              {it.files ? <div className="imgnote"><File size={13} /> ×{it.files} attached</div> : null}
            </div>
            {it.sentAsGoal && (
              <div className="cx-goal-note">
                <Lightning size={12} weight="fill" /> Sent as goal
              </div>
            )}
            <MsgActions text={it.text} ts={it.ts} onContinue={it.continueSide && onContinue ? () => onContinue(it) : undefined} />
          </div>
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
            {/* TAIL-ROW: on the thread's last answer, a settled turn hoists this
                action row out to the bottom of .tl-inner (past the artifact /
                changes cards) so Copy sits beside the goal verdict
                — see the tail row at the end of TimelineView. Middle answers keep
                their hover-only row inline. */}
            {!deferActions && <MsgActions text={it.text} ts={it.ts} onContinue={it.continueSide && onContinue ? () => onContinue(it) : undefined} />}
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
  let lastChip = -1; // index in `out` of that trailing mergeable chip
  for (const it of items) {
    // THREAD-2 · a hidden generation_started ("turn") marker is transparent to
    // a chip run: it is threaded through only so foldWork can date the turn, so
    // it must not break two identical chips out of their "×N" merge. Pass it
    // through without disturbing the run state.
    if (it.kind === "turn") {
      out.push(it);
      continue;
    }
    const prev = lastChip >= 0 ? out[lastChip] : undefined;
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
      out[lastChip] = { ...prev, text: `${it.text} ×${runCount}` };
      continue;
    }
    if (it.kind === "chip") {
      runCount = 1;
      lastChip = out.length;
    } else {
      runCount = 0;
      lastChip = -1;
    }
    out.push(it);
  }
  return out;
}

// Strips the "×N" suffix mergeAdjacentChips itself appended, so a third repeat
// still matches the chip it has already been merged into.
function baseChipText(text: string): string {
  return text.replace(/ ×\d+$/, "");
}

export interface TimelinePendingMessageModel {
  id: number;
  text: string;
  imgs: string[];
  files: number;
  delivery?: "steer" | "queue";
}

export function TimelinePendingMessage({
  message,
}: {
  message: TimelinePendingMessageModel;
}) {
  return (
    <div className="msg user">
      <div
        className={`bubble pending${
          message.delivery === "steer" ? " steer" : ""
        }`}
      >
        <CollapsibleUserText text={message.text} />
        {message.imgs.length ? <Thumbs paths={message.imgs} /> : null}
        {message.files ? (
          <div className="imgnote">
            <File size={13} /> ×{message.files} attached
          </div>
        ) : null}
      </div>
      <span className="who">
        {message.delivery === "steer" ? "steering…" : "queued…"}
      </span>
    </div>
  );
}

export function TimelineTailActions({
  lastAssistant,
  goalVerdict,
  onContinue,
}: {
  lastAssistant?: BubbleItem;
  goalVerdict?: { elapsed: string } | null;
  onContinue?: (item: BubbleItem) => Promise<void>;
}) {
  if (!lastAssistant && !goalVerdict) return null;
  return (
    <div className="tl-tail-row mt-3 flex flex-wrap items-center gap-3 [&_.msg-actions]:mt-0 [&_.turn-footer]:mt-0">
      {lastAssistant && (
        <MsgActions
          text={lastAssistant.text}
          onContinue={
            lastAssistant.continueSide && onContinue
              ? () => onContinue(lastAssistant)
              : undefined
          }
        />
      )}
      {lastAssistant && goalVerdict && (
        <span className="h-4 w-px bg-line" aria-hidden />
      )}
      {goalVerdict && (
        <div className="turn-footer">
          <CheckCircle size={15} /> Goal achieved in {goalVerdict.elapsed}
        </div>
      )}
    </div>
  );
}

export function TimelineJumpToLatest({
  unseen,
  onJump,
}: {
  unseen: number;
  onJump: () => void;
}) {
  const label =
    unseen > 0
      ? `${unseen} new update${unseen === 1 ? "" : "s"}`
      : "Jump to latest";
  return (
    <button
      type="button"
      className={`tl-jump${unseen > 0 ? " has-updates" : ""}`}
      onClick={onJump}
      title={unseen > 0 ? `${label} · Jump to latest` : label}
      aria-label={unseen > 0 ? `${label}; jump to latest` : label}
    >
      {unseen > 0 && (
        <span className="tl-jump-count">{unseen > 99 ? "99+" : unseen}</span>
      )}
      <CaretDown size={16} />
    </button>
  );
}

export function TimelineLoadingState() {
  return (
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
  );
}

export function TimelineEmptyState() {
  return (
    <div className="tl-empty">
      <ChatCircle size={26} weight="light" />
      <b>No messages yet</b>
      <span>This session hasn't started. Send a message below to begin.</span>
    </div>
  );
}

export interface TimelineViewProps {
  sessionKey?: string;
  items: TimelineItem[];
  pending: TimelinePendingMessageModel[];
  typing: string;
  showSys: boolean;
  sentImages?: Map<number, string[]>;
  statusLine?: ReactNode;
  approvalSlot?: ReactNode;
  active?: boolean;
  outcomeSlot?: ReactNode;
  /** When the run ended satisfied, the elapsed to show as an inline "Goal
   *  achieved in N" verdict on the final assistant answer's action row (fix 3).
   *  Undefined/null while the goal is unsettled or wasn't achieved. */
  goalVerdict?: { elapsed: string } | null;
  /** The first events fetch for this session hasn't returned yet (INC-41 L1). */
  loading?: boolean;
  onContinue?: (item: BubbleItem) => Promise<void>;
}

export function TimelineView({
  sessionKey,
  items,
  pending,
  typing,
  showSys,
  sentImages,
  statusLine,
  approvalSlot,
  active = false,
  outcomeSlot,
  goalVerdict,
  loading = false,
  onContinue,
}: TimelineViewProps) {
  // Codex shows a continuous activity feed — no "turn N" dividers, no raw
  // system events. Those stay behind the developer toggle.
  const visible = showSys
    ? items
    : items.filter((it) => it.kind !== "sys" && it.kind !== "turn" && it.kind !== "runtime");
  // THREAD-2 · foldWork needs the hidden generation_started ("turn") markers to
  // date each turn's work-span — an interrupted turn has no settled duration,
  // but its markers still fix when the work started and last ran. foldWork
  // CONSUMES them (they never reach the rendered output), so keeping them here
  // does not surface raw system events; runtime/sys stay filtered as before.
  const foldInput = showSys
    ? visible
    : items.filter((it) => it.kind !== "sys" && it.kind !== "runtime");
  const activityCount = visible.length + pending.length;
  const pendingCount = pending.length;
  const scroll = useTimelineScrollController({
    sessionKey,
    activityCount,
    pendingCount,
    loading,
  });
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
  const nodes: RenderNode[] = showSys ? visible : foldWork(mergeAdjacentChips(foldInput), durations, active);

  // The goal verdict rides the FINAL assistant answer only (fix 3) — a settled
  // run's last word. Assistant answers are turn boundaries, so they sit at the
  // top level of `nodes`, never folded into WorkedFold work.
  //
  // TH-21 reuses the same key: the final assistant answer is also the only
  // message that keeps its action row at rest (`.msg-last`).
  const lastAssistant = (() => {
    for (let i = nodes.length - 1; i >= 0; i--) {
      const n = nodes[i];
      if (n.kind === "assistant") return n;
    }
    return undefined;
  })();
  const lastAssistantKey = lastAssistant?.key;

  // TAIL-ROW: a settled turn (run idle, nothing typing/pending) hoists the final
  // answer's action row out of the bubble and down past the turn's artifact /
  // changes cards, so Copy lands on the same bottom row as the
  // goal verdict — matching Codex, which draws `⧉ … ↗ │ ⊘ Goal achieved in N`
  // AFTER the turn's full content. While the run is live the row stays inline
  // and persistent on `.msg-last` (TH-21), because the tail region below is
  // gated off until the turn settles.
  const settled = !active && !typing && pending.length === 0;
  const deferLastActions = settled && !!lastAssistant;

  // Nothing to show yet — but WHY decides what we render (INC-41 L1): while the
  // first fetch is still in flight the timeline is merely unknown, so claiming
  // "No messages yet" on a session with a long history is a lie the reader sees
  // for ~1s before it snaps away. Skeleton while loading; the empty state only
  // once we know the session really is empty (R4-11).
  const blank = nodes.length === 0 && !typing && pending.length === 0 && !statusLine && !approvalSlot;
  const isEmpty = blank && !loading;

  return (
    <TimelineContentView
      scroll={scroll}
      nodes={nodes}
      pending={pending}
      typing={typing}
      showSys={showSys}
      sentImages={sentImages}
      statusLine={statusLine}
      approvalSlot={approvalSlot}
      settled={settled}
      outcomeSlot={outcomeSlot}
      goalVerdict={goalVerdict}
      loading={loading}
      blank={blank}
      isEmpty={isEmpty}
      durations={durations}
      lastAssistant={lastAssistant}
      lastAssistantKey={lastAssistantKey}
      deferLastActions={deferLastActions}
      openFolds={openFolds}
      onToggleFold={toggleFold}
      onContinue={onContinue}
    />
  );
}

interface TimelineContentViewProps {
  scroll: TimelineScrollController;
  nodes: RenderNode[];
  pending: TimelinePendingMessageModel[];
  typing: string;
  showSys: boolean;
  sentImages?: Map<number, string[]>;
  statusLine?: ReactNode;
  approvalSlot?: ReactNode;
  settled: boolean;
  outcomeSlot?: ReactNode;
  goalVerdict?: { elapsed: string } | null;
  loading: boolean;
  blank: boolean;
  isEmpty: boolean;
  durations: Map<string, number>;
  lastAssistant?: BubbleItem;
  lastAssistantKey?: string;
  deferLastActions: boolean;
  openFolds: ReadonlySet<string>;
  onToggleFold: (id: string) => void;
  onContinue?: (item: BubbleItem) => Promise<void>;
}

function TimelineContentView({
  scroll,
  nodes,
  pending,
  typing,
  showSys,
  sentImages,
  statusLine,
  approvalSlot,
  settled,
  outcomeSlot,
  goalVerdict,
  loading,
  blank,
  isEmpty,
  durations,
  lastAssistant,
  lastAssistantKey,
  deferLastActions,
  openFolds,
  onToggleFold,
  onContinue,
}: TimelineContentViewProps) {
  // TR-1: has a user message already opened a turn above this one? Re-derived on
  // every render (the map below is a fresh closure), so it never carries state
  // across renders — it's a cursor over `nodes`, not view state.
  let seenUser = false;

  return (
    <div
      className="timeline"
      ref={scroll.viewportRef}
      onScroll={scroll.onScroll}
    >
      <div className="tl-inner">
        {blank && loading && (
          <TimelineLoadingState />
        )}
        {isEmpty && <TimelineEmptyState />}
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
          if (it.kind === "retried") {
            return (
              <RetriedFold
                fold={it}
                sentImages={sentImages}
                open={openFolds.has(it.key)}
                onToggle={() => onToggleFold(it.key)}
                onContinue={onContinue}
                key={it.key}
              />
            );
          }
          if (it.kind === "fold") {
            const foldId = it.children[0]?.key ?? it.key;
            return (
              <WorkedFold
                fold={it}
                sentImages={sentImages}
                open={openFolds.has(foldId)}
                onToggle={() => onToggleFold(foldId)}
                onContinue={onContinue}
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
                last={it.kind === "assistant" && it.key === lastAssistantKey}
                deferActions={it.kind === "assistant" && it.key === lastAssistantKey && deferLastActions}
                onContinue={onContinue}
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
          <TimelinePendingMessage message={p} key={`p${p.id}`} />
        ))}
        {settled && outcomeSlot}
        {/* TAIL-ROW: the final answer's Copy action is hoisted out of the bubble
            (deferLastActions above) and rendered HERE, past
            outcomeSlot, on the same bottom row as the goal verdict:
            `⧉ │ ⊘ Goal achieved in N`. The action row carries no timestamp
            (matching the persistent last row; `ts` is intentionally not passed).
            If the turn hasn't been judged, the action row still renders here at
            the bottom, just without the goal badge. */}
        {settled && (lastAssistant || goalVerdict) && (
          <TimelineTailActions
            lastAssistant={lastAssistant}
            goalVerdict={goalVerdict}
            onContinue={onContinue}
          />
        )}
      </div>
      {scroll.showJump && (
        <TimelineJumpToLatest
          unseen={scroll.unseen}
          onJump={scroll.jumpToBottom}
        />
      )}
    </div>
  );
}
