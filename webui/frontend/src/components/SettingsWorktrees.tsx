import { useStore } from "../store";
import { matchesQuery } from "./SettingsSearch";

// SettingsWorktrees is Codex's Settings → Worktrees (INC-41 H5), read-only.
// We have no worktree-registry API, so the list is derived from the sessions
// we already load: each distinct workspace path becomes a card listing the
// tasks that live in it. Deleting/pruning worktrees needs a backend contract
// that doesn't exist yet, so it's called out rather than faked.
export function SettingsWorktrees({ query }: { query: string }) {
  const sessions = useStore((s) => s.sessions);
  const renames = useStore((s) => s.renames);

  const byWorkspace = new Map<string, { id: string; title: string }[]>();
  for (const s of sessions) {
    const ws = s.workspace;
    if (!ws) continue;
    const title = renames[s.id] || s.title || s.id;
    const list = byWorkspace.get(ws) || [];
    list.push({ id: s.id, title });
    byWorkspace.set(ws, list);
  }
  const all = [...byWorkspace.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  const filtered = all.filter(([ws, tasks]) => matchesQuery(query, ws + " " + tasks.map((t) => t.title).join(" ")));

  return (
    <div className="rs-panel max-w-[660px] mx-auto">
      <h2 className="rs-panel-title m-0 mb-[4px] text-[19px] font-[650]">Worktrees</h2>
      <p className="rs-panel-sub m-0 mb-[22px] text-dim text-[13px] leading-[1.5]">Workspaces backing your tasks, with the conversations linked to each. Read-only — pruning needs a daemon API that isn’t available yet.</p>

      {all.length === 0 && <div className="rs-noresults text-dim text-[13px] py-[8px]">No task workspaces yet.</div>}
      {all.length > 0 && filtered.length === 0 && <div className="rs-noresults text-dim text-[13px] py-[8px]">No worktrees match “{query}”.</div>}

      {filtered.map(([ws, tasks]) => (
        <section className="rs-wt-card mb-[12px] border border-line rounded-[12px] overflow-hidden" key={ws}>
          <div className="rs-wt-head flex items-center gap-[10px] px-[13px] py-[10px] bg-panel-2 border-b border-line">
            <span className="rs-wt-path mono flex-1 min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[12px] text-ink-2" title={ws}>
              {ws}
            </span>
            <span className="rs-wt-count shrink-0 text-[11.5px] text-dim">{tasks.length} conversation{tasks.length === 1 ? "" : "s"}</span>
          </div>
          <div className="rs-wt-tasks flex flex-col">
            {tasks.map((t) => (
              <button
                key={t.id}
                className="rs-wt-task text-left px-[13px] py-[8px] rounded-none border-x-0 border-b-0 border-t border-line-2 first:border-t-0 bg-panel text-ink text-[13px] overflow-hidden text-ellipsis whitespace-nowrap hover:bg-rs-accent-soft hover:text-rs-accent"
                onClick={() => useStore.getState().select(t.id)}
                title="Open this task"
              >
                {t.title}
              </button>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}
