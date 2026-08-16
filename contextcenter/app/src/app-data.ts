// 静态样例数据：树结构、面包屑、stub 页。行为层 gaps 未裁决，本层仅供壳演示。
export type IconName =
  | "page" | "ws" | "plan" | "research" | "backlog" | "layers" | "cube" | "compass" | "note" | "map";

export interface TreeNode {
  id: string;
  label: string;
  icon: IconName;
  proj?: boolean;
  kids?: TreeNode[];
}

export const TREE: TreeNode[] = [
  {
    id: "aurora-ide", label: "Aurora IDE", icon: "layers", proj: true,
    kids: [
      { id: "aurora-overview", label: "Overview", icon: "page" },
      { id: "editor-performance", label: "Editor Performance", icon: "ws" },
      { id: "ai-assistant", label: "AI Assistant", icon: "ws" },
      {
        id: "session-recovery", label: "Session Recovery", icon: "ws",
        kids: [
          { id: "impl-plan", label: "Implementation Plan", icon: "plan" },
          { id: "research-notes", label: "Research Notes", icon: "research" },
          { id: "recovery-ux", label: "Recovery UX", icon: "page" },
        ],
      },
      { id: "aurora-bugs", label: "Bugs", icon: "page" },
      { id: "aurora-notes", label: "Notes", icon: "page" },
      { id: "aurora-plans", label: "Plans", icon: "page" },
    ],
  },
  {
    id: "atlas-deploy", label: "Atlas Deploy", icon: "cube", proj: true,
    kids: [
      { id: "atlas-overview", label: "Overview", icon: "page" },
      { id: "deployment-pipeline", label: "Deployment Pipeline", icon: "ws" },
      { id: "infrastructure", label: "Infrastructure", icon: "ws" },
      { id: "bug-backlog", label: "Bug Backlog", icon: "backlog" },
      { id: "atlas-notes", label: "Notes", icon: "page" },
      { id: "atlas-plans", label: "Plans", icon: "page" },
    ],
  },
];

export interface Crumb { label: string; doc?: string; projIcon?: "layers" | "cube"; }

const AUR: Crumb = { label: "Aurora IDE", doc: "aurora-ide", projIcon: "layers" };
const ATL: Crumb = { label: "Atlas Deploy", doc: "atlas-deploy", projIcon: "cube" };
const SR: Crumb = { label: "Session Recovery", doc: "session-recovery" };
const BB: Crumb = { label: "Bug Backlog", doc: "bug-backlog" };

export const CRUMBS: Record<string, Crumb[]> = {
  "aurora-ide": [AUR],
  "session-recovery": [AUR, { label: "Session Recovery" }],
  "task-43": [AUR, SR, { label: "Implement automatic recovery" }],
  "impl-plan": [AUR, SR, { label: "Implementation Plan" }],
  "loop-mvp": [AUR, SR, { label: "Implementation Plan", doc: "impl-plan" }, { label: "Recovery MVP Loop" }],
  "bug-backlog": [ATL, { label: "Bug Backlog" }],
  "bug-142": [ATL, BB, { label: "Recovery sometimes restores stale context…" }],
  "loop-bugfix": [ATL, BB, { label: "Codex Bug Fix Loop" }],
};

export interface StubDef {
  title: string;
  crumbs: Crumb[];
  desc: string;
  links?: Array<{ id: string; label: string; icon: IconName }>;
  fields: Array<[string, string]>;
}

