import { useCallback, useEffect, useState } from "react";
import {
  CaretDown,
  CaretRight,
  CheckCircle,
  Copy,
  FileText,
  GitBranch,
  GitCommit,
  Hourglass,
  Info,
  PlusMinus,
  TreeStructure,
  WarningCircle,
  X,
} from "@phosphor-icons/react";
import { AR } from "../api";
import { useStore } from "../store";
import { copyText } from "../clipboard";
import { loadGitPrefs } from "../theme";
import { splitDiff } from "../diffSummary";
import { Popover, PopItem, PopSection } from "./Popover";
import { deriveGoalState, formatElapsed, isGoalTerminal, type GoalDerived } from "../timeline";
import type { Task } from "../types";
import { friendlyStatus } from "./pill";
import { dedupeInspectNodes } from "../viewModels";
import { Subagents, type InspectNode } from "./Subagents";

// backgroundLabel turns a raw `ar ps` row ("spawn_agent" +
// "running agent=worker task=…") into a person-readable line (W7). The
// detail is a key=value string; a missing/empty task must not render a
// dangling "task=".
export function backgroundLabel(task: Task): string {
  const detail = task.detail || "";
  const agent = /agent=([^\s]+)/.exec(detail)?.[1];
  const taskText = /task=(.*)$/.exec(detail)?.[1]?.trim();
  if (task.tool === "spawn_agent") {
    const who = agent ? `agent “${agent}”` : "a sub-agent";
    return taskText ? `${who} — ${taskText}` : `${who} is working in the background`;
  }
  return `${task.tool}${detail ? " · " + detail : ""}`;
}

export interface GoalState {
  goal: string;
  checks: number;
  max_checks?: number;
  paused?: boolean;
  verifiers?: number;
  claimed?: boolean;
}

// ProgressItem mirrors inspect's `progress` rows (INC-37): the checklist the
// model maintains via progress_update, statuses already normalized.
export interface ProgressItem {
  id: string;
  title: string;
  status: "pending" | "running" | "done" | "failed";
}

// Panel labels for a settled goal's terminal phase — kept short for the GOAL
// section's meta pill (the composer's GoalBanner carries the longer "Goal
// complete/stopped/cancelled" strings).
const GOAL_PANEL_LABEL: Record<string, string> = {
  achieved: "Completed",
  stopped: "Stopped",
  cancelled: "Cancelled",
};

// useSettledGoal recovers a *finished* goal for the GOAL section (R1-4). The
// live `goal` prop comes from inspect, which drops a goal the moment it settles
// — so an achieved goal would collapse the panel to "No active goal" while the
// composer still shows "✓ Goal complete". When there's no active goal (and the
// panel isn't still loading), we fold the durable goal_* journal events the same
// way GoalBanner does (deriveGoalState) and surface the terminal outcome. Reads
// the session from the store and fetches its own events — mirroring how
// EnvironmentSection sources the current session's diff — so SessionView's props
// stay untouched. One fetch, triggered when the active goal clears.
function useSettledGoal(active: boolean, loading: boolean): GoalDerived | null {
  const sid = useStore((s) => s.currentSid);
  const [settled, setSettled] = useState<GoalDerived | null>(null);
  useEffect(() => {
    if (!sid || active || loading) {
      setSettled(null);
      return;
    }
    let alive = true;
    AR.rawEvents(sid)
      .then((evs) => {
        if (!alive) return;
        const g = deriveGoalState(evs);
        setSettled(g && isGoalTerminal(g.phase) ? g : null);
      })
      .catch(() => {
        if (alive) setSettled(null);
      });
    return () => {
      alive = false;
    };
  }, [sid, active, loading]);
  return settled;
}

