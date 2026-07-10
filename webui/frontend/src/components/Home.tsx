import { useStore } from "../store";
import { Composer } from "./Composer";
// Codex keeps task navigation in the sidebar. The landing page has one job:
// start a task without asking users to understand AgentRunner internals first.
export function Home() {
  const { toast } = useStore();

  return (
    <div className="home">
      <div className="hero">
        <Composer variant="home" onError={(m) => toast(m)} />
      </div>
    </div>
  );
}
