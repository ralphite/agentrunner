import { useState } from "react";
import { type GitPrefs, loadGitPrefs, saveGitPrefs } from "../theme";
import { matchesQuery } from "./SettingsSearch";

// SettingsGit is Codex's Settings → Git (INC-41 H4). Only the commit-message
// template has a wired effect today — it seeds the DiffView "Commit changes…"
// prompt. Branch prefix and PR merge method are recorded but not yet consumed
// (the worktree/PR flows that would read them live outside this slice), so they
// carry an honest "Not wired yet" note rather than pretending to do something.
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
    <div className="rs-panel min-w-0">
      <h2 className="rs-panel-title">Git</h2>
      <p className="rs-panel-sub leading-[1.5]">Defaults for committing and branching from a session’s workspace.</p>

      {!any && <div className="rs-noresults">No Git settings match “{query}”.</div>}

      {show("commit message template default") && (
        <section className="rs-row rs-row-block min-w-0 max-[500px]:rounded-[8px] max-[500px]:p-2.5">
          <div className="rs-row-head min-w-0 max-[500px]:flex-col max-[500px]:items-stretch max-[500px]:gap-1">
            <div className="rs-row-label flex min-w-0 flex-wrap items-center gap-1.5">
              <span>Commit message template</span>
              <StatusBadge wired />
            </div>
            <div className="rs-row-desc max-w-[430px] min-w-0 leading-[1.5] max-[500px]:max-w-none">Pre-fills commit messages in Changes.</div>
          </div>
          <textarea
            className="rs-textarea mt-3 min-w-0 max-w-full resize-y"
            rows={2}
            value={g.commitTemplate}
            placeholder="changes from agent session"
            onChange={(e) => patch({ commitTemplate: e.target.value })}
          />
        </section>
      )}

      {show("branch prefix") && (
        <section className="rs-row rs-row-block min-w-0 max-[500px]:rounded-[8px] max-[500px]:p-2.5">
          <div className="rs-row-head min-w-0 max-[500px]:flex-col max-[500px]:items-stretch max-[500px]:gap-1">
            <div className="rs-row-label flex min-w-0 flex-wrap items-center gap-1.5">
              <span>Branch prefix</span>
              <StatusBadge />
            </div>
            <div className="rs-row-desc max-w-[430px] min-w-0 leading-[1.5] max-[500px]:max-w-none">Saved for future worktree branches (for example, <code>codex/</code>).</div>
          </div>
          <input
            className="rs-input mt-3 min-w-0 max-w-full"
            value={g.branchPrefix}
            placeholder="e.g. agent/"
            onChange={(e) => patch({ branchPrefix: e.target.value })}
          />
        </section>
      )}

      {show("pull request merge method squash") && (
        <section className="rs-row min-w-0 max-[500px]:rounded-[8px] max-[500px]:p-2.5">
          <div className="rs-row-head min-w-0 max-[500px]:flex-col max-[500px]:items-stretch max-[500px]:gap-1">
            <div className="rs-row-label flex min-w-0 flex-wrap items-center gap-1.5">
              <span>PR merge method</span>
              <StatusBadge />
            </div>
            <div className="rs-row-desc max-w-[430px] min-w-0 leading-[1.5] max-[500px]:max-w-none">Saved for the future GitHub integration.</div>
          </div>
          <div className="rs-seg mt-3 max-[500px]:flex max-[500px]:w-full">
            {(["merge", "squash"] as const).map((m) => (
              <button key={m} className={"rs-seg-btn max-[500px]:min-w-0 max-[500px]:flex-1" + (g.prMergeMethod === m ? " on" : "")} onClick={() => patch({ prMergeMethod: m })}>
                {m === "merge" ? "Merge" : "Squash"}
              </button>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}

function StatusBadge({ wired = false }: { wired?: boolean }) {
  return (
    <span
      className={
        "shrink-0 whitespace-nowrap rounded-full border px-1.5 py-0.5 text-[10px] font-normal leading-none " +
        (wired ? "border-green/30 bg-green/10 text-green" : "border-line bg-panel-2 text-dim")
      }
    >
      {wired ? "Wired" : "Not wired"}
    </span>
  );
}