// attentionRows folds everything that deserves a human look (W35) into a row
// list: approvals, an agent that stopped abnormally, and background work still
// burning tokens while the conversation itself has gone idle (the
// abandoned-reviewer case: 195k tokens spent after "done"). Lifted out of the
// JSX (TH-3) so the panel can *know* whether Attention has anything to say
// before deciding whether to render the section at all.
function attentionRows(
  children: InspectNode[],
  tasks: Task[],
  approvals: number,
  recovery: boolean,
  sessionIdle: boolean,
): React.ReactNode[] {
  const rows: React.ReactNode[] = [];
  if (approvals > 0) {
    rows.push(
      <div className="attention-row" key="appr">
        <span className="attention-dot" /> Approval requested <b>{approvals}</b>
      </div>,
    );
  }
  if (recovery) {
    rows.push(
      <div className="attention-row" key="recovery">
        <span className="attention-dot" /> Task needs recovery
      </div>,
    );
  }
  for (const node of dedupeInspectNodes(children)) {
    const st = friendlyStatus(node.reason || node.report?.reason || node.report?.status || "");
    if (st.cls === "crash" || st.cls === "stranded") {
      rows.push(
        <div className="attention-row" key={"agent-" + (node.call_id || node.session)}>
          <span className="attention-dot" /> {node.agent || "agent"} — {st.text}
        </div>,
      );
    }
  }
  if (tasks.length > 0 && sessionIdle) {
    rows.push(
      <div className="attention-row" key="bg-idle">
        <span className="attention-dot" /> Background work still running — it keeps
        spending tokens; stop it below if it's no longer needed
      </div>,
    );
  }
  return rows;
}

