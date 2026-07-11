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
    <div className="rs-panel">
      <h2 className="rs-panel-title">Worktrees</h2>
      <p className="rs-panel-sub">Workspaces backing your tasks, with the conversations linked to each. Read-only — pruning needs a daemon API that isn’t available yet.</p>

      {all.length === 0 && <div className="rs-noresults">No task workspaces yet.</div>}
      {all.length > 0 && filtered.length === 0 && <div className="rs-noresults">No worktrees match “{query}”.</div>}

      {filtered.map(([ws, tasks]) => (
        <section className="rs-wt-card" key={ws}>
          <div className="rs-wt-head">
            <span className="rs-wt-path mono" title={ws}>
              {ws}
            </span>
            <span className="rs-wt-count">{tasks.length} conversation{tasks.length === 1 ? "" : "s"}</span>
          </div>
          <div className="rs-wt-tasks">
            {tasks.map((t) => (
              <button
                key={t.id}
                className="rs-wt-task"
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
