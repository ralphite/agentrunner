import "../styles.nav.css";
import { useEffect, useMemo, useState } from "react";
import { ArrowUpRight, GitBranch, GitPullRequest, Info, MagnifyingGlass } from "@phosphor-icons/react";
import { AR } from "../api";
import { useStore } from "../store";
import { summarizeChanges } from "../diffSummary";
import { projectLabel } from "../viewModels";

type Filter = "all" | "reviewing" | "authored";

// Pull requests is Codex's "review and track work across GitHub" surface. We
// have no remote-PR/GitHub integration, so instead of faking remote PRs this
// page aggregates the honest local truth the app already holds: one row per
// real git workspace, showing its current branch, its net +/- change counts
// (from the newest session's working-tree diff), and its uncommitted-file
// count. Clicking a row opens that session so you can review the actual diff.
//
// Deferred backend: real remote pull requests (open PRs, review state, CI,
// authored-vs-assigned) need a GitHub integration endpoint. Until it lands the
// All / Reviewing / Authored filters partition the local rows honestly —
// "Authored" = workspaces with your uncommitted work, "Reviewing" = clean ones.
interface RepoInfo {
  loaded: boolean;
  isRepo: boolean;
  branch: string;
  dirty: number;
  add: number;
  del: number;
}

interface PullRow {
  workspace: string;
  sid: string;
  repo: string;
  branch: string;
  dirty: number;
  add: number;
  del: number;
  authored: boolean;
}

export function PullRequests() {
  const sessions = useStore((s) => s.sessions);
  const select = useStore((s) => s.select);
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const [info, setInfo] = useState<Record<string, RepoInfo>>({});

  // Unique real (absolute-path) workspaces, each paired with its newest session
  // as the representative to open and to read a diff from. Session ids sort as
  // creation stamps, so a descending id sort surfaces the newest first.
  const repos = useMemo(() => {
    const seen = new Map<string, string>();
    for (const s of [...sessions].sort((a, b) => b.id.localeCompare(a.id))) {
      const w = (s.workspace || "").trim().replace(/\/+$/, "");
      if (!w || !w.startsWith("/") || seen.has(w)) continue;
      seen.set(w, s.id);
    }
    return [...seen.entries()].map(([workspace, sid]) => ({ workspace, sid }));
  }, [sessions]);

  const reposKey = repos.map((r) => r.workspace).join("|");
  useEffect(() => {
    let alive = true;
    for (const { workspace, sid } of repos) {
      if (info[workspace]?.loaded) continue;
      Promise.all([
        AR.gitBranches(workspace).catch(() => null),
        AR.diff(sid).then(summarizeChanges).catch(() => null),
      ]).then(([b, sum]) => {
        if (!alive) return;
        setInfo((cur) => ({
          ...cur,
          [workspace]: {
            loaded: true,
            isRepo: b?.isRepo ?? false,
            branch: b && b.isRepo ? (b.current === "HEAD" || !b.current ? "detached HEAD" : b.current) : "",
            dirty: b?.dirty ?? 0,
            add: sum?.totalAdd ?? 0,
            del: sum?.totalDel ?? 0,
          },
        }));
      });
    }
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reposKey]);

  const loading = repos.some((r) => !info[r.workspace]?.loaded);

  const rows = useMemo<PullRow[]>(() => {
    const out: PullRow[] = [];
    for (const { workspace, sid } of repos) {
      const meta = info[workspace];
      if (!meta?.loaded || !meta.isRepo) continue;
      out.push({
        workspace,
        sid,
        repo: projectLabel(workspace),
        branch: meta.branch,
        dirty: meta.dirty,
        add: meta.add,
        del: meta.del,
        authored: meta.dirty > 0 || meta.add > 0 || meta.del > 0,
      });
    }
    return out;
  }, [repos, info]);

  const counts = {
    all: rows.length,
    reviewing: rows.filter((r) => !r.authored).length,
    authored: rows.filter((r) => r.authored).length,
  };
  const ql = query.trim().toLowerCase();
  const filtered = rows.filter((r) => {
    if (filter === "authored" && !r.authored) return false;
    if (filter === "reviewing" && r.authored) return false;
    if (ql && !(r.repo.toLowerCase().includes(ql) || r.branch.toLowerCase().includes(ql))) return false;
    return true;
  });

  return (
    <div className="scheduled-page pulls-page">
      <div className="page-heading">
        <div>
          <span className="page-eyebrow"><GitPullRequest size={16} /> Pull requests</span>
          <h2>Pull requests</h2>
          <p>Review and track work across your git workspaces.</p>
        </div>
      </div>

      <div className="nav-toolbar">
        <div className="sched-search">
          <MagnifyingGlass size={15} />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search pull requests…"
            aria-label="Search pull requests"
          />
        </div>
        <div className="sched-tabs" role="tablist" aria-label="Filter pull requests">
          {(["all", "reviewing", "authored"] as Filter[]).map((f) => (
            <button
              key={f}
              role="tab"
              aria-selected={filter === f}
              className={"sched-tab" + (filter === f ? " on" : "")}
              onClick={() => setFilter(f)}
            >
              {f[0].toUpperCase() + f.slice(1)}
              <span className="sched-tab-count">{counts[f]}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="pulls-note">
        <Info size={14} />
        <span>Showing local git workspaces. Remote pull requests (GitHub open PRs, review state, CI) are a deferred integration.</span>
      </div>

      <div className="scheduled-list pulls-list">
        {loading && rows.length === 0 ? (
          <div className="empty-state">
            <GitPullRequest size={28} />
            <b>Loading workspaces…</b>
            <span>Reading branch and change status from your git workspaces.</span>
          </div>
        ) : filtered.length === 0 ? (
          <div className="empty-state">
            <GitPullRequest size={28} />
            <b>{rows.length === 0 ? "No git workspaces" : "Nothing here"}</b>
            <span>
              {rows.length === 0
                ? "Start a task in a git repository and its branch will show up here."
                : ql
                  ? `No results for "${query.trim()}".`
                  : filter === "authored"
                    ? "No workspaces have uncommitted work."
                    : "Every workspace has uncommitted work right now."}
            </span>
          </div>
        ) : (
          filtered.map((r) => (
            <button className="scheduled-row pull-row" key={r.workspace} onClick={() => select(r.sid)}>
              <span className="pull-ico" aria-hidden><GitBranch size={16} /></span>
              <span className="scheduled-copy pull-copy">
                <b className="pull-repo">{r.repo}</b>
                <span className="pull-branch">{r.branch || "no branch"}{r.dirty > 0 ? ` · ${r.dirty} uncommitted` : ""}</span>
              </span>
              <span className="pull-counts" aria-label={`${r.add} added, ${r.del} deleted`}>
                <span className="add">+{r.add}</span>
                <span className="del">-{r.del}</span>
              </span>
              <ArrowUpRight size={15} />
            </button>
          ))
        )}
      </div>
    </div>
  );
}
