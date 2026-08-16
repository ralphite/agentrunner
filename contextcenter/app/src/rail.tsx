import { useRef, useState, type ReactNode } from "react";
import { useEffect } from "react";
import {
  Asterisk, ArrowUp, Bookmark, Check, CheckCircle2, ChevronDown, Copy, ExternalLink, Lightbulb,
  Paperclip, Pause, Plus, RefreshCw, Send, Sparkles, Square, Trash2, ChevronRight,
} from "lucide-react";
import { STUBS, type TaskStatus, type DocStatus } from "./app-data";
import { DocIcon } from "./icons";
import { useApp, taskPill, type Statuses } from "./ctx";
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem,
} from "@/components/ui/dropdown-menu";

/* ── 小件 ── */
const Field = ({ name, children, mono }: { name: string; children: ReactNode; mono?: boolean }) => (
  <div className="field">
    <span className="fname">{name}</span>
    <span className={`fval ${mono ? "mono" : ""}`}>{children}</span>
  </div>
);
const ISec = ({ children }: { children: ReactNode }) => <div className="isec">{children}</div>;

const docPillCls: Record<DocStatus, string> = { Active: "prog", Paused: "plain", Archived: "plain" };

function StatusDropdown<T extends string>({ value, cls, options, onChange }: {
  value: T; cls: string; options: readonly T[]; onChange: (v: T) => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button className={`pill ${cls} dd`}>{value} <ChevronDown size={10} strokeWidth={2} /></button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-[140px]">
        {options.map((o) => (
          <DropdownMenuItem key={o} onSelect={() => onChange(o)}>{o}</DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

const TASK_OPTIONS = ["Todo", "In progress", "Done", "Blocked"] as const;
const DOC_OPTIONS = ["Active", "Paused", "Archived"] as const;

function TaskStatusPill({ id }: { id: "task-43" | "bug-142" }) {
  const { statuses, setStatus } = useApp();
  const v = statuses[id];
  return (
    <StatusDropdown value={v} cls={taskPill[v].cls} options={TASK_OPTIONS}
      onChange={(nv) => setStatus(id, nv as TaskStatus)} />
  );
}
function DocStatusPill({ id }: { id: "session-recovery" | "impl-plan" }) {
  const { statuses, setStatus } = useApp();
  const v = statuses[id];
  return (
    <StatusDropdown value={v} cls={docPillCls[v]} options={DOC_OPTIONS}
      onChange={(nv) => setStatus(id, nv as DocStatus)} />
  );
}

const Action = ({ icon, children, destructive }: { icon: ReactNode; children: ReactNode; destructive?: boolean }) => (
  <div className="action" style={destructive ? { color: "var(--red-ink)" } : undefined}>
    <span className="ticon" style={{ display: "grid", placeItems: "center" }}>{icon}</span>
    {children}
  </div>
);

function SRow({ children, to, right }: { children: ReactNode; to?: string; right?: ReactNode }) {
  const { openDoc } = useApp();
  return (
    <div className={`srow ${to ? "link" : ""}`} onClick={to ? () => openDoc(to) : undefined}>
      {children}
      {right && <span className="right">{right}</span>}
    </div>
  );
}

/* ── Metadata 面板 ── */
export function MetadataPanel({ doc }: { doc: string }) {
  const { statuses, openDoc } = useApp();
  const i = (n: Parameters<typeof DocIcon>[0]["name"]) => <DocIcon name={n} size={14} />;

  switch (doc) {
    case "aurora-ide":
      return (
        <>
          <Field name="Name">Aurora IDE</Field>
          <Field name="Type"><span className="pill plain">Project</span></Field>
          <Field name="Workspace" mono>~/dev/aurora-ide</Field>
          <Field name="Agents">Codex · Claude Code</Field>
          <Field name="Default agent">Codex</Field>
          <Field name="Created">Apr 28, 2025</Field>
          <Field name="Updated">May 12, 2025</Field>
          <ISec>Actions</ISec>
          <Action icon={<ExternalLink size={15} strokeWidth={1.5} />}>Open workspace</Action>
          <Action icon={<Sparkles size={15} strokeWidth={1.5} />}>Configure agents</Action>
        </>
      );
    case "session-recovery": {
      const s43 = statuses["task-43"];
      return (
        <>
          <Field name="Type"><span className="pill plain">Workstream</span></Field>
          <Field name="Status"><DocStatusPill id="session-recovery" /></Field>
          <Field name="Tags"><span className="pill plain">reliability</span><span className="pill plain">recovery</span></Field>
          <Field name="Created">May 2, 2025</Field>
          <Field name="Updated">May 12, 2025</Field>
          <ISec>Tasks</ISec>
          <div className="stack">
            <SRow right={<span className="tid">#42</span>}>
              <span className="donec" style={{ width: 14, height: 14 }}><Check size={9} strokeWidth={2.75} /></span>
              Define recovery data model
            </SRow>
            <SRow to="task-43" right={<span className="tid">#43</span>}>
              {s43 === "In progress"
                ? <span className="spinner" style={{ width: 12, height: 12, borderWidth: 1.5 }} />
                : s43 === "Done"
                  ? <span className="donec" style={{ width: 14, height: 14 }}><Check size={9} strokeWidth={2.75} /></span>
                  : <span className="radio" style={{ width: 14, height: 14 }} />}
              Implement automatic recovery
            </SRow>
            <SRow right={<span className="tid">#44</span>}>
              <span className="radio" style={{ width: 14, height: 14 }} />
              Handle multi-window sessions
            </SRow>
          </div>
          <ISec>Actions</ISec>
          <Action icon={<Send size={15} strokeWidth={1.5} />}>Delegate to Codex</Action>
          <Action icon={<Plus size={15} strokeWidth={1.5} />}>Add task</Action>
        </>
      );
    }
    case "task-43":
      return (
        <>
          <Field name="ID">#43</Field>
          <Field name="Type"><span className="pill plain">Task</span></Field>
          <Field name="Status"><TaskStatusPill id="task-43" /></Field>
          <Field name="Attempts">3</Field>
          <Field name="Best attempt"><span className="pill promising">Attempt 3 · Promising</span></Field>
          <Field name="Depends on">#42</Field>
          <Field name="Created">May 9, 2025</Field>
          <Field name="Updated">May 12, 2025</Field>
          <ISec>Extracted lesson</ISec>
          <div className="lesson-mini">
            <Lightbulb size={14} strokeWidth={1.5} className="ticon" />
            <span>Use streaming restore for large files with incremental state hydration; verify before marking restore complete.</span>
          </div>
          <ISec>Related docs</ISec>
          <div className="stack">
            <SRow to="impl-plan">{i("plan")}Implementation Plan</SRow>
            <SRow>{i("page")}Streaming Restore Design</SRow>
            <SRow to="recovery-ux">{i("page")}Recovery UX</SRow>
          </div>
          <ISec>Actions</ISec>
          <Action icon={<CheckCircle2 size={15} strokeWidth={1.5} />}>Mark as done</Action>
          <Action icon={<RefreshCw size={15} strokeWidth={1.5} />}>Retry with Codex</Action>
          <Action icon={<Bookmark size={15} strokeWidth={1.5} />}>Save lesson</Action>
          <Action icon={<Trash2 size={15} strokeWidth={1.5} />} destructive>Delete task</Action>
        </>
      );
    case "impl-plan":
      return (
        <>
          <Field name="Type"><span className="pill plain">Plan</span></Field>
          <Field name="Status"><DocStatusPill id="impl-plan" /></Field>
          <Field name="Created">May 6, 2025</Field>
          <Field name="Updated">May 12, 2025</Field>
          <ISec>Loops</ISec>
          <div className="stack">
            <SRow to="loop-mvp" right={<span className="pill running">Running</span>}>
              <RefreshCw size={14} strokeWidth={1.5} className="spin" style={{ color: "var(--accent)" }} />
              Recovery MVP Loop
            </SRow>
          </div>
          <ISec>Actions</ISec>
          <Action icon={<Send size={15} strokeWidth={1.5} />}>Delegate to Codex</Action>
          <Action icon={<RefreshCw size={15} strokeWidth={1.5} />}>Insert loop</Action>
        </>
      );
    case "loop-mvp":
      return (
        <>
          <Field name="Type"><span className="pill plain">Loop</span></Field>
          <Field name="Status"><span className="pill running">Running</span></Field>
          <Field name="Current">
            <span className="linkish" onClick={() => openDoc("task-43")}>#43 Implement automatic recovery</span>
          </Field>
          <Field name="Strategy">Sequential</Field>
          <Field name="On failure">Pause — investigate, fix, retry</Field>
          <Field name="Save lessons">Yes</Field>
          <Field name="Stop when">All tasks completed</Field>
          <ISec>Lessons</ISec>
          <div className="stack">
            <SRow><Lightbulb size={14} strokeWidth={1.5} style={{ color: "var(--amber-ink)" }} />Session Recovery Lessons</SRow>
          </div>
          <ISec>Actions</ISec>
          <Action icon={<Pause size={15} strokeWidth={1.5} />}>Pause loop</Action>
          <Action icon={<Square size={13} strokeWidth={1.5} />}>Stop loop</Action>
        </>
      );
    case "bug-backlog":
      return (
        <>
          <Field name="Type"><span className="pill plain">Backlog</span></Field>
          <Field name="Bugs">5 active · 3 resolved</Field>
          <Field name="Tags"><span className="pill plain">quality</span></Field>
          <Field name="Created">Apr 30, 2025</Field>
          <Field name="Updated">May 12, 2025</Field>
          <ISec>Loops</ISec>
          <div className="stack">
            <SRow to="loop-bugfix" right={<span className="pill running">Running</span>}>
              <RefreshCw size={14} strokeWidth={1.5} className="spin" style={{ color: "var(--accent)" }} />
              Codex Bug Fix Loop
            </SRow>
          </div>
          <ISec>Actions</ISec>
          <Action icon={<Plus size={15} strokeWidth={1.5} />}>Add bug</Action>
          <Action icon={<ExternalLink size={15} strokeWidth={1.5} />}>Open loop run</Action>
        </>
      );
    case "bug-142":
      return (
        <>
          <Field name="ID">#BUG-142</Field>
          <Field name="Type"><span className="pill plain">Bug</span></Field>
          <Field name="Status"><TaskStatusPill id="bug-142" /></Field>
          <Field name="Priority"><span className="pill p1">P1 · High</span></Field>
          <Field name="Found in">Session Recovery</Field>
          <Field name="Environment">All (Cloud, Self-hosted)</Field>
          <Field name="Attempts">3</Field>
          <Field name="Best attempt"><span className="pill promising">Attempt 3 · Promising</span></Field>
          <Field name="Created">May 10, 2025</Field>
          <Field name="Updated">May 12, 2025</Field>
          <ISec>Actions</ISec>
          <Action icon={<Send size={15} strokeWidth={1.5} />}>Delegate to Codex</Action>
          <Action icon={<CheckCircle2 size={15} strokeWidth={1.5} />}>Mark fixed</Action>
          <Action icon={<Lightbulb size={15} strokeWidth={1.5} />}>Create lesson</Action>
          <Action icon={<Copy size={15} strokeWidth={1.5} />}>Duplicate bug</Action>
          <Action icon={<Trash2 size={15} strokeWidth={1.5} />} destructive>Archive bug</Action>
        </>
      );
    case "loop-bugfix":
      return (
        <>
          <Field name="Type"><span className="pill plain">Loop</span></Field>
          <Field name="Status"><span className="pill running">Running</span></Field>
          <Field name="Current"><span className="linkish" onClick={() => openDoc("bug-142")}>#BUG-142</span></Field>
          <Field name="Progress">1 of 5 · Est. 3–6 min</Field>
          <Field name="Strategy">Sequential</Field>
          <Field name="On failure">Record lesson, continue</Field>
          <ISec>Actions</ISec>
          <Action icon={<Pause size={15} strokeWidth={1.5} />}>Pause loop</Action>
          <Action icon={<Square size={13} strokeWidth={1.5} />}>Stop loop</Action>
          <Action icon={<ExternalLink size={15} strokeWidth={1.5} />}>View run</Action>
        </>
      );
    default: {
      const s = STUBS[doc];
      if (!s) return null;
      return (
        <>
          {s.fields.map(([k, v]) => (
            <Field key={k} name={k} mono={k === "Workspace"}>
              {k === "Type" && v !== "—" ? <span className="pill plain">{v}</span>
                : k === "Status" ? <span className={`pill ${v === "Active" ? "prog" : "plain"}`}>{v}</span>
                : k === "Tags" ? v.split(",").map((t) => <span key={t} className="pill plain">{t.trim()}</span>)
                : v}
            </Field>
          ))}
        </>
      );
    }
  }
}

/* ── Chat ── */
export interface ChatMsg {
  who: "you" | "codex";
  ago: string;
  quote?: string;
  text: ReactNode;
  proposal?: boolean;
}

const THREADS: Record<string, ChatMsg[]> = {
  "aurora-ide": [
    { who: "you", ago: "1h ago", text: "What moved this week?" },
    { who: "codex", ago: "1h ago", text: <>Session Recovery advanced: attempt 3 restores large files via streaming. Verification before completion is the remaining gap — I captured it as a lesson on task #43.</> },
  ],
  "session-recovery": [
    { who: "you", ago: "2m ago", quote: "Gaps remain around multi-window scenarios, extension state, and large file recovery.", text: "What's left before we can close the large-file gap?" },
    { who: "codex", ago: "1m ago", text: <>Attempt 3 on <b>#43</b> restores large files via streaming. The remaining gap is verification before marking restore complete — it's captured as a lesson and noted on the task.</> },
  ],
  "task-43": [
    { who: "you", ago: "2m ago", text: "Can you refine this task and strengthen the acceptance criteria?" },
    { who: "codex", ago: "1m ago", proposal: true, text: "Sure. I've refined the task to clarify scope and added concrete acceptance criteria covering restore triggers, data integrity, and error handling." },
  ],
  "impl-plan": [
    { who: "you", ago: "10m ago", quote: "M3: Add end-to-end tests", text: "Expand this milestone into concrete test scenarios before the loop reaches it." },
    { who: "codex", ago: "9m ago", text: "I'll draft scenarios covering crash, reload, restart and multi-window restore, and attach them to M3 before task 3 starts." },
  ],
  "loop-mvp": [
    { who: "you", ago: "2m ago", text: "Continue through the remaining tasks in order. If any step fails, pause, record lessons, and then proceed once resolved." },
    { who: "codex", ago: "1m ago", text: "Got it. I'll proceed through the loop sequentially, pause on any failure, and record lessons from failed attempts." },
  ],
  "bug-backlog": [
    { who: "you", ago: "5m ago", quote: "Deployment pipeline fails on large monorepos", text: "Run this right after the current fix lands." },
    { who: "codex", ago: "4m ago", text: <>Queued <b>#BUG-139</b> directly after <b>#BUG-142</b>. The loop follows the Active Bugs order on this page, so I moved its line up.</> },
  ],
  "bug-142": [
    { who: "you", ago: "3m ago", quote: "restores a session snapshot that predates the disconnect", text: "Is 30s the right invalidation threshold?" },
    { who: "codex", ago: "2m ago", text: "Local telemetry shows gaps over 22s already correlate with stale restores. I'd drop the threshold to 20s and re-run attempt 3's test matrix to confirm." },
  ],
  "loop-bugfix": [
    { who: "you", ago: "2m ago", text: "Keep iterating through the bug list. For failed attempts, save reusable lessons so we can avoid repeating the same dead ends." },
    { who: "codex", ago: "1m ago", text: "Got it. I'll continue working through the active bugs in order and capture lessons from failed attempts for reuse." },
  ],
};

const PROPOSAL_TEXT = `Scope: Restore the last valid session on startup after crashes, reloads, or system restarts.

Acceptance Criteria:
- Session restores automatically within 3s on startup in 95% of cases
- All open files, cursor positions, and UI layout are restored accurately
- Corrupted state is detected and a safe fallback is used
- Telemetry event \`session_restore\` is emitted with outcome`;

function Message({ m }: { m: ChatMsg }) {
  const [applied, setApplied] = useState(false);
  return (
    <div className="msg">
      <span className={`avatar ${m.who}`}>
        {m.who === "you" ? "You" : <Asterisk size={13} strokeWidth={2.25} />}
      </span>
      <div className="body">
        <div className="who">{m.who === "you" ? "You" : "Codex"} <span className="ago">{m.ago}</span></div>
        <div className="text">
          {m.quote && <div className="quote">{m.quote}</div>}
          {m.text}
          {m.proposal && (
            <div className="proposed">
              <div className="prop-head">Proposed update <span className="pill preview">Preview</span></div>
              <pre>{PROPOSAL_TEXT}</pre>
              <div className="prop-actions">
                <button className="btn sm">Copy</button>
                <button className={`btn sm ${applied ? "applied" : "primary"}`} onClick={() => setApplied(true)}>
                  {applied ? "Applied ✓" : "Apply update to document"}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export function ChatPanel({ doc, extra, draft, setDraft, taRef }: {
  doc: string;
  extra: ChatMsg[];
  draft: string;
  setDraft: (v: string) => void;
  taRef: React.RefObject<HTMLTextAreaElement>;
}) {
  const { agent, setAgent } = useApp();
  useEffect(() => {
    if (draft === "" && taRef.current) taRef.current.style.height = "auto";
  }, [draft, taRef]);
  const base = THREADS[doc] ?? [];
  const all = [...base, ...extra];
  const empty = all.length === 0;
  const canSend = draft.trim().length > 0;
  const sendRef = useRef<() => void>(() => {});
  sendRef.current = () => {
    if (!canSend) return;
    const lines = draft.split("\n");
    const quoteLines = [];
    let i = 0;
    while (i < lines.length && lines[i].startsWith("> ")) { quoteLines.push(lines[i].slice(2)); i++; }
    const rest = lines.slice(i).join("\n").trim();
    window.dispatchEvent(new CustomEvent("cc-send", {
      detail: { doc, msg: { who: "you", ago: "now", quote: quoteLines.join("\n") || undefined, text: rest || "…" } },
    }));
    setDraft("");
  };

  return (
    <div className="rail-chat">
      <div className="rail-head">Chat</div>
      <div className="chat-thread">
        {empty
          ? <div className="chat-empty">No messages yet.<br />Select anything on the page and quote it here, or just ask.</div>
          : all.map((m, idx) => <Message key={idx} m={m} />)}
      </div>
      <div className="chat-input">
        <div className="composer-box">
          <textarea
            ref={taRef}
            rows={1}
            placeholder="Ask, or select text to quote..."
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value);
              e.target.style.height = "auto";
              e.target.style.height = Math.min(130, e.target.scrollHeight) + "px";
            }}
          />
          <div className="composer-row">
            <button className="iconbtn" title="Attach"><Paperclip size={15} strokeWidth={1.5} /></button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <span className="agentsel"><b>{agent}</b> <ChevronRight size={10} strokeWidth={2} style={{ transform: "rotate(90deg)" }} /></span>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="min-w-[150px]">
                <DropdownMenuItem onSelect={() => setAgent("Codex")}>Codex</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setAgent("Claude Code")}>Claude Code</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <span style={{ flex: 1 }} />
            <button
              className={`sendbtn ${canSend ? "" : "empty"}`}
              onClick={() => sendRef.current()}
              title="Send"
            >
              <ArrowUp size={14} strokeWidth={2} />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
