import { useEffect, useRef, type Ref } from "react";
import {
  ArrowClockwise,
  ArrowUUpLeft,
  ArrowUp,
  CaretDown,
  ChartBar,
  Code,
  Cpu,
  Desktop,
  File,
  Folder,
  GitBranch,
  GitFork,
  Lightning,
  ListChecks,
  LockOpen,
  MagnifyingGlass,
  Microphone,
  Paperclip,
  PencilSimple,
  Plus,
  ShieldCheck,
  Sparkle,
  Stop as StopIcon,
  Target,
  UserCircle,
  WarningCircle,
  X,
  Eye,
} from "@phosphor-icons/react";
import { uploadURL } from "../api";
import {
  ACCESS_LEVELS,
  agentLabel,
  EFFORT_LEVELS,
  MODELS,
  runtimeModeTarget,
  type AccessId,
  type EffortId,
} from "../specs";
import type { AgentCatalogEntry } from "../types";
import type { SlashCmd } from "./slash";
import { Popover, PopItem, PopSection } from "./Popover";

function usePickerPageFocus(page: string) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      ref.current
        ?.querySelector<HTMLElement>(
          "[data-popover-autofocus], button:not([disabled])",
        )
        ?.focus();
    });
    return () => cancelAnimationFrame(frame);
  }, [page]);
  return ref;
}

export interface ComposerAttachment {
  path: string;
  name: string;
  isImage: boolean;
  ref?: string;
  partId?: string;
  draftOrdinal?: number;
}

export interface ProjectPickerItem {
  workspace: string;
  label: string;
  subtitle?: string;
  active?: boolean;
}

export interface ProjectPickerProps {
  label: string;
  query: string;
  page: "projects" | "new";
  projects: ProjectPickerItem[];
  selected: boolean;
  onOpen: () => void;
  onQueryChange: (query: string) => void;
  onSelect: (workspace: string) => void;
  onShowNew: () => void;
  onBack: () => void;
  onStartScratch: () => void | Promise<void>;
  onUseExisting: () => void;
  onClear: () => void;
}

export function ProjectPicker({
  label,
  query,
  page,
  projects,
  selected,
  onOpen,
  onQueryChange,
  onSelect,
  onShowNew,
  onBack,
  onStartScratch,
  onUseExisting,
  onClear,
}: ProjectPickerProps) {
  const pageRef = usePickerPageFocus(page);
  return (
    <Popover
      align="left"
      wrapClass="cx-env-project-wrap"
      panelRole="dialog"
      ariaLabel="Project picker"
      trigger={(open, toggle) => (
        <button
          className={"cx-env-control project" + (open ? " active" : "")}
          onClick={toggle}
          title="Select project"
          aria-haspopup="dialog"
          aria-expanded={open}
        >
          <Folder size={17} />
          <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">
            {label}
          </span>
        </button>
      )}
      panelClass="cx-project-popover"
      onOpen={onOpen}
    >
      {(close) => (
        <div className="cx-menu project-menu" ref={pageRef}>
          {page === "projects" ? (
            <>
              <label className="cx-project-search">
                <MagnifyingGlass size={16} />
                <input
                  data-popover-autofocus
                  aria-label="Search projects"
                  placeholder="Search projects"
                  value={query}
                  onChange={(event) => onQueryChange(event.target.value)}
                />
              </label>
              <div className="cx-project-list max-h-[180px] overflow-y-auto pb-[4px] border-b border-line-2">
                {projects.map((project) => (
                  <PopItem
                    key={project.workspace}
                    icon={<Folder size={13} />}
                    title={project.label}
                    desc={project.subtitle}
                    active={project.active}
                    onClick={() => {
                      onSelect(project.workspace);
                      close();
                    }}
                  />
                ))}
                {projects.length === 0 && (
                  <div className="pop-empty">No projects found</div>
                )}
              </div>
              <PopSection>
                <PopItem
                  icon={<Plus size={16} />}
                  title="New project"
                  right={<span aria-hidden>›</span>}
                  onClick={onShowNew}
                />
                <PopItem
                  icon={<X size={15} />}
                  title="Don't work in a project"
                  active={!selected}
                  onClick={() => {
                    onClear();
                    close();
                  }}
                />
              </PopSection>
            </>
          ) : (
            <>
              <div className="pop-menu-title">
                <button
                  className="pop-back"
                  onClick={onBack}
                  aria-label="Back to projects"
                >
                  ‹
                </button>
                <b>New project</b>
              </div>
              <PopItem
                icon={<Sparkle size={15} />}
                title="Start from scratch"
                desc="Create a fresh local workspace"
                onClick={async () => {
                  await onStartScratch();
                  close();
                }}
              />
              <PopItem
                icon={<Folder size={13} />}
                title="Use an existing folder"
                desc="Choose an absolute local path"
                onClick={() => {
                  close();
                  onUseExisting();
                }}
              />
            </>
          )}
        </div>
      )}
    </Popover>
  );
}

