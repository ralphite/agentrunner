import "../styles.nav.css";
import { MagnifyingGlass, Plus, SquaresFour } from "@phosphor-icons/react";
import { useStore } from "../store";

// Sites is Codex's "turn your ideas into live websites" surface. In Codex today
// it is literally an empty state — there is no sites backend yet — so we ship
// that faithfully: the heading, a (disabled) search, and a centered "No sites
// yet" card with a Create-new-site call to action.
//
// Deferred backend: AgentRunner has no site publishing/hosting endpoint. Both
// Create actions surface an honest "coming soon" toast rather than pretending
// to create something. Wire them to a real endpoint when one exists.
export function Sites() {
  const { toast } = useStore();
  const notReady = () => toast("Sites aren't available yet — this surface is a placeholder.", "info");

  return (
    <div className="scheduled-page sites-page">
      <div className="page-heading">
        <div>
          <span className="page-eyebrow"><SquaresFour size={16} /> Sites</span>
          <h2>Sites</h2>
          <p>Turn your ideas into live websites.</p>
        </div>
        <button className="page-action nav-primary-btn" onClick={notReady}>
          <Plus size={15} /> Create
        </button>
      </div>

      <div className="nav-toolbar">
        <div className="sched-search">
          <MagnifyingGlass size={15} />
          <input placeholder="Search sites…" aria-label="Search sites" disabled />
        </div>
      </div>

      <div className="empty-state nav-empty">
        <SquaresFour size={28} />
        <b>No sites yet</b>
        <span>Publish a project to the web and it will show up here.</span>
        <button className="nav-empty-action" onClick={notReady}>
          <Plus size={14} /> Create new site
        </button>
      </div>
    </div>
  );
}
