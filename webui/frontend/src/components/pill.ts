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
  if (s.includes("crash") || s.includes("error")) return { text: "crashed", cls: "crash" };
  // "stranded" covers both a crashed host AND a fresh fork that was never
  // hosted; both recover by sending a message. Keep it calm and accurate
  // rather than alarming ("host lost").
  if (s.includes("strand")) return { text: "stranded · send to resume", cls: "stranded" };
  if (s.includes("interrupt")) return { text: "interrupted", cls: "stranded" };
  if (s.includes("approval")) return { text: "needs approval", cls: "appr" };
  if (s.includes("run") || s.includes("busy")) return { text: "running…", cls: "run" };
  if (s.includes("clos")) return { text: "closed", cls: "closed" };
  if (s.includes("complete") || s.includes("done") || s.includes("end"))
    return { text: "completed", cls: "closed" };
  if (s.includes("idle") || s.includes("ready") || s.includes("wait"))
    return { text: "waiting: input", cls: "idle" };
  return { text: raw || "—", cls: pillClass(raw || "") };
}
