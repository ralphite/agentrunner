import type { Ref } from "react";
import {
  ArrowClockwise,
  ArrowsHorizontal,
  ArrowsInLineVertical,
  ArrowsOutLineVertical,
  CaretDown,
  ClockCounterClockwise,
  Columns,
  Copy,
  DotsThree,
  FileDashed,
  FileMagnifyingGlass,
  FolderDashed,
  GitBranch,
  GitCommit,
  MagnifyingGlass,
  Rows,
  TextAlignLeft,
  TreeStructure,
  X,
} from "@phosphor-icons/react";
import { splitPath, type FileStatus } from "../diffSummary";
import type { DiffScope } from "../types";
import { Button } from "../ui/Button";
import { IconButton } from "../ui/IconButton";
import { SearchField } from "../ui/Field";
import { Popover, PopItem, PopSection } from "./Popover";

const STATUS_GLYPH: Record<FileStatus, string> = {
  modified: "M",
  added: "A",
  deleted: "D",
  renamed: "R",
  copied: "C",
};

const HIDDEN_NOTE_TITLE =
  "Untracked files that look generated — dependencies, build output — are omitted so the review stays responsive. Every source file remains visible.";

export interface DiffChangedFile {
  path: string;
  status: FileStatus;
  add: number | null;
  del: number;
  binary: boolean;
  conflict: boolean;
}

export function DiffScopePicker({
  scope,
  triggerRef,
  onSelect,
}: {
  scope: DiffScope;
  triggerRef?: Ref<HTMLButtonElement>;
  onSelect: (scope: DiffScope) => void;
}) {
  return (
    <Popover
      panelClass="diff-scope-menu"
      trigger={(open, toggle) => (
        <button
          ref={triggerRef}
          className={
            "diff-scope-trigger inline-flex shrink-0 items-center gap-1 whitespace-nowrap" +
            (open ? " active" : "")
          }
          onClick={toggle}
          aria-label="Change diff scope"
          aria-haspopup="menu"
          aria-expanded={open}
          title="Choose which workspace changes to review"
        >
          {scope === "working-tree" ? "Working Tree" : "Last Turn"}
          <CaretDown size={12} />
        </button>
      )}
    >
      {(close) => (
        <PopSection label="Compare changes">
          <PopItem
            title="Working Tree"
            desc="All uncommitted workspace changes"
            active={scope === "working-tree"}
            onClick={() => {
              onSelect("working-tree");
              close();
            }}
          />
          <PopItem
            title="Last Turn"
            desc="Since the latest human turn began"
            active={scope === "last-turn"}
            onClick={() => {
              onSelect("last-turn");
              close();
            }}
          />
        </PopSection>
      )}
    </Popover>
  );
}

const SKEL_FILES: { path: number; rows: number[] }[] = [
  { path: 152, rows: [74, 46, 62, 38, 84, 54, 30] },
  { path: 108, rows: [58, 80, 42, 66, 34] },
  { path: 176, rows: [] },
];

