import { useState } from "react";
import { type GitPrefs, loadGitPrefs, saveGitPrefs } from "../theme";
import { matchesQuery } from "./SettingsSearch";
import { SEG_BTN } from "./SettingsAppearance";

// SettingsGit is Codex's Settings → Git (INC-41 H4). Only the commit-message
// template has a wired effect today — it seeds the DiffView "Commit changes…"
// prompt. Branch prefix and PR merge method are recorded but not yet consumed
// (the worktree/PR flows that would read them live outside this slice), so they
// carry an honest "Not wired yet" note rather than pretending to do something.
const ROW = "rs-row flex items-center justify-between gap-[22px] py-[16px] border-t border-line-2 first-of-type:border-t-0";
const ROW_BLOCK = "rs-row rs-row-block flex flex-col items-stretch justify-between gap-[12px] py-[16px] border-t border-line-2 first-of-type:border-t-0";
const FIELD =
  "w-full border border-line rounded-[9px] bg-panel text-ink px-[11px] py-[9px] text-[13px] font-sans focus:outline-none focus:border-rs-accent";
export const TODO_BADGE =
  "rs-todo text-[10px] font-semibold uppercase tracking-[0.4px] px-[7px] py-[2px] rounded-full text-dim bg-panel-2 border border-line";

export function SettingsGit({ query }: { query: string }) {
  const [g, setG] = useState<GitPrefs>(loadGitPrefs);
  const patch = (p: Partial<GitPrefs>) => {
    const next = { ...g, ...p };
    setG(next);
    saveGitPrefs(next);
  };

  const show = (s: string) => matchesQuery(query, s);
  const any = show("commit message template default") || show("branch prefix") || show("pull request merge method squash");

  return (
    <div className="rs-panel max-w-[660px] mx-auto">
      <h2 className="rs-panel-title m-0 mb-[4px] text-[19px] font-[650]">Git</h2>
      <p className="rs-panel-sub m-0 mb-[22px] text-dim text-[13px] leading-[1.5]">Defaults for committing and branching from a task’s workspace.</p>

      {!any && <div className="rs-noresults text-dim text-[13px] py-[8px]">No Git settings match “{query}”.</div>}

      {show("commit message template default") && (
        <section className={ROW_BLOCK}>
          <div className="rs-row-head min-w-0">
            <div className="rs-row-label flex items-center gap-[8px] text-[14px] text-ink">
              Commit message template{" "}
              <span className="rs-wired text-[10px] font-semibold uppercase tracking-[0.4px] px-[7px] py-[2px] rounded-full text-green bg-green-soft">
                Wired
              </span>
            </div>
            <div className="rs-row-desc mt-[3px] text-[12.5px] text-dim leading-[1.5]">Pre-fills the message when you commit a task’s changes from the Changes view.</div>
          </div>
          <textarea
            className={"rs-textarea resize-y leading-[1.5] " + FIELD}
            rows={2}
            value={g.commitTemplate}
            placeholder="changes from agent session"
            onChange={(e) => patch({ commitTemplate: e.target.value })}
          />
        </section>
      )}

      {show("branch prefix") && (
        <section className={ROW_BLOCK}>
          <div className="rs-row-head min-w-0">
            <div className="rs-row-label flex items-center gap-[8px] text-[14px] text-ink">
              Branch prefix <span className={TODO_BADGE}>Not wired yet</span>
            </div>
            <div className="rs-row-desc mt-[3px] text-[12.5px] text-dim leading-[1.5]">
              Prefix for new worktree branches (e.g. <code className="font-mono text-[11.5px] bg-panel-2 px-[5px] py-[1px] rounded-[5px]">codex/</code>).
              Recorded now; the worktree flow will read it.
            </div>
          </div>
          <input
            className={"rs-input " + FIELD}
            value={g.branchPrefix}
            placeholder="e.g. agent/"
            onChange={(e) => patch({ branchPrefix: e.target.value })}
          />
        </section>
      )}

      {show("pull request merge method squash") && (
        <section className={ROW}>
          <div className="rs-row-head min-w-0">
            <div className="rs-row-label flex items-center gap-[8px] text-[14px] text-ink">
              PR merge method <span className={TODO_BADGE}>Not wired yet</span>
            </div>
            <div className="rs-row-desc mt-[3px] text-[12.5px] text-dim leading-[1.5]">Preferred merge strategy once GitHub integration lands.</div>
          </div>
          <div className="rs-seg inline-flex shrink-0 border border-line rounded-[9px] overflow-hidden">
            {(["merge", "squash"] as const).map((m) => (
              <button
                key={m}
                className={SEG_BTN + (g.prMergeMethod === m ? " sel bg-rs-accent-soft text-rs-accent font-[550]" : " bg-panel text-ink-2")}
                onClick={() => patch({ prMergeMethod: m })}
              >
                {m === "merge" ? "Merge" : "Squash"}
              </button>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