export interface RunLocationPickerProps {
  kind: "chat" | "background";
  location: "worktree" | "local";
  worktreeUnavailableReason?: string;
  onSelect: (location: "worktree" | "local") => void;
  onUnavailableWorktree: (reason: string) => void;
}

export function RunLocationPicker({
  kind,
  location,
  worktreeUnavailableReason,
  onSelect,
  onUnavailableWorktree,
}: RunLocationPickerProps) {
  const isBackground = kind === "background";
  return (
    <Popover
      align="left"
      trigger={(open, toggle) => (
        <button
          className={"cx-env-control" + (open ? " active" : "")}
          onClick={toggle}
          title={
            isBackground
              ? "Runs in the background — choose where it runs"
              : "Choose where this session runs"
          }
          aria-haspopup="menu"
          aria-expanded={open}
        >
          {isBackground ? (
            <Lightning size={17} />
          ) : location === "local" ? (
            <Desktop size={17} />
          ) : (
            <GitFork size={17} />
          )}
          <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis">
            {(isBackground ? "Background · " : "") +
              (location === "local" ? "Local" : "New worktree")}
          </span>
        </button>
      )}
    >
      {(close) => (
        <div className="cx-menu">
          <PopSection label="Start in">
            <PopItem
              icon={<GitBranch size={15} />}
              title="New worktree"
              desc={
                worktreeUnavailableReason ||
                "Isolated checkout; your project stays untouched"
              }
              active={location === "worktree"}
              onClick={() => {
                if (worktreeUnavailableReason) {
                  onUnavailableWorktree(worktreeUnavailableReason);
                  return;
                }
                onSelect("worktree");
                close();
              }}
            />
            <PopItem
              icon={<Desktop size={15} />}
              title="Local"
              desc="Work directly in the selected project"
              active={location === "local"}
              onClick={() => {
                onSelect("local");
                close();
              }}
            />
          </PopSection>
        </div>
      )}
    </Popover>
  );
}

export interface BranchPickerProps {
  label: string;
  narrow?: boolean;
  isRepo: boolean;
  location: "worktree" | "local";
  dirty?: number;
  query: string;
  branches: string[];
  totalBranches: number;
  onOpen: () => void;
  onQueryChange: (query: string) => void;
  onSelect: (branch: string, close: () => void) => void | Promise<void>;
}

