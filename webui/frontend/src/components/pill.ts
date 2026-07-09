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
  if (s.includes("crash") || s.includes("error")) return { text: "已中断", cls: "crash" };
  if (s.includes("strand")) return { text: "游离·宿主丢失", cls: "stranded" };
  if (s.includes("interrupt")) return { text: "已停止", cls: "stranded" };
  if (s.includes("approval")) return { text: "需要你批准", cls: "appr" };
  if (s.includes("run") || s.includes("busy")) return { text: "运行中…", cls: "run" };
  if (s.includes("complete") || s.includes("closed") || s.includes("done") || s.includes("end"))
    return { text: "已完成", cls: "closed" };
  if (s.includes("idle") || s.includes("ready") || s.includes("wait"))
    return { text: "等待你", cls: "idle" };
  return { text: raw || "—", cls: pillClass(raw || "") };
}
