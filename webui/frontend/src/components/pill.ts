// pillClass maps a free-text session/run status onto a coloured pill class.
export function pillClass(status: string): string {
  const s = status.toLowerCase();
  if (s.includes("crash") || s.includes("error") || s.includes("fail")) return "crash";
  if (s.includes("strand") || s.includes("interrupt")) return "stranded";
  if (s.includes("approval")) return "appr";
  if (s.includes("run") || s.includes("busy") || s.includes("wait")) return "run";
  if (s.includes("clos") || s.includes("done") || s.includes("end") || s.includes("complete"))
    return "closed";
  if (s.includes("idle") || s.includes("ready")) return "idle";
  return "";
}

// friendlyStatus maps a raw daemon/session status word to one consistent,
// user-facing label + color class, used by both the sidebar and the task
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
  if (s.includes("budget") || s.includes("max_tokens") || s.includes("token limit"))
    return { text: "Budget limit reached", cls: "stranded" };
  if (s.includes("kill")) return { text: "Stopped by parent", cls: "closed" };
  if (s.includes("cancel")) return { text: "Cancelled", cls: "closed" };
  if (s.includes("crash") || s.includes("error") || s.includes("fail")) return { text: "Failed", cls: "crash" };
  // "stranded" covers both a crashed host AND a fresh fork that was never
  // hosted; both recover by sending a message. Keep it calm and accurate
  // rather than alarming ("host lost").
  if (s.includes("strand") || s.includes("interrupt")) return { text: "Needs recovery", cls: "stranded" };
  if (s.includes("approval")) return { text: "Needs approval", cls: "appr" };
  if (s.includes("run") || s.includes("busy")) return { text: "Running", cls: "run" };
  if (s.includes("clos")) return { text: "Closed", cls: "closed" };
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
  if (s.includes("limit_exceeded") || s.includes("budget") || s.includes("max_tokens") || s.includes("token limit")) {
    return {
      title: "Budget limit reached",
      body: driver
        ? "This scheduled run stopped at its configured token budget. Review the run before changing its limits."
        : "This task stopped at its configured token budget. Continue from a checkpoint in a new task with a larger budget.",
      tone: "attention",
      action: driver ? "inspect" : "continue",
      actionLabel: driver ? "Run details" : "Continue in new task",
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
      body: "The task stopped at its configured generation-step limit. Review the run or continue from a checkpoint.",
      tone: "attention",
      action: driver ? "inspect" : "continue",
      actionLabel: driver ? "Run details" : "Continue in new task",
    };
  }
  if (s.includes("strand") || s.includes("interrupt")) {
    return {
      title: "Task needs recovery",
      body: "The previous host stopped before this task reached a durable terminal state. Resume from its last checkpoint.",
      tone: "attention",
      action: "resume",
      actionLabel: "Resume task",
    };
  }
  if (s.includes("crash") || s.includes("error") || s.includes("fail")) {
    return {
      title: "Task failed",
      body: "The last run ended unexpectedly. Review the recorded run details before deciding whether to retry.",
      tone: "danger",
      action: "inspect",
      actionLabel: "Run details",
    };
  }
  return null;
}
