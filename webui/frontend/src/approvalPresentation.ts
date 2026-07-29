export interface ApprovalPresentation {
  title: string;
  subject: string;
  description: string;
  scope: string;
}

// Approval cards need enough location context to prevent approving work in the
// wrong project, but a full temp/worktree path overwhelms the decision. Keep
// the complete path in the card title/Details and use its final segment in the
// primary UI, matching Codex's compact environment labels.
export function compactWorkspaceName(workspace?: string): string {
  const clean = (workspace || "").trim().replace(/\/+$/, "");
  if (!clean) return "";
  const parts = clean.split("/").filter(Boolean);
  return parts[parts.length - 1] || "/";
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

// Credential shapes, mirroring the runtime's own list (internal/pipeline).
// These reach the card as an extra warning, not as a different decision — the
// gate already decided to ask; the card's job is to say what is being asked.
const CREDENTIAL_PATTERNS = [
  /(^|\/)\.env(\.|$)/,
  /(^|\/)\.npmrc$/,
  /(^|\/)\.netrc$/,
  /(^|\/)id_(rsa|dsa|ecdsa|ed25519)$/,
  /\.pem$/,
  /(^|\/)\.ssh\//,
  /(^|\/)\.aws\/credentials$/,
  /(^|\/)credentials\.json$/,
];

function isCredentialPath(path: string): boolean {
  return CREDENTIAL_PATTERNS.some((re) => re.test(path));
}

// Whether a path the agent asked for sits outside the session's workspace.
// Only an absolute path can be judged here; a relative one is workspace-bound
// by construction, and the runtime resolves traversal before it ever asks.
function isOutsideWorkspace(path: string, workspace?: string): boolean {
  const ws = (workspace || "").trim().replace(/\/+$/, "");
  if (!ws || !path.startsWith("/")) return false;
  return path !== ws && !path.startsWith(ws + "/");
}

export function describeApproval(
  tool: string,
  rawArgs: unknown,
  workspace?: string,
): ApprovalPresentation {
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
  if (name.includes("read") && !name.includes("thread")) {
    const path = firstString(args, ["path", "file", "filename"]) || tool;
    // A read only reaches an approval when it is out of bounds or sensitive;
    // ordinary reads are allowed outright, so say which one this is.
    return {
      title: "Read file",
      subject: path,
      ...fileScope(path, workspace, "read"),
    };
  }
  if (name.includes("write") || name.includes("edit") || name.includes("patch")) {
    const path = firstString(args, ["path", "file", "filename"]) || tool;
    return {
      title: name.includes("write") ? "Write file" : "Edit file",
      subject: path,
      ...fileScope(path, workspace, "change"),
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
      subject: firstString(args, ["agent", "name", "prompt"]) || "Subagent",
      description: "The agent wants to start another agent for this session.",
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

// The card must never claim a file is "in the current workspace" when the whole
// reason it is being asked about is that it is NOT — that sentence used to be
// hardcoded, and it told the user the opposite of the truth on exactly the
// decision where truth matters most (LOG 2026-07-29).
function fileScope(
  path: string,
  workspace: string | undefined,
  verb: "read" | "change",
): { description: string; scope: string } {
  if (isCredentialPath(path)) {
    return {
      description: `This looks like a credential file. The agent wants to ${verb} it.`,
      scope: "Credential file",
    };
  }
  if (isOutsideWorkspace(path, workspace)) {
    return {
      description: `This file is OUTSIDE this session's workspace. The agent wants to ${verb} it.`,
      scope: "Outside the workspace",
    };
  }
  return {
    description: `The agent wants to ${verb} a file in the current workspace.`,
    scope: "Current workspace",
  };
}
