export interface ApprovalPresentation {
  title: string;
  subject: string;
  description: string;
  scope: string;
}

function objectArgs(raw: unknown): Record<string, unknown> {
  if (raw && typeof raw === "object" && !Array.isArray(raw)) return raw as Record<string, unknown>;
  if (typeof raw !== "string") return {};
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return { value: raw };
  }
}

function firstString(args: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = args[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

export function describeApproval(tool: string, rawArgs: unknown): ApprovalPresentation {
  const name = (tool || "action").toLowerCase();
  const args = objectArgs(rawArgs);
  if (name === "bash" || name === "shell" || name === "command") {
    return {
      title: "Run command",
      subject: firstString(args, ["command", "cmd", "value"]) || tool,
      description: "The agent wants to run this command in the current workspace.",
      scope: "Current workspace",
    };
  }
  if (name.includes("write") || name.includes("edit") || name.includes("patch")) {
    return {
      title: name.includes("write") ? "Write file" : "Edit file",
      subject: firstString(args, ["path", "file", "filename"]) || tool,
      description: "The agent wants to change a file in the current workspace.",
      scope: "Current workspace",
    };
  }
  if (name.includes("fetch") || name.includes("http") || name.includes("network")) {
    return {
      title: "Open network resource",
      subject: firstString(args, ["url", "uri", "host"]) || tool,
      description: "The agent wants to access an external network resource.",
      scope: "Network access",
    };
  }
  if (name.includes("spawn")) {
    return {
      title: "Start agent",
      subject: firstString(args, ["agent", "name", "task"]) || "Subagent",
      description: "The agent wants to start another agent for this task.",
      scope: "Current session",
    };
  }
  return {
    title: "Allow action",
    subject: tool || "Requested action",
    description: "Review this request before the agent continues.",
    scope: "Current session",
  };
}