export function BranchPicker({
  label,
  narrow = false,
  isRepo,
  location,
  dirty = 0,
  query,
  branches,
  totalBranches,
  onOpen,
  onQueryChange,
  onSelect,
}: BranchPickerProps) {
  return (
    <Popover
      align="left"
      wrapClass={narrow ? "min-w-0 flex-1" : ""}
      panelClass="cx-branch-popover"
      panelRole="dialog"
      ariaLabel="Branch picker"
      onOpen={onOpen}
      trigger={(open, toggle) => (
        <button
          className={
            "cx-env-control branch" +
            (narrow ? " w-full" : "") +
            (open ? " active" : "")
          }
          onClick={toggle}
          title={isRepo ? "Choose starting branch" : "No Git branch available"}
          disabled={!isRepo}
          aria-haspopup="dialog"
          aria-expanded={open}
        >
          <GitBranch size={17} />
          <span className="cx-env-value min-w-0 overflow-hidden text-ellipsis [direction:rtl] text-left">
            {label}
          </span>
        </button>
      )}
    >
      {(close) => (
        <div className="cx-menu branch-menu">
          <label className="cx-project-search cx-branch-search">
            <MagnifyingGlass size={16} />
            <input
              data-popover-autofocus
              aria-label="Search branches"
              placeholder="Search branches"
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
            />
          </label>
          <PopSection
            label={
              location === "worktree"
                ? "Start worktree from"
                : `Local branch${dirty ? ` · ${dirty} uncommitted` : ""}`
            }
          >
            {branches.map((branch) => (
              <PopItem
                key={branch}
                icon={<GitBranch size={13} />}
                title={branch}
                active={branch === label}
                onClick={() => onSelect(branch, close)}
              />
            ))}
            {branches.length === 0 && (
              <div className="pop-empty">
                {totalBranches === 0 ? "No branches yet" : "No branches found"}
              </div>
            )}
          </PopSection>
        </div>
      )}
    </Popover>
  );
}

export function AttachmentChip({
  attachment,
  onRemove,
}: {
  attachment: ComposerAttachment;
  onRemove: () => void;
}) {
  return (
    <button
      type="button"
      className="cx-att cx-att-codex"
      onClick={onRemove}
      title="Remove attachment"
      aria-label={`Remove attachment ${attachment.name}`}
    >
      {attachment.isImage ? (
        <img
          className="cx-att-thumb"
          src={uploadURL(attachment.path)}
          alt=""
          aria-hidden
        />
      ) : (
        <span className="cx-att-ico">
          <File size={14} />
        </span>
      )}
      <span className="cx-att-name">{attachment.name}</span>
      <span className="cx-att-x" aria-hidden>
        <X size={11} weight="bold" />
      </span>
    </button>
  );
}

export function AttachmentList({
  attachments,
  onRemove,
}: {
  attachments: ComposerAttachment[];
  onRemove: (index: number) => void;
}) {
  if (attachments.length === 0) return null;
  return (
    <div
      className="cx-atts flex flex-wrap gap-[6px] pt-[12px] px-[14px]"
      role="group"
      aria-label="Attachments"
    >
      {attachments.map((attachment, index) => (
        <AttachmentChip
          key={`${attachment.partId || attachment.path}:${index}`}
          attachment={attachment}
          onRemove={() => onRemove(index)}
        />
      ))}
    </div>
  );
}

export interface FileMentionMenuProps {
  query: string;
  known: boolean;
  files: string[];
  activeIndex: number;
  onActiveIndexChange: (index: number) => void;
  onSelect: (file: string) => void;
}

export function FileMentionMenu({
  query,
  known,
  files,
  activeIndex,
  onActiveIndexChange,
  onSelect,
}: FileMentionMenuProps) {
  return (
    <div
      className="cx-slash cx-at"
      role={files.length > 0 ? "listbox" : "status"}
      aria-label="Workspace files"
    >
      <div className="cx-slash-hd">
        {known ? `Files · @${query}` : "Workspace unknown"}
      </div>
      {!known && (
        <div className="cx-at-empty">
          This session&apos;s workspace isn&apos;t known to arwebui, so files
          can&apos;t be listed.
        </div>
      )}
      {known && files.length === 0 && (
        <div className="cx-at-empty">No matching files</div>
      )}
      {files.map((file, index) => (
        <button
          key={file}
          type="button"
          role="option"
          aria-selected={index === activeIndex}
          className={"cx-slash-item" + (index === activeIndex ? " on" : "")}
          onMouseEnter={() => onActiveIndexChange(index)}
          onClick={() => onSelect(file)}
        >
          <span className="cx-slash-name mono">{file}</span>
        </button>
      ))}
    </div>
  );
}

export interface SlashCommandMenuProps {
  commands: SlashCmd[];
  activeIndex: number;
  onActiveIndexChange: (index: number) => void;
  onSelect: (command: SlashCmd) => void;
}

