import "../styles.home.css";
import { ChatCircleDots } from "@phosphor-icons/react";
import { useStore } from "../store";
import { Composer } from "./Composer";
import { DaemonAlert } from "./DaemonAlert";

// Chat is Codex's zero-config chat entry — the fastest way to start talking to
// the agent without first picking a project or thinking about background work.
// It reuses the existing Home composer (variant="home") verbatim, so a message
// here spins up a normal interactive session exactly like the New-task page.
//
// Deferred (Composer follow-up): Codex's Chat hides the project / worktree /
// environment chips for a truly zero-config feel. Suppressing them would need a
// new Composer prop (e.g. `chrome="minimal"`); rather than edit Composer here we
// ship it reusing the composer as-is and leave the chip-hiding to that change.
export function Chat() {
  const { toast } = useStore();
  return (
    <div className="home home-welcome home-empty-state">
      <div className="hero">
        <div className="home-empty">
          <div className="home-hero-icon" aria-hidden>
            <ChatCircleDots size={28} />
          </div>
          <h2 className="home-empty-headline">What's on your mind?</h2>
        </div>
        <DaemonAlert />
        <Composer variant="home" onError={(m) => toast(m)} />
      </div>
    </div>
  );
}