export const STUBS: Record<string, StubDef> = {
  "aurora-overview": {
    title: "Overview", crumbs: [AUR, { label: "Overview" }],
    desc: "A plain document. Give it a type in frontmatter if you want a preset icon and fields — otherwise it stays exactly this simple.",
    fields: [["Type", "—"], ["Created", "Apr 28, 2025"], ["Updated", "May 3, 2025"]],
  },
  "editor-performance": {
    title: "Editor Performance", crumbs: [AUR, { label: "Editor Performance" }],
    desc: "Workstream document. Capture context, tasks and results for editor performance work here.",
    fields: [["Type", "Workstream"], ["Status", "Active"], ["Tags", "perf"], ["Updated", "May 8, 2025"]],
  },
  "ai-assistant": {
    title: "AI Assistant", crumbs: [AUR, { label: "AI Assistant" }],
    desc: "Workstream document for the built-in AI assistant.",
    fields: [["Type", "Workstream"], ["Status", "Paused"], ["Updated", "May 5, 2025"]],
  },
  "research-notes": {
    title: "Research Notes", crumbs: [AUR, SR, { label: "Research Notes" }],
    desc: "Findings from prior art on crash recovery, checkpoint formats and state hydration.",
    fields: [["Type", "Research"], ["Updated", "May 11, 2025"]],
  },
  "recovery-ux": {
    title: "Recovery UX", crumbs: [AUR, SR, { label: "Recovery UX" }],
    desc: "UX notes for the restore flow: silent restore by default, visible fallback when state is partial.",
    fields: [["Type", "—"], ["Updated", "May 10, 2025"]],
  },
  "aurora-bugs": {
    title: "Bugs", crumbs: [AUR, { label: "Bugs" }],
    desc: "Bugs for Aurora IDE. Lines can stay lightweight markdown, or become bug documents when they need attempts and lessons.",
    fields: [["Type", "—"], ["Updated", "May 12, 2025"]],
  },
  "aurora-notes": {
    title: "Notes", crumbs: [AUR, { label: "Notes" }],
    desc: "Loose notes. Select anything and quote it in chat.",
    fields: [["Type", "—"]],
  },
  "aurora-plans": {
    title: "Plans", crumbs: [AUR, { label: "Plans" }],
    desc: "Planning documents live here as ordinary nested pages.",
    fields: [["Type", "—"]],
  },
  "atlas-deploy": {
    title: "Atlas Deploy", crumbs: [ATL],
    desc: "Project root for the deploy platform. Agents start reading here.",
    links: [
      { id: "atlas-overview", label: "Overview", icon: "page" },
      { id: "deployment-pipeline", label: "Deployment Pipeline", icon: "ws" },
      { id: "infrastructure", label: "Infrastructure", icon: "ws" },
      { id: "bug-backlog", label: "Bug Backlog", icon: "backlog" },
      { id: "atlas-notes", label: "Notes", icon: "page" },
      { id: "atlas-plans", label: "Plans", icon: "page" },
    ],
    fields: [["Type", "Project"], ["Workspace", "~/dev/atlas-deploy"], ["Agents", "Codex"], ["Default agent", "Codex"], ["Updated", "May 12, 2025"]],
  },
  "atlas-overview": {
    title: "Overview", crumbs: [ATL, { label: "Overview" }],
    desc: "A plain document describing the Atlas Deploy platform.",
    fields: [["Type", "—"]],
  },
  "deployment-pipeline": {
    title: "Deployment Pipeline", crumbs: [ATL, { label: "Deployment Pipeline" }],
    desc: "Workstream document for pipeline reliability and speed.",
    fields: [["Type", "Workstream"], ["Status", "Active"]],
  },
  infrastructure: {
    title: "Infrastructure", crumbs: [ATL, { label: "Infrastructure" }],
    desc: "Workstream document for infra work.",
    fields: [["Type", "Workstream"], ["Status", "Active"]],
  },
  "atlas-notes": {
    title: "Notes", crumbs: [ATL, { label: "Notes" }],
    desc: "Loose notes for Atlas Deploy.",
    fields: [["Type", "—"]],
  },
  "atlas-plans": {
    title: "Plans", crumbs: [ATL, { label: "Plans" }],
    desc: "Planning documents for Atlas Deploy.",
    fields: [["Type", "—"]],
  },
};

export type TaskStatus = "Todo" | "In progress" | "Done" | "Blocked";
export type DocStatus = "Active" | "Paused" | "Archived";

export const INITIAL_STATUSES = {
  "task-43": "In progress" as TaskStatus,
  "bug-142": "In progress" as TaskStatus,
  "session-recovery": "Active" as DocStatus,
  "impl-plan": "Active" as DocStatus,
};