export function SupervisionPanel({
  loading,
  goal,
  goalEdit,
  progress,
  artifacts,
  children,
  tasks,
  approvals,
  sessionIdle,
  recovery,
  goalEchoed = false,
  onOpenChanges,
  onGoalEdit,
  onGoalSave,
  onGoalDiscard,
  onGoalAction,
  onOpenArtifact,
  onOpenChild,
  onKillTask,
  onInspect,
  onClose,
}: {
  loading: boolean;
  goal: GoalState | null;
  goalEdit: string | null;
  progress: ProgressItem[];
  artifacts: { stream: string; version: number }[];
  children: InspectNode[];
  tasks: Task[];
  approvals: number;
  // The conversation itself is idle (not mid-turn): background work running
  // in that state is worth the user's attention (W35).
  sessionIdle: boolean;
  recovery: boolean;
  // TH-14 · the chrome above the composer is already showing this settled goal's
  // outcome. The GOAL section then states the fact once, on one line, instead of
  // repeating the elapsed + check count the banner just gave.
  goalEchoed?: boolean;
  // TH-15 · open the Changes view. Owned by SessionView (the `view` state lives
  // there); the rail's Changes row is now the primary door to the diff.
  onOpenChanges?: () => void;
  onGoalEdit: (value: string) => void;
  onGoalSave: () => void;
  onGoalDiscard: () => void;
  onGoalAction: (action: "pause" | "resume" | "cancel") => void;
  onOpenArtifact: (stream: string, version: number) => void;
  onOpenChild: (sid: string) => void;
  onKillTask: (handle: string) => void;
  onInspect: () => void;
  onClose: () => void;
}) {
  // When no goal is active, recover the last settled goal so the GOAL section
  // shows its outcome instead of collapsing to "No active goal" (R1-4).
  const settledGoal = useSettledGoal(!!goal, loading);
  // TH-3 — a section with nothing in it doesn't get to take 100px of the panel.
  // Codex's Environment panel simply omits the groups that have no content; it
  // never spends a titled block telling you a group is empty. A resting session
  // used to burn ~325px on three such blocks (Goal "No active goal" + Agents
  // "No subagents" + Attention "Nothing needs you") — each *taller* than a row
  // carrying real data. So: each of the three renders only when it has
  // something, and when none of them does they collapse into the single dim
  // line below (a resting panel must still read as "fine", not as "broken").
  const attention = attentionRows(children, tasks, approvals, recovery, sessionIdle);
  const hasGoal = !!goal || !!settledGoal;
  const resting = !loading && !hasGoal && children.length === 0 && attention.length === 0;
  return (
    // TH-15 · the rail is named `Environment` — in the topbar pill that opens it,
    // in its first section's label, and here in its accessible name. It used to
    // answer to "Supervision" from the outside and "Environment" from the inside.
    <aside className="supervision-panel session-side" aria-label="Environment">
      {/* INC-41 DF-D4 · the `Supervision` title bar is gone. It was a 40px strip
          whose icon+label were a word-for-word second copy of the topbar pill
          that *opens this very panel* — the pill sat 54px above it, always on
          the same screen. RV-1 already settled this for the other right rail
          (see DiffView's `.changes-panel-head` note): a panel opened by a named
          pill does not re-title itself. So the two rails now agree — each opens
          straight onto its first row, and the ✕ rides on that row's right end,
          exactly where Codex's Environment panel keeps its `+`.
          It's a zero-height sticky slot rather than a child of the Environment
          label because EnvironmentSection renders nothing for a non-repo /
          workspace-less session — a panel you couldn't close would be a worse
          bug than the one we're fixing. Height 0 ⇒ Environment gains the whole
          40px back; sticky ⇒ ✕ stays reachable in a long, scrolled panel. */}
      <div className="supervision-close-slot">
        <button
          className="supervision-close"
          onClick={onClose}
          title="Hide Environment"
          aria-label="Hide Environment"
        >
          <X size={15} />
        </button>
      </div>

      <EnvironmentSection onOpenChanges={onOpenChanges} />

      {/* One indeterminate line while inspect is in flight — not three titled
          "Checking…" blocks that then collapse into nothing (TH-3): the panel
          keeps the same height from load to rest, so it never flashes a hole. */}
      {loading && (
        <div className="supervision-quiet supervision-loading">
          <Hourglass size={14} className="spin" /> Checking…
        </div>
      )}

      {/* TH-14 · a settled goal whose outcome is ALREADY on screen above the
          composer gets one line here, not a titled 123px block that repeats the
          banner's own "Cancelled · 00:34 · 0 checks" word for word. The goal text
          is the thing the rail can add (the banner has no room for it), so that's
          what the line carries, behind the phase chip. */}
      {!loading && !goal && settledGoal && goalEchoed && (
        <section className="supervision-section">
          <div className="goal-settled-line" title={settledGoal.goal}>
            <span className={"goal-outcome " + settledGoal.phase}>
              {GOAL_PANEL_LABEL[settledGoal.phase] || "Ended"}
            </span>
            <span className="goal-settled-copy">{settledGoal.goal}</span>
          </div>
        </section>
      )}

      {!loading && hasGoal && !(!goal && settledGoal && goalEchoed) && (
      <section className="supervision-section">
        <div className="supervision-label">Goal</div>
        {goal ? (
          <>
            {goalEdit === null ? (
              <div className="goal-copy">{goal.goal}</div>
            ) : (
              <input className="goal-input" autoFocus value={goalEdit} onChange={(event) => onGoalEdit(event.target.value)} onKeyDown={(event) => {
                if (event.key === "Enter") onGoalSave();
                if (event.key === "Escape") onGoalDiscard();
              }} />
            )}
            <div className="goal-meta">
              <span>{goal.checks}{goal.max_checks ? `/${goal.max_checks}` : ""} checks</span>
              {goal.paused && <span>Paused</span>}
              {goal.verifiers === 0 && <span>Self-certified</span>}
            </div>
            <div className="goal-actions">
              {goalEdit === null ? (
                <>
                  <button onClick={() => onGoalEdit(goal.goal)}>Edit</button>
                  <button onClick={() => onGoalAction(goal.paused ? "resume" : "pause")}>{goal.paused ? "Resume" : "Pause"}</button>
                  <button className="danger" onClick={() => onGoalAction("cancel")}>Cancel</button>
                </>
              ) : (
                <>
                  <button className="primary" onClick={onGoalSave}>Save</button>
                  <button onClick={onGoalDiscard}>Discard</button>
                </>
              )}
            </div>
          </>
        ) : settledGoal ? (
          // No active goal, but the journal carries a finished one — show its
          // outcome (Completed · elapsed · N checks) so the panel agrees with
          // the composer's goal banner instead of going silent.
          <>
            <div className="goal-copy">{settledGoal.goal}</div>
            <div className="goal-meta goal-meta-settled">
              <span className={"goal-outcome " + settledGoal.phase}>
                {GOAL_PANEL_LABEL[settledGoal.phase] || "Ended"}
              </span>
              {settledGoal.elapsedMs !== undefined && <span>{formatElapsed(settledGoal.elapsedMs)}</span>}
              <span>{settledGoal.checks} check{settledGoal.checks === 1 ? "" : "s"}</span>
            </div>
          </>
        ) : null}
      </section>
      )}

      {progress.length > 0 && (
        <section className="supervision-section">
          {/* The model-maintained checklist (INC-37). Rendered only when the
              model actually keeps one — an empty permanent section would be
              exactly the W5 dead-weight this panel was purged of. */}
          <div className="supervision-label">
            Progress
            <span className="progress-count">
              {progress.filter((it) => it.status === "done").length}/{progress.length}
            </span>
          </div>
          <div className="progress-list">
            {progress.map((it) => (
              <div className={"progress-row " + it.status} key={it.id} title={it.title}>
                {it.status === "running" ? (
                  <Hourglass size={13} className="spin" />
                ) : it.status === "done" ? (
                  <CheckCircle size={13} weight="fill" />
                ) : it.status === "failed" ? (
                  <WarningCircle size={13} weight="fill" />
                ) : (
                  <CaretRight size={13} />
                )}
                <span className="progress-title">{it.title}</span>
              </div>
            ))}
          </div>
        </section>
      )}

      {artifacts.length > 0 && (
        <section className="supervision-section">
          {/* Published artifacts (INC-40): latest version per stream, click
              to read. Rendered only when something was actually published
              (the W5 no-dead-weight rule). */}
          <div className="supervision-label">Artifacts</div>
          <div className="artifact-list">
            {artifacts.map((a) => (
              <button
                type="button"
                className="artifact-row"
                key={a.stream}
                title={`Read ${a.stream} (latest v${a.version})`}
                onClick={() => onOpenArtifact(a.stream, a.version)}
              >
                <FileText size={13} />
                <span className="artifact-stream">{a.stream}</span>
                <span className="artifact-version">v{a.version}</span>
              </button>
            ))}
          </div>
        </section>
      )}

      {!loading && children.length > 0 && (
        <section className="supervision-section supervision-agents">
          <div className="supervision-label">Agents</div>
          <Subagents nodes={children} onOpen={onOpenChild} />
        </section>
      )}

      {!loading && attention.length > 0 && (
        <section className="supervision-section">
          <div className="supervision-label">Attention</div>
          {attention}
        </section>
      )}

      {/* The resting panel: one dim line instead of three empty blocks (TH-3). */}
      {resting && (
        <div className="supervision-quiet">
          <CheckCircle size={15} /> Nothing needs you
        </div>
      )}

      {tasks.length > 0 && (
        <section className="supervision-section">
          <div className="supervision-label">Background work</div>
          {tasks.map((task) => (
            <div className="background-row" key={task.handle}>
              <span className="status-dot run" />
              <span title={task.detail || task.handle}>{backgroundLabel(task)}</span>
              <button title="Stop this background work (ar kill)" onClick={() => onKillTask(task.handle)}><X size={13} /></button>
            </div>
          ))}
        </section>
      )}

      {/* INC-41 ENV-4/ENV-5 · the panel's footer row. It used to be the *heaviest*
          text on the panel (weight 550, --ink-2) and the only line with no leading
          icon — so the loudest thing in a rail full of live git state was a link to
          a modal, and its text started 30px left of every row's label. Codex's
          same-position element (`View all`) is the *quietest* row on its panel and
          rides the same icon+label grid as everything above it. So does ours now:
          a 14px icon on the env-row icon column (15px section pad + 6px row pad),
          the label on the env-row label column (+14px icon +9px gap), --dim at
          weight 400. styles.panel.css then drops it to the panel's floor with
          `margin-top:auto` + a hairline top border (ENV-5) — the rail's ~510px of
          empty background is now *framed* between content and footer instead of
          reading as a torn-off strip of the thread. */}
      <button className="supervision-details" onClick={onInspect} title="Review this run's status, usage, activity, and provider capabilities">
        <Info size={14} />
        <span className="supervision-details-label">Run details</span>
        <span className="supervision-details-caret"><CaretRight size={12} /></span>
      </button>
    </aside>
  );
}

