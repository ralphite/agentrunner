// pillClass maps a free-text session/run status onto a coloured pill class.
export function pillClass(status: string): string {
  const s = status.toLowerCase();
  if (s.includes("crash") || s.includes("error") || s.includes("fail")) return "crash";
  if (s.includes("approval")) return "appr";
  if (s.includes("run") || s.includes("busy") || s.includes("wait")) return "run";
  if (s.includes("clos") || s.includes("done") || s.includes("end") || s.includes("complete"))
    return "closed";
  if (s.includes("idle") || s.includes("ready")) return "idle";
  return "";
}