export function SlashCommandMenu({
  commands,
  activeIndex,
  onActiveIndexChange,
  onSelect,
}: SlashCommandMenuProps) {
  if (commands.length === 0) return null;
  return (
    <div
      className="cx-slash cx-slash-codex"
      role="listbox"
      aria-label="Slash commands"
    >
      <div className="cx-slash-hd">Commands</div>
      {commands.map((command, index) => (
        <button
          key={command.name}
          type="button"
          role="option"
          aria-selected={index === activeIndex}
          className={"cx-slash-item" + (index === activeIndex ? " on" : "")}
          onMouseEnter={() => onActiveIndexChange(index)}
          onClick={() => onSelect(command)}
        >
          <span className="cx-slash-text">
            <span className="cx-slash-name">/{command.name}</span>
            <span className="cx-slash-desc">{command.desc}</span>
          </span>
          {command.arg && (
            <span className="cx-slash-hint">{command.arg}</span>
          )}
        </button>
      ))}
    </div>
  );
}

export interface AddMenuProps {
  page: "root" | "advanced" | "agent";
  isSession: boolean;
  goalMode: boolean;
  planMode: boolean;
  kind: "chat" | "background";
  persona: string;
  agents: AgentCatalogEntry[];
  onOpen: () => void;
  onPageChange: (page: "root" | "advanced" | "agent") => void;
  onPickFiles: () => void;
  onToggleGoal: () => void;
  onTogglePlan: () => void;
  onStartLoop: () => void;
  onStartBest: () => void;
  onToggleBackground: () => void;
  onSelectPersona: (persona: string) => void | Promise<void>;
  onEditSpec: () => void;
}

