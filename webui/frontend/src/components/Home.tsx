import { useStore } from "../store";
import { Composer } from "./Composer";
import { DaemonAlert } from "./DaemonAlert";
// Codex keeps task navigation in the sidebar. The landing page has one job:
// start a task without asking users to understand AgentRunner internals first.
// It greets the way Codex does — a soft brand mark over a project-aware
// headline, with the composer centered just beneath (W1).
export function Home() {
  const { toast } = useStore();
  return (
    <div className="home home-welcome">
      <div className="hero">
        <DaemonAlert />
        <Composer variant="home" onError={(m) => toast(m)} />
      </div>
    </div>
  );
}
