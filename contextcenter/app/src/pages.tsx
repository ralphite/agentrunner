import type { ReactNode } from "react";
import { Check, Lightbulb, RefreshCw, Pause, Bookmark } from "lucide-react";
import { STUBS, type IconName } from "./app-data";
import { DocIcon } from "./icons";
import { useApp, taskPill } from "./ctx";

/* ── 共享原语 ── */
const ed = { contentEditable: true, suppressContentEditableWarning: true, spellCheck: false } as const;

export const Sec = ({ children }: { children: ReactNode }) => <h2 className="sec" {...ed}>{children}</h2>;
const P = ({ children }: { children: ReactNode }) => <p {...ed}>{children}</p>;
const Li = ({ children }: { children: ReactNode }) => <li {...ed}>{children}</li>;

export function StateDot({ state }: { state: "done" | "prog" | "todo" }) {
  if (state === "done") return <span className="donec"><Check size={10} strokeWidth={2.5} /></span>;
  if (state === "prog") return <span className="spinner" />;
  return <span className="radio" />;
}

function DocLink({ icon, label, to, color }: { icon: IconName; label: string; to?: string; color?: string }) {
  const { openDoc } = useApp();
  return (
    <div className="doclink" onClick={to ? () => openDoc(to) : undefined}>
      <DocIcon name={icon} />
      <span className="label" style={color ? { color } : undefined}>{label}</span>
    </div>
  );
}

function LessonLink({ label }: { label: string }) {
  return (
    <div className="doclink">
      <Lightbulb size={16} strokeWidth={1.5} style={{ color: "var(--amber-ink)", flex: "none" }} />
      <span className="label">{label}</span>
    </div>
  );
}

interface TaskRowProps {
  state: "done" | "prog" | "todo";
  title: string;
  id?: string;
  pill?: { cls: string; label: string };
  prePill?: { cls: string; label: string };
  to?: string;
  num?: string;
}
function TaskRow({ state, title, id, pill, prePill, to, num }: TaskRowProps) {
  const { openDoc } = useApp();
  return (
    <div className={`task-row ${to ? "link" : ""}`} onClick={to ? () => openDoc(to) : undefined}>
      {num !== undefined && <span className="tnum">{num}</span>}
      <StateDot state={state} />
      <span className={`t ${state === "done" ? "done" : ""}`}>{title}</span>
      {prePill && <><span className="tid" /><span className={`pill ${prePill.cls}`}>{prePill.label}</span></>}
      {id && <span className="tid">{id}</span>}
      {pill && <span className={`pill ${pill.cls}`}>{pill.label}</span>}
    </div>
  );
}

// #43 的行在多处出现，状态统一从 ctx 读（结构化状态单一来源的演示）
function Task43Row({ num, id = "#43", to = "task-43" }: { num?: string; id?: string; to?: string }) {
  const { statuses } = useApp();
  const s = statuses["task-43"];
  const state = s === "Done" ? "done" : s === "In progress" ? "prog" : "todo";
  return <TaskRow state={state} title="Implement automatic recovery" id={id} pill={taskPill[s]} to={to} num={num} />;
}

export function AttemptRow(props: { n: string; cls: string; label: string; date: string; summary: string; sel?: boolean; best?: boolean }) {
  return (
    <div className={`attempt-row ${props.sel ? "sel" : ""}`}>
      <span className="an">{props.n}</span>
      <span className={`pill ${props.cls}`}>{props.label}</span>
      <span className="ad">{props.date}</span>
      <span className="as">{props.summary}</span>
      {props.best && <span className="pill best">Best</span>}
    </div>
  );
}

function Callout({ children }: { children: ReactNode }) {
  return (
    <div className="callout">
      <Lightbulb size={16} strokeWidth={1.5} className="ticon" />
      <span {...ed}>{children}</span>
    </div>
  );
}

/* ── 页面 ── */

function AuroraIde() {
  return (
    <div className="content">
      <h1 className="title" {...ed}>Aurora IDE</h1>
      <p className="subtitle" {...ed}>Project memory for the Aurora desktop editor. Everything below is an editable document.</p>
      <P>This page is the root the agents start from: they read it first, then follow links into the documents they need. Keep the pointers below current.</P>
      <Sec>Documents</Sec>
      <div>
        <DocLink icon="page" label="Overview" to="aurora-overview" />
        <DocLink icon="ws" label="Editor Performance" to="editor-performance" />
        <DocLink icon="ws" label="AI Assistant" to="ai-assistant" />
        <DocLink icon="ws" label="Session Recovery" to="session-recovery" />
        <DocLink icon="page" label="Bugs" to="aurora-bugs" />
        <DocLink icon="page" label="Notes" to="aurora-notes" />
        <DocLink icon="page" label="Plans" to="aurora-plans" />
      </div>
      <Sec>Now</Sec>
      <ul>
        <Li>Session Recovery is the active workstream — task <b>#43</b> in progress, attempt 3 promising.</Li>
        <Li>Verification before marking restore complete is the open gap; lesson captured.</Li>
      </ul>
    </div>
  );
}