export function AddMenu({
  page,
  isSession,
  goalMode,
  planMode,
  kind,
  persona,
  agents,
  onOpen,
  onPageChange,
  onPickFiles,
  onToggleGoal,
  onTogglePlan,
  onStartLoop,
  onStartBest,
  onToggleBackground,
  onSelectPersona,
  onEditSpec,
}: AddMenuProps) {
  const pageRef = usePickerPageFocus(page);
  return (
    <Popover
      align="left"
      panelClass="cx-pop-codex"
      onOpen={onOpen}
      trigger={(open, toggle) => (
        <button
          type="button"
          className={"cx-icon" + (open ? " active" : "")}
          onClick={toggle}
          title="Add & advanced options"
          aria-label="Add and advanced options"
          aria-haspopup="menu"
          aria-expanded={open}
        >
          <Plus size={16} />
        </button>
      )}
    >
      {(close) => (
        <div
          className={
            "cx-menu cx-add-menu [&_.pop-body]:flex-row [&_.pop-body]:items-baseline [&_.pop-body]:gap-2 " +
            "[&_.pop-title]:shrink-0 [&_.pop-desc]:min-w-0 [&_.pop-desc]:truncate" +
            (page === "agent" ? " cx-add-agent" : "")
          }
          ref={pageRef}
          style={{ width: 320, maxWidth: "calc(100vw - 32px)" }}
          onClick={(event) => event.preventDefault()}
        >
          {page === "root" ? (
            <>
              <PopSection label="Add">
                <PopItem
                  icon={<Paperclip size={16} />}
                  title="Files and folders"
                  onClick={() => {
                    close();
                    onPickFiles();
                  }}
                />
                <PopItem
                  icon={<Target size={14} />}
                  title="Goal"
                  desc={
                    goalMode
                      ? "Turn goal mode off"
                      : "Set a goal to keep pursuing"
                  }
                  active={goalMode}
                  onClick={() => {
                    close();
                    onToggleGoal();
                  }}
                />
                <PopItem
                  icon={<ListChecks size={14} />}
                  title="Plan mode"
                  desc={
                    planMode ? "Turn plan mode off" : "Turn plan mode on"
                  }
                  active={!isSession && planMode}
                  disabled={isSession}
                  onClick={
                    !isSession
                      ? () => {
                          close();
                          onTogglePlan();
                        }
                      : undefined
                  }
                />
              </PopSection>
              <PopSection label="Advanced">
                <PopItem
                  icon={<Lightning size={16} />}
                  title="Automation"
                  desc={
                    kind === "background"
                      ? "Background run"
                      : agentLabel(persona)
                  }
                  right={<span aria-hidden>›</span>}
                  onClick={() => onPageChange("advanced")}
                />
              </PopSection>
            </>
          ) : page === "advanced" ? (
            <>
              <div className="pop-menu-title">
                <button
                  className="pop-back"
                  role="menuitem"
                  onClick={() => onPageChange("root")}
                  aria-label="Back to add menu"
                >
                  ‹
                </button>
                <b>Automation</b>
              </div>
              <PopItem
                icon={<ArrowClockwise size={14} />}
                title="Loop"
                desc="Repeat on a cadence"
                onClick={() => {
                  close();
                  onStartLoop();
                }}
              />
              <PopItem
                icon={<ChartBar size={14} />}
                title="Best of N"
                desc="Keep the best of N tries"
                onClick={() => {
                  close();
                  onStartBest();
                }}
              />
              {!isSession && (
                <PopItem
                  icon={<Lightning size={16} />}
                  title="Background run"
                  desc="Run headless, no chat"
                  active={kind === "background"}
                  onClick={() => {
                    onToggleBackground();
                    close();
                  }}
                />
              )}
              <PopItem
                icon={<UserCircle size={13} />}
                title="Agent"
                desc={
                  agentLabel(persona)
                }
                right={<span aria-hidden>›</span>}
                onClick={() => onPageChange("agent")}
              />
            </>
          ) : (
            <>
              <div className="pop-menu-title">
                <button
                  className="pop-back"
                  role="menuitem"
                  onClick={() => onPageChange("advanced")}
                  aria-label="Back to automation menu"
                >
                  ‹
                </button>
                <b>Agent</b>
              </div>
              {agents.map((item) => (
                <PopItem
                  key={item.name}
                  icon={<UserCircle size={13} />}
                  title={agentLabel(item.name)}
                  desc={`${item.description || "Custom Agent"} · ${item.source}`}
                  active={persona === item.name}
                  onClick={() => {
                    onSelectPersona(item.name);
                    close();
                  }}
                />
              ))}
              <PopSection>
                <PopItem
                  icon={<Code size={16} />}
                  title="Edit agent spec (YAML)…"
                  onClick={() => {
                    close();
                    onEditSpec();
                  }}
                />
              </PopSection>
            </>
          )}
        </div>
      )}
    </Popover>
  );
}

const accessIconById: Record<AccessId, typeof LockOpen> = {
  full: LockOpen,
  acceptEdits: PencilSimple,
  ask: ShieldCheck,
  plan: Eye,
};

function AccessIcon({ id, risk }: { id: AccessId; risk: string }) {
  const Icon = accessIconById[id] || ShieldCheck;
  return (
    <span className={`cx-access-ico ${risk}`}>
      <Icon size={16} />
    </span>
  );
}

function RiskGlyph({ risk }: { risk: string }) {
  return risk === "high" ? (
    <WarningCircle
      size={15}
      weight="regular"
      className="shrink-0"
      style={{ color: "var(--amber)" }}
    />
  ) : (
    <span
      className={`risk-dot h-[7px] w-[7px] shrink-0 rounded-full ${risk}`}
    />
  );
}

export interface AccessPickerProps {
  variant: "home" | "session";
  active?: AccessId;
  label?: string;
  risk?: string;
  triggerRef?: Ref<HTMLButtonElement>;
  onHomeSelect?: (access: AccessId, close: () => void) => void;
  onSessionSelect?: (
    target: "default" | "acceptEdits",
    close: () => void,
  ) => void;
}