export function DiffSkeleton() {
  return (
    <div className="diff-skeleton" role="status" aria-label="Loading changes">
      {SKEL_FILES.map((file, index) => (
        <div className="dsk-file" key={index}>
          <div className="dsk-head">
            <span className="dsk-bar dsk-glyph" />
            <span
              className="dsk-bar dsk-path"
              style={{ width: file.path }}
            />
            <span className="dsk-bar dsk-counts" />
          </div>
          {file.rows.length > 0 && (
            <div className="dsk-body">
              {file.rows.map((width, row) => (
                <div className="dsk-row" key={row}>
                  <span className="dsk-bar dsk-marker" />
                  <span className="dsk-bar dsk-no" />
                  <span
                    className="dsk-bar dsk-code"
                    style={{ width: width + "%" }}
                  />
                </div>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

export type DiffState =
  | { kind: "loading" }
  | { kind: "error"; message: string; onRetry: () => void }
  | { kind: "last-turn-unavailable"; reason?: string }
  | { kind: "workspace-unavailable"; onRetry: () => void }
  | { kind: "nested"; busy: boolean; onTrack: () => void }
  | { kind: "non-repo"; busy: boolean; onTrack: () => void }
  | { kind: "empty"; scope: DiffScope }
  | { kind: "no-matches"; query: string; fileCount: number };

export function DiffStateView({ state }: { state: DiffState }) {
  if (state.kind === "loading") return <DiffSkeleton />;
  if (state.kind === "error") {
    return (
      <div className="diff-empty">
        <b>Couldn’t load changes</b>
        <span>{state.message}</span>
        <Button size="md" variant="outline" onClick={state.onRetry}>Try again</Button>
      </div>
    );
  }
  if (state.kind === "last-turn-unavailable") {
    return (
      <div className="diff-empty">
        <ClockCounterClockwise size={26} weight="light" />
        <b>Last turn unavailable</b>
        <span>
          {state.reason ||
            "This session has no durable workspace baseline for its latest human turn."}
        </span>
        <span className="dim">
          Working tree remains available for the session's current uncommitted
          changes.
        </span>
      </div>
    );
  }
  if (state.kind === "workspace-unavailable") {
    return (
      <div className="diff-empty">
        <FolderDashed size={26} weight="light" />
        <b>Workspace unavailable</b>
        <span>
          This session predates workspace metadata, so AgentRunner cannot
          reconstruct its changes view.
        </span>
        <Button size="md" variant="outline" onClick={state.onRetry}>Try again</Button>
      </div>
    );
  }
  if (state.kind === "nested") {
    return (
      <div className="diff-empty">
        <GitBranch size={26} weight="light" />
        <b>Changes can't be tracked here yet</b>
        <span>
          This session's workspace sits inside another repository, so its files
          aren't tracked on their own.
        </span>
        <Button
          size="md"
          variant="solid"
          className="primary"
          onClick={state.onTrack}
          disabled={state.busy}
          title="git init in the workspace — safe, local-only"
        >
          Track changes (git init)
        </Button>
      </div>
    );
  }
  if (state.kind === "non-repo") {
    return (
      <div className="diff-empty">
        <GitBranch size={26} weight="light" />
        <b>No Git changes to review</b>
        <span>This session's workspace has no version control yet.</span>
        <Button
          size="md"
          variant="solid"
          className="primary"
          onClick={state.onTrack}
          disabled={state.busy}
          title="git init in the workspace — safe, local-only"
        >
          Track changes (git init)
        </Button>
      </div>
    );
  }
  if (state.kind === "no-matches") {
    return (
      <div className="diff-empty">
        <FileMagnifyingGlass size={26} weight="light" />
        <b>No matching files</b>
        <span>
          No changed file’s path contains “{state.query}”. Clear the filter to
          see all {state.fileCount} of them.
        </span>
      </div>
    );
  }
  return (
    <div className="diff-empty">
      <FileDashed size={26} weight="light" />
      {state.scope === "last-turn" ? (
        <>
          <b>No changes this turn</b>
          <span>
            The agent hasn't touched the workspace since the latest human turn
            began.
          </span>
        </>
      ) : (
        <>
          <b>No changes yet</b>
          <span>
            Edits the agent makes to the workspace will show up here.
          </span>
        </>
      )}
    </div>
  );
}

export function ChangedFilesMenu({
  files,
  fileCount,
  query,
  hiddenUntracked,
  onQueryChange,
  onFocusFile,
}: {
  files: DiffChangedFile[];
  fileCount: number;
  query: string;
  hiddenUntracked: number;
  onQueryChange: (query: string) => void;
  onFocusFile: (path: string) => void;
}) {
  if (fileCount <= 1 && hiddenUntracked === 0) return null;
  const filtering = query.trim().length > 0;
  return (
      <Popover
        align="right"
        panelClass="diff-files-menu"
        panelRole="dialog"
        ariaLabel="Changed files"
        trigger={(open, toggle) => (
        <IconButton
          size="md"
          variant="ghost"
          pressed={open || filtering}
          onClick={toggle}
          aria-label="Changed files"
          aria-haspopup="dialog"
          aria-expanded={open}
          title={
            filtering
              ? "Changed files — filtering by “" + query + "”"
              : "Changed files — jump to one, or filter the review"
          }
        >
          <TreeStructure size={15} />
        </IconButton>
      )}
    >
      {(close) => (
        <>
          <PopSection
            label={
              filtering
                ? `${files.length} of ${fileCount} files match`
                : `${fileCount} files changed`
            }
          >
            <SearchField
              data-popover-autofocus
              type="text"
              containerClassName="mx-[6px] py-[5px]"
              className="text-[12px]"
              icon={<MagnifyingGlass size={13} />}
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
              placeholder="Filter files…"
              aria-label="Filter files by path"
            />
            {files.length === 0 ? (
              <div className="diff-filelist-empty">
                No changed file’s path contains “{query}”.
              </div>
            ) : (
              <div className="diff-filelist">
                {files.map((file) => {
                  const { dir, base } = splitPath(file.path);
                  return (
                    <button
                      key={file.path}
                      type="button"
                      className="diff-fileitem mono"
                      title={file.path}
                      onClick={() => {
                        close();
                        onFocusFile(file.path);
                      }}
                    >
                      <span
                        className={"fd-glyph fd-glyph-" + file.status}
                        aria-hidden="true"
                      >
                        {STATUS_GLYPH[file.status]}
                      </span>
                      <span className="diff-fileitem-path">
                        {dir && <span className="fd-dir">{dir}</span>}
                        <b
                          style={{
                            fontWeight: 600,
                            color: "var(--ink)",
                          }}
                        >
                          {base}
                        </b>
                      </span>
                      {file.conflict && (
                        <span className="fd-badge conflict">conflict</span>
                      )}
                      {!file.binary && (
                        <span className="fd-counts">
                          <span className="add">
                            +{file.add === null ? "…" : file.add}
                          </span>
                          <span className="del">-{file.del}</span>
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            )}
          </PopSection>
          {hiddenUntracked > 0 && (
            <div className="diff-hidden-note" title={HIDDEN_NOTE_TITLE}>
              <b>{hiddenUntracked.toLocaleString()} generated files hidden</b>
              <span>Source files all still shown.</span>
            </div>
          )}
        </>
      )}
    </Popover>
  );
}

export interface DiffMoreActionsMenuProps {
  fileCount: number;
  allShownOpen: boolean;
  barTight: boolean;
  empty: boolean;
  wrap: boolean;
  narrow: boolean;
  view: "inline" | "split";
  scope: DiffScope;
  worktree: boolean;
  mainRepo?: string;
  busy: boolean;
  onToggleAll: () => void;
  onToggleWrap: () => void;
  onCopy: () => void;
  onToggleView: () => void;
  onRefresh: () => void;
  onApplyProject: () => void;
  onRemoveWorktree: () => void;
}

export function DiffMoreActionsMenu({
  fileCount,
  allShownOpen,
  barTight,
  empty,
  wrap,
  narrow,
  view,
  scope,
  worktree,
  mainRepo,
  busy,
  onToggleAll,
  onToggleWrap,
  onCopy,
  onToggleView,
  onRefresh,
  onApplyProject,
  onRemoveWorktree,
}: DiffMoreActionsMenuProps) {
  return (
    <Popover
      align="right"
      panelClass="diff-more-menu"
      trigger={(open, toggle) => (
        <IconButton
          size="md"
          variant="ghost"
          pressed={open}
          onClick={toggle}
          aria-label="More changes actions"
          aria-haspopup="menu"
          aria-expanded={open}
          title="More actions"
        >
          <DotsThree size={18} weight="bold" />
        </IconButton>
      )}
    >
      {(close) => (
        <PopSection label="Changes">
          {fileCount > 1 && (
            <PopItem
              icon={
                allShownOpen ? (
                  <ArrowsInLineVertical size={15} />
                ) : (
                  <ArrowsOutLineVertical size={15} />
                )
              }
              title={
                allShownOpen ? "Collapse all files" : "Expand all files"
              }
              desc={
                allShownOpen
                  ? "Fold every file down to its header"
                  : "Open every file's diff"
              }
              onClick={() => {
                close();
                onToggleAll();
              }}
            />
          )}
          {barTight && !empty && (
            <PopItem
              icon={
                wrap ? (
                  <TextAlignLeft size={15} />
                ) : (
                  <ArrowsHorizontal size={15} />
                )
              }
              title={wrap ? "Disable line wrap" : "Wrap long lines"}
              desc={
                wrap
                  ? "Let long lines scroll horizontally again"
                  : "Soft-wrap long diff lines so nothing is clipped"
              }
              onClick={() => {
                close();
                onToggleWrap();
              }}
            />
          )}
          {barTight && !empty && (
            <PopItem
              icon={<Copy size={15} />}
              title="Copy diff"
              desc="Copy the whole unified diff to the clipboard"
              onClick={() => {
                close();
                onCopy();
              }}
            />
          )}
          {barTight && !empty && !narrow && (
            <PopItem
              icon={view === "split" ? <Rows size={15} /> : <Columns size={15} />}
              title={view === "split" ? "Inline view" : "Split view"}
              desc={
                view === "split"
                  ? "Show changes in one column"
                  : "Show old and new side by side"
              }
              onClick={() => {
                close();
                onToggleView();
              }}
            />
          )}
          <PopItem
            title="Refresh changes"
            desc="Re-read the workspace diff"
            onClick={() => {
              close();
              onRefresh();
            }}
          />
          {scope === "working-tree" && worktree && mainRepo && (
            <PopItem
              title="Apply to project…"
              desc={
                "Apply these changes back onto " +
                mainRepo +
                " (unstaged, for review)"
              }
              disabled={busy || empty}
              onClick={() => {
                close();
                onApplyProject();
              }}
            />
          )}
          {scope === "working-tree" && worktree && (
            <PopItem
              title="Remove worktree…"
              desc="Delete this worktree checkout and prune it from git"
              danger
              disabled={busy}
              onClick={() => {
                close();
                onRemoveWorktree();
              }}
            />
          )}
        </PopSection>
      )}
    </Popover>
  );
}

export function CommitPushMenu({
  isRepo,
  busy,
  empty,
  conflictCount,
  compact,
  onCommit,
  onCommitAndPush,
  onPush,
}: {
  isRepo: boolean;
  busy: boolean;
  empty: boolean;
  conflictCount: number;
  compact: boolean;
  onCommit: () => void;
  onCommitAndPush: () => void;
  onPush: () => void;
}) {
  const conflict = conflictCount > 0;
  return (
    <Popover
      align="right"
      panelClass="w-[264px] max-w-[calc(100vw-24px)]"
      trigger={(open, toggle) => (
        <button
          className={
            "sm diff-commit-btn" +
            (open ? " active" : "") +
            (compact ? " diff-commit-compact" : "")
          }
          onClick={toggle}
          disabled={busy || !isRepo}
          aria-label="Commit or push"
          aria-haspopup="menu"
          aria-expanded={open}
          title={
            !isRepo
              ? "This workspace is not a Git repository"
              : conflict
                ? "Resolve merge conflicts before committing — Push remains available"
                : empty
                  ? "No workspace changes to commit — you can still push existing commits"
                  : "Commit or push the workspace changes"
          }
        >
          <GitCommit size={14} />
          {!compact && (
            <>
              Commit or push
              <CaretDown size={12} className="diff-commit-caret" />
            </>
          )}
        </button>
      )}
    >
      {(close) => (
        <PopSection label="Commit or push">
          <PopItem
            title="Commit"
            desc={
              conflict
                ? "Resolve merge conflicts before committing"
                : "git add -A && git commit locally (no push)"
            }
            disabled={empty || conflict}
            onClick={() => {
              close();
              onCommit();
            }}
          />
          <PopItem
            title="Commit &amp; push"
            desc={
              conflict
                ? "Resolve merge conflicts before committing"
                : "Commit locally, then push to the upstream branch"
            }
            disabled={empty || conflict}
            onClick={() => {
              close();
              onCommitAndPush();
            }}
          />
          <PopItem
            title="Push"
            desc="Push existing commits to the upstream branch"
            onClick={() => {
              close();
              onPush();
            }}
          />
        </PopSection>
      )}
    </Popover>
  );
}

interface DiffToolbarBaseProps {
  scope: DiffScope;
  scopeTriggerRef?: Ref<HTMLButtonElement>;
  onScopeChange: (scope: DiffScope) => void;
  onRefresh: () => void;
  onClose?: () => void;
}

export type DiffToolbarProps =
  | (DiffToolbarBaseProps & { variant: "state" })
  | (DiffToolbarBaseProps & {
      variant: "ready";
      barRef?: (element: HTMLDivElement | null) => void;
      barTight: boolean;
      empty: boolean;
      totalAdd: number;
      totalDel: number;
      worktree: boolean;
      mainRepo?: string;
      branch?: string;
      chipCompact: boolean;
      files: DiffChangedFile[];
      fileCount: number;
      query: string;
      hiddenUntracked: number;
      allShownOpen: boolean;
      narrow: boolean;
      view: "inline" | "split";
      wrap: boolean;
      busy: boolean;
      isRepo: boolean;
      conflictCount: number;
      onQueryChange: (query: string) => void;
      onFocusFile: (path: string) => void;
      onToggleAll: () => void;
      onToggleWrap: () => void;
      onCopy: () => void;
      onToggleView: () => void;
      onApplyProject: () => void;
      onRemoveWorktree: () => void;
      onCommit: () => void;
      onCommitAndPush: () => void;
      onPush: () => void;
    });

function DiffCloseButton({ onClose }: { onClose?: () => void }) {
  if (!onClose) return null;
  return (
    <IconButton
      size="md"
      variant="ghost"
      onClick={onClose}
      aria-label="Close changes"
      title="Close changes (back to the conversation)"
    >
      <X size={15} />
    </IconButton>
  );
}

export function DiffToolbar(props: DiffToolbarProps) {
  const scope = (
    <DiffScopePicker
      scope={props.scope}
      triggerRef={props.scopeTriggerRef}
      onSelect={props.onScopeChange}
    />
  );
  if (props.variant === "state") {
    return (
      <div className="diffbar diffbar-state">
        {scope}
        <span className="spacer" />
        <IconButton
          size="md"
          variant="ghost"
          onClick={props.onRefresh}
          aria-label="Refresh changes"
          title="Refresh changes"
        >
          <ArrowClockwise size={15} />
        </IconButton>
        <DiffCloseButton onClose={props.onClose} />
      </div>
    );
  }
  return (
    <div className="diffbar" ref={props.barRef}>
      {!props.barTight && (
        <span className="diff-review-label inline-flex shrink-0 items-center gap-[4px] whitespace-nowrap rounded-[5px] border border-line-2 bg-panel-2 px-[7px] py-[2px] text-[12px] font-medium text-ink">
          <FileMagnifyingGlass
            size={13}
            weight="bold"
            className="shrink-0"
          />
          Review
        </span>
      )}
      {scope}
      {!props.empty && (
        <span className="diff-summary">
          <span className="add">+{props.totalAdd}</span>
          <span className="del">-{props.totalDel}</span>
        </span>
      )}
      {props.worktree && (
        <span
          className="diff-wt-badge inline-flex min-w-0 items-center gap-[4px] whitespace-nowrap text-[11px] text-ink-2 bg-panel-2 border border-line-2 rounded-[5px] px-[6px] py-[2px]"
          title={
            (props.mainRepo
              ? "Isolated worktree of " + props.mainRepo
              : "Isolated git worktree") +
            (props.branch
              ? " · branch " + props.branch
              : " · detached HEAD")
          }
        >
          <GitBranch size={12} className="shrink-0" />
          {!props.chipCompact && (
            <span className="min-w-0 truncate">
              worktree{" "}
              <span className="dim">
                · {props.branch || "detached"}
              </span>
            </span>
          )}
        </span>
      )}
      <span className="spacer" />
      <DiffMoreActionsMenu
        fileCount={props.fileCount}
        allShownOpen={props.allShownOpen}
        barTight={props.barTight}
        empty={props.empty}
        wrap={props.wrap}
        narrow={props.narrow}
        view={props.view}
        scope={props.scope}
        worktree={props.worktree}
        mainRepo={props.mainRepo}
        busy={props.busy}
        onToggleAll={props.onToggleAll}
        onToggleWrap={props.onToggleWrap}
        onCopy={props.onCopy}
        onToggleView={props.onToggleView}
        onRefresh={props.onRefresh}
        onApplyProject={props.onApplyProject}
        onRemoveWorktree={props.onRemoveWorktree}
      />
      <ChangedFilesMenu
        files={props.files}
        fileCount={props.fileCount}
        query={props.query}
        hiddenUntracked={props.hiddenUntracked}
        onQueryChange={props.onQueryChange}
        onFocusFile={props.onFocusFile}
      />
      {!props.empty && !props.barTight && (
        <IconButton
          size="md"
          variant="ghost"
          onClick={props.onCopy}
          aria-label="Copy diff"
          title="Copy the whole diff to the clipboard"
        >
          <Copy size={15} />
        </IconButton>
      )}
      {!props.empty && !props.barTight && (
        <IconButton
          size="md"
          variant="ghost"
          pressed={props.wrap}
          onClick={props.onToggleWrap}
          aria-label="Wrap long lines"
          title={props.wrap ? "Disable line wrap" : "Wrap long lines"}
        >
          {props.wrap ? (
            <TextAlignLeft size={15} />
          ) : (
            <ArrowsHorizontal size={15} />
          )}
        </IconButton>
      )}
      {!props.empty && !props.barTight && (
        <div className="diff-viewtoggle" role="group" aria-label="Diff layout">
          <button
            className={"sm icon" + (props.view === "inline" ? " sel" : "")}
            onClick={() => props.view !== "inline" && props.onToggleView()}
            title="Inline view"
            aria-label="Inline view"
            aria-pressed={props.view === "inline"}
          >
            <Rows size={14} />
          </button>
          <button
            className={"sm icon" + (props.view === "split" ? " sel" : "")}
            onClick={() => props.view !== "split" && props.onToggleView()}
            disabled={props.narrow}
            title={
              props.narrow ? "Split view needs a wider window" : "Split view"
            }
            aria-label="Split view"
            aria-pressed={props.view === "split"}
          >
            <Columns size={14} />
          </button>
        </div>
      )}
      <CommitPushMenu
        isRepo={props.isRepo}
        busy={props.busy}
        empty={props.empty}
        conflictCount={props.conflictCount}
        compact={props.barTight}
        onCommit={props.onCommit}
        onCommitAndPush={props.onCommitAndPush}
        onPush={props.onPush}
      />
      <DiffCloseButton onClose={props.onClose} />
    </div>
  );
}