function SessionRecovery() {
  return (
    <div className="content">
      <h1 className="title" {...ed}>Session Recovery</h1>
      <p className="subtitle" {...ed}>Improve reliability by restoring work after crashes, reloads, or system restarts.</p>
      <Sec>Goal</Sec>
      <P>Automatically recover the user's last active session so they can continue where they left off with minimal friction.</P>
      <Sec>Current Understanding</Sec>
      <P>We persist editor state to disk on key events and on a debounce. On startup, we attempt to restore the last valid state. Gaps remain around multi-window scenarios, extension state, and large file recovery.</P>
      <Sec>Linked docs</Sec>
      <div>
        <DocLink icon="plan" label="Implementation Plan" to="impl-plan" />
        <DocLink icon="research" label="Research Notes" to="research-notes" />
        <DocLink icon="page" label="Recovery UX" to="recovery-ux" />
      </div>
      <Sec>Tasks</Sec>
      <div>
        <TaskRow state="done" title="Define recovery data model" id="#42" pill={{ cls: "done", label: "Done" }} />
        <Task43Row />
        <TaskRow state="todo" title="Handle multi-window sessions" id="#44" pill={{ cls: "todo", label: "Todo" }} />
      </div>
      <Sec>Open Questions</Sec>
      <ul>
        <Li>How do we reconcile conflicts between remote and local session state?</Li>
      </ul>
    </div>
  );
}

function Task43() {
  return (
    <div className="content">
      <h1 className="title" {...ed}>Implement automatic recovery</h1>
      <p className="subtitle" {...ed}>Restore the last valid session on startup after crashes, reloads, or system restarts.</p>
      <Sec>Acceptance Criteria</Sec>
      <ul>
        <Li>Session restores automatically within 3s on startup in 95% of cases</Li>
        <Li>All open files, cursor positions, and UI layout are restored accurately</Li>
        <Li>Corrupted state is detected and a safe fallback is used</Li>
        <Li>Telemetry event <code className="inlinecode">session_restore</code> is emitted with outcome</Li>
      </ul>
      <Sec>Attempt History</Sec>
      <div className="attempts">
        <AttemptRow n="Attempt 1" cls="failed" label="Failed" date="May 10, 2025" summary="Restored state missing after hard crash (power loss)." />
        <AttemptRow n="Attempt 2" cls="partial" label="Partial" date="May 11, 2025" summary="Restores small files; large files exceed timeout." />
        <AttemptRow n="Attempt 3" cls="promising" label="Promising" date="May 12, 2025" summary="Handles large files via streaming; minor gaps remain." sel best />
      </div>
      <Sec>Lessons</Sec>
      <Callout>Use streaming restore for large files with incremental state hydration. Add a verification step before marking restore complete to prevent partial sessions.</Callout>
    </div>
  );
}

function ImplPlan() {
  const { openDoc } = useApp();
  return (
    <div className="content">
      <h1 className="title" {...ed}>Implementation Plan</h1>
      <Sec>Goal</Sec>
      <P>Restore the last valid session state automatically after crashes, reloads, or system restarts so users can continue where they left off with minimal friction.</P>
      <Sec>Milestones</Sec>
      <ul>
        <Li><span className="mlabel">M1:</span> Define recovery data model</Li>
        <Li><span className="mlabel">M2:</span> Implement automatic recovery</Li>
        <Li><span className="mlabel">M3:</span> Add end-to-end tests</Li>
        <Li><span className="mlabel">M4:</span> Update docs</Li>
      </ul>
      <Sec>Execution Loop</Sec>
      <P>We will iterate in a tight loop to build and validate the recovery MVP. The loop below is itself a document — its queue is just an ordered list of task links.</P>
      <div className="widget">
        <div className="widget-head" onClick={() => openDoc("loop-mvp")}>
          <RefreshCw size={16} strokeWidth={1.5} className="ticon spin" />
          <span className="label">Recovery MVP Loop</span>
        </div>
        <WidgetQueueRows compact />
        <div className="widget-foot">
          <span><RefreshCw size={14} strokeWidth={1.5} className="ticon" />Run sequentially</span>
          <span><Pause size={14} strokeWidth={1.5} className="ticon" />Pause on failure</span>
          <span><Bookmark size={14} strokeWidth={1.5} className="ticon" />Save lessons from failed attempts</span>
        </div>
      </div>
    </div>
  );
}

