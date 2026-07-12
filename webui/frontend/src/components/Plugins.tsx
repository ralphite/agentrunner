import "../styles.nav.css";
import { useMemo, useState } from "react";
import type { Icon } from "@phosphor-icons/react";
import {
  Browser,
  ChartBar,
  ChatCircleDots,
  Cube,
  Database,
  FileText,
  GitBranch,
  MagnifyingGlass,
  Plugs,
  PuzzlePiece,
  Table,
} from "@phosphor-icons/react";
import { useStore } from "../store";

// Plugins is Codex's "work with ChatGPT across your favorite tools" registry:
// a search, an Installed shelf, and a Featured grid of installable connectors.
//
// Deferred backend: AgentRunner has no plugin / MCP registry endpoint yet, so
// this is a faithful shell — the layout, search, Installed empty state and a
// Featured grid are all real, but the catalog is static and Install surfaces an
// honest "coming soon" toast. Swap FEATURED for a real registry fetch (and wire
// Install to it) once the backend lands.
interface FeaturedPlugin {
  key: string;
  name: string;
  desc: string;
  icon: Icon;
  tone: string;
}

const FEATURED: FeaturedPlugin[] = [
  { key: "browser", name: "Browser", desc: "Let the agent read and act on live web pages", icon: Browser, tone: "#3b82f6" },
  { key: "spreadsheets", name: "Spreadsheets", desc: "Create and edit spreadsheets", icon: Table, tone: "#22c55e" },
  { key: "github", name: "GitHub", desc: "Triage PRs, issues, CI, and publish work", icon: GitBranch, tone: "#8b5cf6" },
  { key: "docs", name: "Documents", desc: "Draft and revise long-form documents", icon: FileText, tone: "#f59e0b" },
  { key: "analytics", name: "Data Analytics", desc: "Answer product and business questions", icon: ChartBar, tone: "#06b6d4" },
  { key: "database", name: "Databases", desc: "Query and manage your databases", icon: Database, tone: "#ef4444" },
  { key: "chat", name: "Team Chat", desc: "Read and post to your team channels", icon: ChatCircleDots, tone: "#ec4899" },
  { key: "sandbox", name: "Code Sandbox", desc: "Run isolated code in a scratch environment", icon: Cube, tone: "#64748b" },
];

export function Plugins() {
  const { toast } = useStore();
  const [query, setQuery] = useState("");
  const notReady = (name?: string) =>
    toast(name ? `Installing "${name}" isn't available yet — the plugin registry is coming soon.` : "The plugin registry is coming soon.", "info");

  const ql = query.trim().toLowerCase();
  const featured = useMemo(
    () => FEATURED.filter((p) => !ql || p.name.toLowerCase().includes(ql) || p.desc.toLowerCase().includes(ql)),
    [ql],
  );

  return (
    <div className="scheduled-page plugins-page">
      <div className="page-heading">
        <div>
          <span className="page-eyebrow"><Plugs size={16} /> Plugins</span>
          <h2>Plugins</h2>
          <p>Work with AgentRunner across your favorite tools.</p>
        </div>
        <button className="page-action nav-primary-btn" onClick={() => notReady()}>
          <PuzzlePiece size={15} /> Create
        </button>
      </div>

      <div className="nav-toolbar">
        <div className="sched-search">
          <MagnifyingGlass size={15} />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search plugins…"
            aria-label="Search plugins"
          />
        </div>
      </div>

      <div className="plugins-body">
        <section className="plugins-section">
          <div className="plugins-section-title">Installed</div>
          <div className="plugins-installed-empty">
            <PuzzlePiece size={20} />
            <span>No plugins installed yet. Browse the featured tools below to get started.</span>
          </div>
        </section>

        <section className="plugins-section">
          <div className="plugins-section-title">Featured</div>
          {featured.length === 0 ? (
            <div className="plugins-installed-empty">
              <MagnifyingGlass size={20} />
              <span>No plugins match "{query.trim()}".</span>
            </div>
          ) : (
            <div className="plugin-grid">
              {featured.map((p) => {
                const Ic = p.icon;
                return (
                  <div className="plugin-card" key={p.key}>
                    <span className="plugin-card-icon" style={{ color: p.tone }} aria-hidden>
                      <Ic size={22} />
                    </span>
                    <span className="plugin-card-body">
                      <b className="plugin-card-name">{p.name}</b>
                      <span className="plugin-card-desc">{p.desc}</span>
                    </span>
                    <button className="plugin-install" onClick={() => notReady(p.name)}>Install</button>
                  </div>
                );
              })}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
