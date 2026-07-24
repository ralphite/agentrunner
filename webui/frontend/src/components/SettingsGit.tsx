import { useState } from "react";
import { type GitPrefs, loadGitPrefs, saveGitPrefs } from "../theme";
import { matchesQuery } from "./SettingsSearch";
import { useAppServices } from "../app/appServices";
import { Textarea } from "../ui/Field";

// SettingsGit is Codex's Settings → Git (INC-41 H4). Only settings with a
// wired effect live here: the commit-message template seeds the DiffView
// "Commit changes…" prompt. (Branch prefix / PR merge method were recorded-
// but-unconsumed placeholders and were removed — add a setting back only
// together with the flow that reads it.)
export function SettingsGit({ query }: { query: string }) {
  const { storage } = useAppServices();
  const [g, setG] = useState<GitPrefs>(() => loadGitPrefs(storage.local));
  const patch = (p: Partial<GitPrefs>) => {
    const next = { ...g, ...p };
    setG(next);
    saveGitPrefs(next, storage.local);
  };

  const show = (s: string) => matchesQuery(query, s);
  const any = show("commit message template default");

  return (
    <div className="rs-panel min-w-0">
      <h2 className="rs-panel-title">Git</h2>
      <p className="rs-panel-sub leading-[1.5]">Defaults for committing from a session’s workspace.</p>

      {!any && <div className="rs-noresults">No Git settings match “{query}”.</div>}

      {show("commit message template default") && (
        <section className="rs-row rs-row-block min-w-0 max-[500px]:rounded-[8px] max-[500px]:p-2.5">
          <div className="rs-row-head min-w-0 max-[500px]:flex-col max-[500px]:items-stretch max-[500px]:gap-1">
            <div className="rs-row-label flex min-w-0 flex-wrap items-center gap-1.5">
              <span>Commit message template</span>
            </div>
            <div className="rs-row-desc max-w-[430px] min-w-0 leading-[1.5] max-[500px]:max-w-none">Pre-fills commit messages in Changes.</div>
          </div>
          <Textarea
            className="rs-textarea mt-3 min-w-0 max-w-full resize-y"
            aria-label="Commit message template"
            rows={2}
            value={g.commitTemplate}
            placeholder="changes from agent session"
            onChange={(e) => patch({ commitTemplate: e.target.value })}
          />
        </section>
      )}
    </div>
  );
}