function WidgetQueueRows({ compact }: { compact?: boolean }) {
  const { statuses, openDoc } = useApp();
  const s43 = statuses["task-43"];
  const state43 = s43 === "Done" ? "done" : s43 === "In progress" ? "prog" : "todo";
  const Row = ({ num, state, title, pill, to }: { num: string; state: "done" | "prog" | "todo"; title: string; pill: { cls: string; label: string }; to?: string }) => (
    <div className="widget-row">
      <span className="tnum">{num}</span>
      <StateDot state={state} />
      <span className={`t ${state === "done" ? "done" : ""} ${to ? "link" : ""}`} onClick={to ? () => openDoc(to) : undefined}>{title}</span>
      <span className={`pill ${pill.cls}`}>{pill.label}</span>
    </div>
  );
  void compact;
  return (
    <>
      <Row num="1" state="done" title="Define recovery data model" pill={{ cls: "done", label: "Done" }} />
      <Row num="2" state={state43} title="Implement automatic recovery" pill={taskPill[s43]} to="task-43" />
      <Row num="3" state="todo" title="Add end-to-end tests" pill={{ cls: "queued", label: "Queued" }} />
      <Row num="4" state="todo" title="Update docs" pill={{ cls: "queued", label: "Queued" }} />
    </>
  );
}

function LoopMvp() {
  return (
    <div className="content">
      <h1 className="title" {...ed}>Recovery MVP Loop</h1>
      <p className="subtitle" {...ed}>Sequential loop over the recovery tasks in Implementation Plan.</p>
      <P>The queue is an ordered list of task links — reorder by moving lines. Configuration lives in this document's frontmatter; runtime state lives in the companion store.</P>
      <Sec>Queue</Sec>
      <div>
        <TaskRow num="1" state="done" title="Define recovery data model" id="#42" pill={{ cls: "done", label: "Done" }} />
        <Task43Row num="2" />
        <TaskRow num="3" state="todo" title="Add end-to-end tests" id="#45" pill={{ cls: "queued", label: "Queued" }} />
        <TaskRow num="4" state="todo" title="Update docs" id="#46" pill={{ cls: "queued", label: "Queued" }} />
      </div>
      <Sec>Behavior</Sec>
      <ul>
        <Li>Run tasks sequentially; one delegated session at a time.</Li>
        <Li>Pause on failure — investigate, fix, then retry the failed task.</Li>
        <Li>Save lessons from failed attempts back to Session Recovery.</Li>
      </ul>
    </div>
  );
}

function BugBacklog() {
  const { openDoc, statuses } = useApp();
  const s = statuses["bug-142"];
  const state142 = s === "Done" ? "done" : s === "In progress" ? "prog" : "todo";
  return (
    <div className="content">
      <h1 className="title" {...ed}>Bug Backlog</h1>
      <Sec>Overview</Sec>
      <P>A living list of product issues discovered in Atlas Deploy. We track active bugs, resolution progress, and the learnings we capture along the way.</P>
      <Sec>Active Bugs</Sec>
      <div>
        <TaskRow state={state142} title="Recovery sometimes restores stale context after reconnect" prePill={{ cls: "p1", label: "P1" }} id="#BUG-142" to="bug-142" />
        <TaskRow state="todo" title="Deployment pipeline fails on large monorepos" prePill={{ cls: "p1", label: "P1" }} id="#BUG-139" />
        <TaskRow state="todo" title="Agent logs missing for cancelled runs" prePill={{ cls: "p2", label: "P2" }} id="#BUG-137" />
        <TaskRow state="todo" title="Rollback leaves orphaned resources in rare cases" prePill={{ cls: "p2", label: "P2" }} id="#BUG-131" />
        <TaskRow state="todo" title="UI flicker when switching environments quickly" prePill={{ cls: "p3", label: "P3" }} id="#BUG-129" />
      </div>
      <Sec>Resolved Bugs</Sec>
      <div>
        <TaskRow state="done" title="Intermittent auth errors during deploy" id="#BUG-136" />
        <TaskRow state="done" title="Webhook delivery duplicates under load" id="#BUG-128" />
        <TaskRow state="done" title="CLI hangs on large plan outputs" id="#BUG-124" />
      </div>
      <Sec>Codex Bug Fix Loop</Sec>
      <div className="loopcard">
        <div className="lc-head">
          <RefreshCw size={16} strokeWidth={1.5} className="ticon spin" />
          <span className="lc-desc">
            <span className="label" onClick={() => openDoc("loop-bugfix")}>Codex Bug Fix Loop</span>
            {" "}— continuously working through active bugs one by one and writing results back to this page.
          </span>
          <span className="pill running">Running</span>
        </div>
        <div className="progress"><i /></div>
        <div className="lc-meta">
          <span>Working on 1 of 5</span>
          <span style={{ flex: 1 }} />
          <span>Est. 3–6 min</span>
          <button className="btn sm" onClick={() => openDoc("loop-bugfix")}>View run</button>
        </div>
        <div className="lc-meta" style={{ marginTop: 6 }}>
          <span className="cur">Recovery sometimes restores stale context after reconnect</span>
        </div>
      </div>
      <Sec>Learnings</Sec>
      <div>
        <LessonLink label="Flaky Reconnects: Context Invalidation Patterns" />
        <LessonLink label="Safe State Restoration Checklist" />
      </div>
    </div>
  );
}