export function AccessPicker({
  variant,
  active,
  label = "Access: set by agent spec",
  risk = "unknown",
  triggerRef,
  onHomeSelect,
  onSessionSelect,
}: AccessPickerProps) {
  const session = variant === "session";
  return (
    <Popover
      align="left"
      panelClass="cx-pop-codex"
      panelRole={session ? "dialog" : "menu"}
      ariaLabel={session ? "Access options" : undefined}
      trigger={(open, toggle) => (
        <button
          type="button"
          ref={triggerRef}
          className={`cx-pill cx-mode ${risk}${open ? " active" : ""}`}
          onClick={toggle}
          aria-label={session ? label : undefined}
          aria-haspopup={session ? "dialog" : "menu"}
          aria-expanded={open}
          title={
            session
              ? active
                ? "The session's live approval mode — click to switch Ask ↔ Auto-accept edits"
                : "This session's approval posture comes from its spec"
              : "How the agent's actions are approved"
          }
        >
          <RiskGlyph risk={risk} />
          <span className="cx-mode-label">{label}</span>
        </button>
      )}
    >
      {(close) => (
        <div className="cx-menu wide cx-access-menu">
          <PopSection
            label={
              session
                ? "Switch approval mode"
                : "How should actions be approved?"
            }
          >
            {ACCESS_LEVELS.map((item) => {
              const target = runtimeModeTarget(item.id);
              const desc =
                session && item.id === "full"
                  ? "Set at launch — mid-session switching only toggles Ask ↔ Auto-accept edits"
                  : session && item.id === "plan"
                    ? "Plan mode exits through an approval, not this switch"
                    : item.desc;
              return (
                <PopItem
                  key={item.id}
                  icon={<AccessIcon id={item.id} risk={item.risk} />}
                  title={item.label}
                  desc={desc}
                  active={active === item.id}
                  disabled={session && target === null}
                  onClick={
                    session
                      ? target
                        ? () => onSessionSelect?.(target, close)
                        : undefined
                      : () => onHomeSelect?.(item.id, close)
                  }
                />
              );
            })}
          </PopSection>
          <div className="cx-pop-note">
            {session
              ? "Approvals still surface here whenever a gate asks. Full access and Plan are fixed once the session starts."
              : "Approvals still surface here whenever a gate asks; the posture is fixed once the session starts."}
          </div>
        </div>
      )}
    </Popover>
  );
}

export interface ModelPickerProps {
  provider: string;
  model: string;
  modelLabel: string;
  effort: EffortId;
  effortLabel: string;
  page: "root" | "model" | "effort" | "advanced";
  onOpen: () => void;
  onPageChange: (page: "root" | "model" | "effort" | "advanced") => void;
  onSelectModel: (provider: string, model: string) => void | Promise<void>;
  onSelectEffort: (effort: EffortId) => void | Promise<void>;
  onCustomModel: () => void;
}

