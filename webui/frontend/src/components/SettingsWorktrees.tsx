import { useEffect, useState } from "react";
import { useAppStoreApi, useStore } from "../store";
import { matchesQuery } from "./SettingsSearch";
import { WorktreeCard } from "./WorktreeCard";

const PAGE_SIZE = 40;

// SettingsWorktrees is Codex's Settings → Worktrees (INC-41 H5), read-only.
// We have no worktree-registry API, so the list is derived from the sessions
// we already load: each distinct workspace path becomes a card listing the
// sessions that live in it. Deleting/pruning worktrees needs a backend contract
// that doesn't exist yet, so it's called out rather than faked.
export function SettingsWorktrees({ query }: { query: string }) {
  const store = useAppStoreApi();
  const sessions = useStore((s) => s.sessions);
  const renames = useStore((s) => s.renames);
  const [visibleCount, setVisibleCount] = useState(PAGE_SIZE);

  useEffect(() => setVisibleCount(PAGE_SIZE), [query]);

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
  const filtered = all.filter(([ws, sessions]) => matchesQuery(query, ws + " " + sessions.map((t) => t.title).join(" ")));
  const visible = filtered.slice(0, visibleCount);

  return (
    <div className="rs-panel min-w-0">
      <h2 className="rs-panel-title">Worktrees</h2>
      <p className="rs-panel-sub break-words">Workspaces backing your sessions, with the conversations linked to each. Read-only — pruning needs a daemon API that isn’t available yet.</p>

      {all.length === 0 && <div className="rs-noresults">No session workspaces yet.</div>}
      {all.length > 0 && filtered.length === 0 && <div className="rs-noresults">No worktrees match “{query}”.</div>}

      {visible.map(([ws, sessions]) => (
        <WorktreeCard
          key={ws}
          workspace={ws}
          sessions={sessions}
          onOpenSession={(sessionId) => store.getState().select(sessionId)}
        />
      ))}
      {visible.length < filtered.length && (
        <button
          className="mt-3 w-full rounded-[8px] border border-line bg-panel px-3 py-2 text-[12.5px] text-ink-2 hover:bg-panel-2"
          onClick={() => setVisibleCount((count) => count + PAGE_SIZE)}
        >
          Show {Math.min(PAGE_SIZE, filtered.length - visible.length)} more · {filtered.length - visible.length} remaining
        </button>
      )}
    </div>
  );
}