function Bug142() {
  return (
    <div className="content">
      <h1 className="title" {...ed}>Recovery sometimes restores stale context after reconnect</h1>
      <Sec>What happens</Sec>
      <P>After a network reconnect, the recovery flow occasionally restores a session snapshot that predates the disconnect, so users see stale context.</P>
      <ul>
        <Li>First seen in Session Recovery testing, May 10.</Li>
        <Li>Reproduces roughly 1 in 12 reconnects when the network gap exceeds 30s.</Li>
      </ul>
      <Sec>Fix Attempts</Sec>
      <div className="attempts">
        <AttemptRow n="Attempt 1" cls="failed" label="Failed" date="May 10, 2025" summary="Freshness check missed cached snapshots." />
        <AttemptRow n="Attempt 2" cls="partial" label="Partial" date="May 11, 2025" summary="Invalidation worked; reconnect race remained." />
        <AttemptRow n="Attempt 3" cls="promising" label="Promising" date="May 12, 2025" summary="Stricter freshness checks; stale restores −94% locally." sel best />
      </div>
      <Sec>Current best result</Sec>
      <P>Added stricter context freshness checks on reconnect and invalidated cached session snapshots when the network gap exceeds 30s. Reduced stale restores by 94% in local tests.</P>
      <Sec>Related lessons</Sec>
      <div>
        <LessonLink label="Flaky Reconnects: Context Invalidation Patterns" />
        <LessonLink label="Session State Consistency Principles" />
      </div>
    </div>
  );
}

function LoopBugfix() {
  const { statuses } = useApp();
  const s = statuses["bug-142"];
  const state142 = s === "Done" ? "done" : s === "In progress" ? "prog" : "todo";
  return (
    <div className="content">
      <h1 className="title" {...ed}>Codex Bug Fix Loop</h1>
      <p className="subtitle" {...ed}>Continuously picks the next active bug from Bug Backlog and delegates it to Codex.</p>
      <P>Results, attempts and lessons are written back to each bug document and to Bug Backlog. The queue mirrors the Active Bugs list.</P>
      <Sec>Queue</Sec>
      <div>
        <TaskRow num="1" state={state142} title="Recovery sometimes restores stale context after reconnect" id="#BUG-142" pill={taskPill[s]} to="bug-142" />
        <TaskRow num="2" state="todo" title="Deployment pipeline fails on large monorepos" id="#BUG-139" pill={{ cls: "queued", label: "Queued" }} />
        <TaskRow num="3" state="todo" title="Agent logs missing for cancelled runs" id="#BUG-137" pill={{ cls: "queued", label: "Queued" }} />
        <TaskRow num="4" state="todo" title="Rollback leaves orphaned resources in rare cases" id="#BUG-131" pill={{ cls: "queued", label: "Queued" }} />
        <TaskRow num="5" state="todo" title="UI flicker when switching environments quickly" id="#BUG-129" pill={{ cls: "queued", label: "Queued" }} />
      </div>
      <Sec>Behavior</Sec>
      <ul>
        <Li>Sequential over the Active Bugs list in Bug Backlog.</Li>
        <Li>On failure: record a lesson, keep the result, continue to the next bug.</Li>
        <Li>Write attempts and status back to each bug document.</Li>
      </ul>
    </div>
  );
}

function StubPage({ id }: { id: string }) {
  const s = STUBS[id];
  return (
    <div className="content">
      <h1 className="title" {...ed}>{s.title}</h1>
      <p className="stubnote" {...ed}>{s.desc}</p>
      {s.links && (
        <div style={{ marginTop: 8 }}>
          {s.links.map((l) => <DocLink key={l.id} icon={l.icon} label={l.label} to={l.id} />)}
        </div>
      )}
    </div>
  );
}

export const PAGES: Record<string, () => ReactNode> = {
  "aurora-ide": AuroraIde,
  "session-recovery": SessionRecovery,
  "task-43": Task43,
  "impl-plan": ImplPlan,
  "loop-mvp": LoopMvp,
  "bug-backlog": BugBacklog,
  "bug-142": Bug142,
  "loop-bugfix": LoopBugfix,
};

export function Page({ doc }: { doc: string }) {
  const Hero = PAGES[doc];
  if (Hero) return <>{Hero()}</>;
  return <StubPage id={doc} />;
}