export function ModelPicker({
  provider,
  model,
  modelLabel,
  effort,
  effortLabel,
  page,
  onOpen,
  onPageChange,
  onSelectModel,
  onSelectEffort,
  onCustomModel,
}: ModelPickerProps) {
  const pageRef = usePickerPageFocus(page);
  return (
    <Popover
      align="right"
      panelClass="cx-pop-codex"
      onOpen={onOpen}
      trigger={(open, toggle) => (
        <button
          type="button"
          className={"cx-pill cx-model" + (open ? " active" : "")}
          onClick={toggle}
          title="Model & effort"
          aria-haspopup="menu"
          aria-expanded={open}
        >
          <span className="cx-model-name">{modelLabel}</span>
          <span className="cx-pill-sub">
            {effortLabel}
          </span>
          <CaretDown className="cx-caret text-dim shrink-0" size={10} />
        </button>
      )}
    >
      {(close) => (
        <div
          className="cx-menu wide cx-model-menu"
          ref={pageRef}
          style={{ width: 320, maxWidth: "calc(100vw - 32px)" }}
          onClick={(event) => event.preventDefault()}
        >
          {page === "root" ? (
            <>
              <div className="cx-model-roots">
                <PopItem
                  title="Model"
                  right={
                    <span className="inline-flex max-w-[210px] items-center gap-2">
                      <span className="truncate">{modelLabel}</span>
                      <span aria-hidden>›</span>
                    </span>
                  }
                  onClick={() => onPageChange("model")}
                />
                <PopItem
                  title="Effort"
                  right={
                    <span className="inline-flex max-w-[210px] items-center gap-2">
                      <span className="truncate">
                        {effortLabel}
                      </span>
                      <span aria-hidden>›</span>
                    </span>
                  }
                  onClick={() => onPageChange("effort")}
                />
              </div>
              <div className="cx-model-advanced">
                <PopItem
                  title={
                    <span className="inline-flex items-center gap-1">
                      Advanced
                      <CaretDown
                        size={14}
                        className="cx-model-adv-chev open"
                        aria-hidden="true"
                      />
                    </span>
                  }
                  onClick={() => onPageChange("advanced")}
                />
              </div>
            </>
          ) : page === "model" ? (
            <>
              <PickerBack title="Model" onBack={() => onPageChange("root")} />
              <div className="cx-model-list">
                {MODELS.map((item) => (
                  <PopItem
                    key={item.provider + item.id}
                    icon={
                      item.provider === "anthropic" ? (
                        <Cpu size={14} />
                      ) : (
                        <Sparkle size={14} />
                      )
                    }
                    title={item.label}
                    desc={item.sub}
                    active={provider === item.provider && model === item.id}
                    onClick={() => {
                      onSelectModel(item.provider, item.id);
                      close();
                    }}
                  />
                ))}
              </div>
            </>
          ) : page === "effort" ? (
            <>
              <PickerBack title="Effort" onBack={() => onPageChange("root")} />
              {EFFORT_LEVELS.map((item) => (
                <PopItem
                  key={item.id}
                  title={item.label}
                  desc={item.desc}
                  active={effort === item.id}
                  onClick={() => {
                    onSelectEffort(item.id);
                    close();
                  }}
                />
              ))}
            </>
          ) : (
            <>
              <PickerBack
                title="Advanced"
                onBack={() => onPageChange("root")}
              />
              <PopItem
                icon={<Code size={15} />}
                title="Custom model id…"
                desc={`provider stays ${provider}`}
                onClick={() => {
                  close();
                  onCustomModel();
                }}
              />
            </>
          )}
        </div>
      )}
    </Popover>
  );
}

function PickerBack({
  title,
  onBack,
}: {
  title: string;
  onBack: () => void;
}) {
  return (
    <div className="pop-menu-title">
      <button
        type="button"
        className="pop-back"
        role="menuitem"
        onClick={onBack}
        aria-label="Back to model menu"
      >
        ‹
      </button>
      <b>{title}</b>
    </div>
  );
}

export interface GoalOptionsProps {
  verifier: string;
  rounds: number;
  onVerifierChange: (verifier: string) => void;
  onRoundsChange: (rounds: number) => void;
  onExit: () => void;
}

export function GoalOptions({
  verifier,
  rounds,
  onVerifierChange,
  onRoundsChange,
  onExit,
}: GoalOptionsProps) {
  return (
    <Popover
      align="left"
      panelClass="cx-pop-codex"
      panelRole="dialog"
      ariaLabel="Goal options"
      trigger={(open, toggle) => (
        <button
          type="button"
          className={"cx-pill cx-goal-mode" + (open ? " active" : "")}
          onClick={toggle}
          aria-haspopup="dialog"
          aria-expanded={open}
          title="Goal mode — configure completion checks or exit"
        >
          <Target size={14} />
          Goal
        </button>
      )}
    >
      {(close) => (
        <div className="cx-menu wide cx-goal-options">
          <div className="pop-menu-title">
            <b>Goal options</b>
          </div>
          <label
            className="cx-launcher-field"
            title="Optional shell command that must exit 0 for the goal to count as met"
          >
            <span>Done when (command)</span>
            <input
              placeholder="e.g. go test ./…  (empty = agent self-certifies)"
              value={verifier}
              onChange={(event) => onVerifierChange(event.target.value)}
            />
          </label>
          <label
            className="cx-launcher-field small"
            title="Safety cap on iterations"
          >
            <span>Max rounds</span>
            <input
              type="number"
              min={1}
              value={rounds}
              onChange={(event) =>
                onRoundsChange(Math.max(1, Number(event.target.value) || 1))
              }
            />
          </label>
          <PopItem
            title="Exit Goal mode"
            onClick={() => {
              onExit();
              close();
            }}
          />
        </div>
      )}
    </Popover>
  );
}

