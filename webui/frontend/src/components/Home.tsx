import { useStore } from "../store";
import { Composer } from "./Composer";
// Codex keeps task navigation in the sidebar. The landing page has one job:
// start a task without asking users to understand AgentRunner internals first.
export function Home() {
  const { toast } = useStore();

  return (
    <div className="home">
      <div className="hero">
        <div className="home-brand">AgentRunner</div>
        <h2>What should we work on?</h2>
        <Composer variant="home" onError={(m) => toast(m)} />
        <p className="hero-hint">Describe what you want done — pick a workspace, access level, and model below when the defaults don't fit.</p>
      </div>
    </div>
  );
}