interface EnvState {
  workspace: string;
  known: boolean;
  isRepo: boolean;
  nested: boolean;
  add: number;
  del: number;
  files: number;
  untracked: number;
}

// workspaceName is the tail segment of a workspace path — for a worktree
// session that's the generated dir (wt-20260710-143427); for an in-place
// session it's the repo's own directory name. Trailing slashes don't count.
export function workspaceName(path: string): string {
  const parts = (path || "").split("/").filter(Boolean);
  return parts.length > 0 ? parts[parts.length - 1] : "";
}

// EnvironmentSection is Codex's panel-top ENVIRONMENT block (backlog B2,
// CX-4): a live read on the session's workspace. Codex keeps the same four
// rows on screen at all times — Changes · Worktree · Create branch · Commit
// or push — so the git entry points are reachable *before* there's anything
// to commit (you can't cut a branch from a panel that only appears once you've
// already dirtied the tree). We follow that: every row is always rendered; the
// ones that can't act right now go disabled with the reason on the right,
// rather than vanishing. SupervisionPanel takes no sid prop (SessionView owns
// that and stays untouched), so we read the current session from the store and
// fetch our own diff + branch. The section as a whole is still hidden for
// non-repo / workspace-less sessions, where git means nothing.
function EnvironmentSection({ onOpenChanges }: { onOpenChanges?: () => void }) {
  const sid = useStore((s) => s.currentSid);
  const openPrompt = useStore((s) => s.openPrompt);
  const toast = useStore((s) => s.toast);
  const [env, setEnv] = useState<EnvState | null>(null);
  const [branch, setBranch] = useState<string | null>(null);
  // One busy flag for every mutating row (commit / push / branch): they all
  // write the same workspace, so none of them should overlap.
  const [busy, setBusy] = useState(false);
  const [wtOpen, setWtOpen] = useState(false);
  const isSub = !!sid && sid.includes("-sub-");

  const load = useCallback(() => {
    if (!sid) {
      setEnv(null);
      setBranch(null);
      return;
    }
    let alive = true;
    AR.diff(sid)
      .then((d) => {
        if (!alive) return;
        const files = splitDiff(d.diff || "");
        setEnv({
          workspace: d.workspace,
          known: d.known,
          isRepo: d.isRepo,
          nested: !!d.nested,
          add: files.reduce((n, f) => n + f.add, 0),
          del: files.reduce((n, f) => n + f.del, 0),
          files: files.length,
          untracked: (d.untracked || []).length,
        });
        if (d.known && d.isRepo && !d.nested && d.workspace) {
          AR.gitBranches(d.workspace)
            .then((b) => alive && setBranch(b.isRepo ? (b.current === "HEAD" ? "" : b.current) : null))
            .catch(() => alive && setBranch(null));
        } else {
          setBranch(null);
        }
      })
      .catch(() => {
        if (!alive) return;
        setEnv(null);
        setBranch(null);
      });
    return () => {
      alive = false;
    };
  }, [sid]);

  useEffect(() => load(), [load]);

  // TH-15 · Jump to the Changes view. This used to synthesise a click on the
  // topbar's `Changes` pill — a DOM-reaching hack that survived only because
  // that pill existed. It doesn't any more (this row IS the door to the diff),
  // so the view toggle arrives honestly, as a callback from SessionView.
  const goToChanges = () => onOpenChanges?.();

  // Same review→commit(→push) flow DiffView offers (seeded from the Settings
  // template). `thenPush` chains a push only after a successful commit.
  const doCommit = async (message: string, thenPush = false) => {
    if (!sid) return;
    setBusy(true);
    try {
      await AR.commit(sid, message);
      if (thenPush) {
        const r = await AR.push(sid);
        toast(r.branch ? `committed & pushed ${r.branch}` : "committed & pushed", "info");
      } else {
        toast("committed", "info");
      }
      load();
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };
  const commit = (thenPush = false) => {
    if (!sid) return;
    openPrompt({
      title: thenPush ? "Commit & push" : "Commit changes",
      label: "commit message",
      initial: loadGitPrefs().commitTemplate,
      submitLabel: thenPush ? "Commit & push" : "Commit",
      onSubmit: (message) => void doCommit(message, thenPush),
    });
  };
  const doPush = async () => {
    if (!sid) return;
    setBusy(true);
    try {
      const r = await AR.push(sid);
      toast(r.branch ? `pushed ${r.branch}` : "pushed", "info");
      load();
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };

  // Create branch (CX-4): cut a branch straight from the session, at any time —
  // no changes required. Reuses the app's prompt modal (the window.prompt
  // replacement in the store) and the existing checkout endpoint with
  // create=true, which is `git checkout -b` on the session's workspace.
  const doCreateBranch = async (dir: string, name: string) => {
    setBusy(true);
    try {
      const r = await AR.gitCheckout(dir, name, true);
      toast(`switched to ${r.branch || name}`, "info");
      load();
    } catch (e: any) {
      toast(e.message);
    } finally {
      setBusy(false);
    }
  };
  const createBranch = (dir: string) => {
    openPrompt({
      title: "Create branch",
      label: "branch name",
      placeholder: "my-feature",
      submitLabel: "Create",
      onSubmit: (name) => {
        const clean = name.trim();
        if (clean) void doCreateBranch(dir, clean);
      },
    });
  };

  if (!env || !env.known || !env.isRepo || env.nested) return null;

  const hasChanges = env.add > 0 || env.del > 0 || env.untracked > 0;
  const workspace = env.workspace || "";
  // A sub-agent session shares its parent's workspace and must not commit on
  // its own — so the commit row stays visible (Codex never hides it) but says
  // why it can't act, exactly like the nothing-to-commit case.
  const canCommit = hasChanges && !isSub;
  const commitBlockedWhy = isSub ? "Sub-agent" : "Nothing to commit";
  return (
    <section className="supervision-section supervision-env">
      <div className="supervision-label">Environment</div>
      <div className="env-rows">
        {/* TH-6a — the Changes row is a *diff* (± lines), not a branch: GitDiff's
            forking arrows read as branch/merge. Codex uses the ± glyph; Phosphor's
            PlusMinus is that glyph (lucide's Diff would mean a new dependency for
            one icon in an all-Phosphor codebase). Size/spacing unchanged. */}
        {/* INC-41 ENV-3 · a clean tree renders *no* value. The right edge of this
            panel used to be a column of negations — "No changes" / "No branch yet"
            / "Nothing to commit" — set in the same 12px --dim as the real numbers,
            so the loudest reading of the Environment block was three sentences
            about things that don't exist. Codex's rows simply carry nothing on the
            right when there's nothing to say. The reason still lives in the row's
            title/disabled state; only the noise is gone. A row with real state
            (+12 / −3 / "4 new") is untouched. */}
        <button className="env-row" onClick={goToChanges} title="Review workspace changes">
          <PlusMinus size={14} />
          <span className="env-row-label">Changes</span>
          {hasChanges && (
            <span className="env-row-val">
              {env.add > 0 && <span className="add">+{env.add}</span>}
              {env.del > 0 && <span className="del">−{env.del}</span>}
              {env.add === 0 && env.del === 0 && env.untracked > 0 && (
                <span className="dim">{env.untracked} new</span>
              )}
            </span>
          )}
        </button>
        {/* Worktree — always listed, so the user can always see (and copy)
            where this session is actually working. Expands to the full path;
            a session with no workspace shows an em dash and can't expand. */}
        <button
          className={"env-row env-row-action" + (wtOpen ? " active" : "")}
          onClick={() => setWtOpen((open) => !open)}
          disabled={!workspace}
          aria-expanded={wtOpen}
          title={workspace || "This session has no workspace"}
        >
          <TreeStructure size={14} />
          <span className="env-row-label">Worktree</span>
          <span className="env-row-val">
            <span className="dim env-row-name">{workspace ? workspaceName(workspace) : "—"}</span>
          </span>
          {workspace && (wtOpen ? <CaretDown size={12} /> : <CaretRight size={12} />)}
        </button>
        {wtOpen && workspace && (
          <div className="env-detail">
            <code className="env-path" title={workspace}>{workspace}</code>
            <button
              type="button"
              className="env-path-copy"
              onClick={() => {
                void copyText(workspace);
                toast("workspace path copied", "info");
              }}
              title="Copy the full workspace path"
            >
              <Copy size={12} /> Copy path
            </button>
          </div>
        )}
        {/* Create branch — a permanent entry point (Codex parity CX-4): you
            can cut a branch before touching a single file. The current branch
            rides on the right, replacing the old static branch-only row. */}
        <button
          className="env-row env-row-action"
          onClick={() => workspace && createBranch(workspace)}
          disabled={busy || !workspace}
          title={workspace ? "Create a new branch in this workspace" : "This session has no workspace"}
        >
          <GitBranch size={14} />
          <span className="env-row-label">Create branch</span>
          {/* ENV-3 · the current branch rides on the right only when there *is*
              one; a detached / branchless workspace says nothing rather than
              shouting "No branch yet" at the same weight as the action. */}
          {branch && (
            <span className="env-row-val">
              <span className="dim env-row-name">{branch}</span>
            </span>
          )}
        </button>
        {canCommit ? (
          <div className="w-full [&>.pop-wrap]:w-full">
          <Popover
            align="left"
            panelClass="w-[264px] max-w-[calc(100vw-24px)]"
            trigger={(open, toggle) => (
              <button
                className={"env-row env-row-action" + (open ? " active" : "")}
                onClick={toggle}
                disabled={busy}
                aria-haspopup="menu"
                aria-expanded={open}
                title="Commit or push the workspace changes"
              >
                <GitCommit size={14} />
                <span className="env-row-label">Commit or push</span>
                <CaretDown size={12} />
              </button>
            )}
          >
            {(close) => (
              <PopSection label="Commit or push">
                <PopItem
                  title="Commit"
                  desc="Commit locally (no push)"
                  onClick={() => {
                    close();
                    commit(false);
                  }}
                />
                <PopItem
                  title="Commit &amp; push"
                  desc="Commit, then push to the upstream branch"
                  onClick={() => {
                    close();
                    commit(true);
                  }}
                />
                <PopItem
                  title="Push"
                  desc="Push existing commits to the upstream branch"
                  onClick={() => {
                    close();
                    void doPush();
                  }}
                />
              </PopSection>
            )}
          </Popover>
          </div>
        ) : (
          // Nothing to commit (or a sub-agent session): the row stays — Codex
          // never hides it — but goes inert. ENV-3: it no longer *also* prints
          // "Nothing to commit" on the right; that was the row's own disabled
          // state said twice, in a colour as loud as the action itself. The
          // reason survives in the title, one hover away.
          <button className="env-row env-row-action" disabled title={commitBlockedWhy}>
            <GitCommit size={14} />
            <span className="env-row-label">Commit or push</span>
          </button>
        )}
      </div>
    </section>
  );
}
