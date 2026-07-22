// pillClass maps a free-text session/run status onto a coloured pill class.
export function pillClass(status: string): string {
  const s = status.toLowerCase();
  if (s.includes("crash") || s.includes("error") || s.includes("fail")) return "crash";
  if (s.trim() === "interrupted") return "closed";
  if (s.includes("strand") || s.includes("interrupt")) return "stranded";
  if (s.includes("approval")) return "appr";
  if (s.includes("run") || s.includes("busy") || s.includes("wait")) return "run";
  if (s.includes("clos") || s.includes("done") || s.includes("end") || s.includes("complete"))
    return "closed";
  if (s.includes("idle") || s.includes("ready")) return "idle";
  return "";
}

// friendlyStatus maps a raw daemon/session status word to one consistent,
// user-facing label + color class, used by both the sidebar and the session
// header so the two never contradict each other (QA #8).
export function friendlyStatus(raw: string): { text: string; cls: string } {
  const s = (raw || "").toLowerCase();
  // Terminal reasons from `ar inspect` land here raw (W6) — say what they
  // mean instead of leaking the enum. Checked before the broad keyword
  // buckets so e.g. max_generation_steps never matches "run".
  if (s.includes("max_generation_steps") || s.includes("step limit"))
    return { text: "Step limit reached", cls: "stranded" };
  if (s.includes("max_iterations"))
    return { text: "Iteration limit reached", cls: "stranded" };
  // Goal-scoped endings come through Quiescence verbatim ("goal_budget_
  // exhausted" persists across later turns by design, QA Wave3 ivan-08).
  // They are about the GOAL, not the session: the conversation is still
  // open for input. Matching the bare "budget" bucket below branded a
  // perfectly continuable session "Budget limit reached" (QA v2sim).
  if (s.includes("goal_budget_exhausted"))
    return { text: "Goal stopped — check budget", cls: "stranded" };
  if (s.includes("goal_satisfied")) return { text: "Goal completed", cls: "closed" };
  if (s.includes("budget") || s.includes("max_tokens") || s.includes("token limit"))
    return { text: "Budget limit reached", cls: "stranded" };
  if (s.includes("kill")) return { text: "Stopped by parent", cls: "closed" };
  if (s.includes("cancel")) return { text: "Cancelled", cls: "closed" };
  if (s.includes("crash") || s.includes("error") || s.includes("fail")) return { text: "Failed", cls: "crash" };
  // A durable, user-requested Stop is complete and immediately continuable.
  // Composite crash reasons that merely contain "interrupt" have already hit
  // the crash branch above and must not be softened.
  if (s.trim() === "interrupted") return { text: "Stopped", cls: "closed" };
  // "stranded" covers both a crashed host AND a fresh fork that was never
  // hosted; both recover by sending a message. Keep it calm and accurate
  // rather than alarming ("host lost").
  if (s.includes("strand") || s.includes("interrupt")) return { text: "Needs recovery", cls: "stranded" };
  if (s.includes("approval")) return { text: "Needs approval", cls: "appr" };
  if (s.includes("run") || s.includes("busy")) return { text: "Running", cls: "run" };
  // "idle" (a legacy lifecycle mark folded neutral, INC-83) falls through to
  // the Ready bucket below — an idle conversation is just ready for input.
  if (s.includes("clos")) return { text: "Idle", cls: "closed" }; // legacy journals
  if (s.includes("satisfied")) return { text: "Completed", cls: "closed" };
  if (s.includes("limit_exceeded")) return { text: "Budget limit reached", cls: "stranded" };
  if (s.includes("complete") || s.includes("done") || s.includes("end"))
    return { text: "Completed", cls: "closed" };
  if (s.includes("idle") || s.includes("ready") || s.includes("wait"))
    return { text: "Ready", cls: "idle" };
  return { text: raw || "Unknown", cls: pillClass(raw || "") };
}

export interface TerminalNotice {
  title: string;
  body: string;
  tone: "attention" | "danger";
  action: "continue" | "resume" | "inspect";
  actionLabel: string;
}

// terminalNoticeFor turns an abnormal durable session status into an honest
// next-step banner. It intentionally does not invent provider reset times,
// purchasable credits, or a retry that the runtime cannot actually perform.
export function terminalNoticeFor(raw: string, driver = false): TerminalNotice | null {
  const s = (raw || "").toLowerCase();
  // A goal ending is not a session ending. Quiescence keeps "goal_budget_
  // exhausted" (and "goal_satisfied") as the durable status so the signal
  // survives later turns, but the session itself stays open — the GoalBanner
  // owns that story (label + checks + dismiss). Substring-matching "budget"
  // below used to pin a false "Budget limit reached · Continue in new
  // session" terminal card over a waiting session, and the fork it advertised
  // inherited the exhausted goal, reproducing the same dead end (QA v2sim).
  if (s.includes("goal_")) return null;
  if (s.includes("limit_exceeded") || s.includes("budget") || s.includes("max_tokens") || s.includes("token limit")) {
    return {
      title: "Budget limit reached",
      body: driver
        ? "This scheduled run stopped at its configured token budget. Review the run before changing its limits."
        : "This session stopped at its configured token budget. Continue from a checkpoint in a new session with a larger budget.",
      tone: "attention",
      action: driver ? "inspect" : "continue",
      actionLabel: driver ? "Run details" : "Continue in new session",
    };
  }
  if (s.includes("max_iterations")) {
    return {
      title: "Iteration limit reached",
      body: "The scheduled run completed its configured number of iterations. Review the run before extending it.",
      tone: "attention",
      action: "inspect",
      actionLabel: "Run details",
    };
  }
  if (s.includes("max_generation_steps") || s.includes("step limit")) {
    return {
      title: "Step limit reached",
      body: "The session stopped at its configured generation-step limit. Review the run or continue from a checkpoint.",
      tone: "attention",
      action: driver ? "inspect" : "continue",
      actionLabel: driver ? "Run details" : "Continue in new session",
    };
  }
  // A deliberate Stop already has an in-thread terminal chip and a Retry
  // action. Resume is only for a host/session that actually lost liveness.
  if (s.trim() === "interrupted") return null;
  if (s.includes("strand") || s.includes("interrupt")) {
    return {
      title: "Session needs recovery",
      body: "The previous host stopped before this session reached a durable terminal state. Resume from its last checkpoint.",
      tone: "attention",
      action: "resume",
      actionLabel: "Resume session",
    };
  }
  if (s.includes("crash") || s.includes("error") || s.includes("fail")) {
    return {
      title: "Session failed",
      body: "The last run ended unexpectedly. Review the recorded run details before deciding whether to retry.",
      tone: "danger",
      action: "inspect",
      actionLabel: "Run details",
    };
  }
  return null;
}
