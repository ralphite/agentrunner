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
    <div className="rs-panel">
      <h2 className="rs-panel-title">Git</h2>
      <p className="rs-panel-sub">Defaults for committing and branching from a session’s workspace.</p>

      {!any && <div className="rs-noresults">No Git settings match “{query}”.</div>}

      {show("commit message template default") && (
        <section className="rs-row rs-row-block">
          <div className="rs-row-head">
            <div className="rs-row-label">
              Commit message template <span className="rs-wired">Wired</span>
            </div>
            <div className="rs-row-desc">Pre-fills the message when you commit a session’s changes from the Changes view.</div>
          </div>
          <textarea
            className="rs-textarea"
            rows={2}
            value={g.commitTemplate}
            placeholder="changes from agent session"
            onChange={(e) => patch({ commitTemplate: e.target.value })}
          />
        </section>
      )}

      {show("branch prefix") && (
        <section className="rs-row rs-row-block">
          <div className="rs-row-head">
            <div className="rs-row-label">
              Branch prefix <span className="rs-todo">Not wired yet</span>
            </div>
            <div className="rs-row-desc">Prefix for new worktree branches (e.g. <code>codex/</code>). Recorded now; the worktree flow will read it.</div>
          </div>
          <input
            className="rs-input"
            value={g.branchPrefix}
            placeholder="e.g. agent/"
            onChange={(e) => patch({ branchPrefix: e.target.value })}
          />
        </section>
      )}

      {show("pull request merge method squash") && (
        <section className="rs-row">
          <div className="rs-row-head">
            <div className="rs-row-label">
              PR merge method <span className="rs-todo">Not wired yet</span>
            </div>
            <div className="rs-row-desc">Preferred merge strategy once GitHub integration lands.</div>
          </div>
          <div className="rs-seg">
            {(["merge", "squash"] as const).map((m) => (
              <button key={m} className={"rs-seg-btn" + (g.prMergeMethod === m ? " sel" : "")} onClick={() => patch({ prMergeMethod: m })}>
                {m === "merge" ? "Merge" : "Squash"}
              </button>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