export interface AssistActionsProps {
  hasText: boolean;
  canUndo: boolean;
  optimizing: boolean;
  micVisible: boolean;
  micActive: boolean;
  dictationBusy: boolean;
  onOptimize: () => void;
  onUndo: () => void;
  onToggleMic: () => void;
}

export function AssistActions({
  hasText,
  canUndo,
  optimizing,
  micVisible,
  micActive,
  dictationBusy,
  onOptimize,
  onUndo,
  onToggleMic,
}: AssistActionsProps) {
  return (
    <>
      {canUndo ? (
        <button
          type="button"
          className="cx-icon cx-undo"
          onClick={onUndo}
          title="Undo optimize — restore your original draft"
        >
          <ArrowUUpLeft size={15} />
        </button>
      ) : (
        hasText && (
          <button
            type="button"
            className={"cx-icon cx-optimize" + (optimizing ? " working" : "")}
            onClick={onOptimize}
            disabled={optimizing}
            title="Optimize prompt — rewrite this draft to be clearer"
          >
            <Sparkle
              size={15}
              weight={optimizing ? "fill" : "regular"}
            />
          </button>
        )
      )}
      {micVisible && (
        <button
          type="button"
          className={
            "cx-icon cx-mic" +
            (micActive ? " listening" : "") +
            (dictationBusy ? " working" : "")
          }
          onClick={onToggleMic}
          disabled={dictationBusy}
          title={
            dictationBusy
              ? "Transcribing…"
              : micActive
                ? "Stop dictation"
                : "Dictate"
          }
        >
          <Microphone size={15} />
        </button>
      )}
    </>
  );
}

export function DeliveryModeControl({
  mode,
  onChange,
}: {
  mode: "queue" | "steer";
  onChange: (mode: "queue" | "steer") => void;
}) {
  return (
    <div className="cx-delivery" role="group" aria-label="Delivery mode">
      <button
        type="button"
        className={"cx-deliv" + (mode === "queue" ? " on" : "")}
        aria-pressed={mode === "queue"}
        onClick={() => onChange("queue")}
        title="Queue: deliver after the current turn ends (⌘⏎ to steer this one)"
      >
        <ListChecks size={14} />
        <span className="cx-deliv-label">Queue</span>
      </button>
      <button
        type="button"
        className={"cx-deliv" + (mode === "steer" ? " on" : "")}
        aria-pressed={mode === "steer"}
        onClick={() => onChange("steer")}
        title="Steer: fold into the current turn at its next safe boundary (⌘⏎ to queue this one)"
      >
        <Lightning size={14} />
        <span className="cx-deliv-label">Steer</span>
      </button>
    </div>
  );
}

export interface SubmitButtonProps {
  mode: "send" | "stop";
  disabled?: boolean;
  running?: boolean;
  deliveryMode?: "queue" | "steer";
  onSubmit: () => void;
}

export function SubmitButton({
  mode,
  disabled = false,
  running = false,
  deliveryMode = "queue",
  onSubmit,
}: SubmitButtonProps) {
  if (mode === "stop") {
    return (
      <button
        type="button"
        className="cx-send cx-stop"
        onClick={onSubmit}
        aria-label="Stop active turn"
        title="Stop the active turn"
      >
        <StopIcon size={15} weight="fill" />
      </button>
    );
  }
  return (
    <button
      type="button"
      className="cx-send"
      onClick={onSubmit}
      disabled={disabled}
      aria-label="Send message"
      title={
        running
          ? `Send · ${deliveryMode} (⌘⏎ to ${
              deliveryMode === "queue" ? "steer" : "queue"
            })`
          : "Send (Enter)"
      }
    >
      <ArrowUp />
    </button>
  );
}
