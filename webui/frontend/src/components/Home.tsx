import { useState } from "react";
import { Robot } from "@phosphor-icons/react";
import { useStore } from "../store";
import { Composer } from "./Composer";
// Codex keeps task navigation in the sidebar. The landing page has one job:
// start a task without asking users to understand AgentRunner internals first.
// It greets the way Codex does — a soft brand mark over a project-aware
// headline, with the composer centered just beneath (W1).
export function Home() {
  const { toast } = useStore();
  // Mirror the composer's selected project (its `ws` is the source of truth)
  // so the headline can address it by name.
  const [project, setProject] = useState<string | null>(null);
  const headline = project ? `What should we build in ${project}?` : "What should we build?";

  return (
    <div className="home home-welcome">
      <div className="hero">
        <span className="home-hero-icon" aria-hidden="true">
          <Robot size={30} weight="regular" />
        </span>
        <h2 className="home-headline">{headline}</h2>
        <Composer variant="home" onError={(m) => toast(m)} onProjectChange={setProject} />
      </div>
    </div>
  );
}
