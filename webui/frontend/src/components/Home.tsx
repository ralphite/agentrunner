import "../styles.home.css";
import { useMemo, useState } from "react";
import { ArrowsClockwise, Binoculars, Bug, Hammer, Terminal } from "@phosphor-icons/react";
import { useStore } from "../store";
import { projectLabel } from "../viewModels";
import { Composer } from "./Composer";
import { DaemonAlert } from "./DaemonAlert";

// Codex keeps task navigation in the sidebar. The landing page has one job:
// start a task without asking users to understand AgentRunner internals first.
// It greets the way Codex does — a soft brand mark over a project-aware
// headline, four suggestion cards, and the composer pinned just beneath (W1).

// One suggestion card: a colored line icon, a short label, and the starter
// prompt it drops into the composer draft when clicked.
interface Suggestion {
  key: string;
  tone: "blue" | "violet" | "green" | "amber";
  icon: React.ReactNode;
  label: string;
  prompt: string;
}

const SUGGESTIONS: Suggestion[] = [
  {
    key: "explore",
    tone: "blue",
    icon: <Binoculars size={22} />,
    label: "Explore and understand code",
    prompt:
      "Explore this codebase and help me understand it. Walk me through the overall structure, the main components, and how they fit together.",
  },
  {
    key: "build",
    tone: "violet",
    icon: <Hammer size={22} />,
    label: "Build a new feature, app, or tool",
    prompt: "Help me build a new feature. Let's scope out what it should do, then implement it step by step.",
  },
  {
    key: "review",
    tone: "green",
    icon: <ArrowsClockwise size={22} />,
    label: "Review code and suggest changes",
    prompt: "Review the recent changes in this project and suggest improvements to correctness, clarity, and structure.",
  },
  {
    key: "fix",
    tone: "amber",
    icon: <Bug size={22} />,
    label: "Fix issues and failures",
    prompt: "Investigate the failing tests and errors in this project, find the root cause, and fix them.",
  },
];

// Drop a starter prompt into the (self-contained) home composer draft and focus
// the input. The composer owns its draft state, so we set the textarea's value
// through React's native input path — the dispatched `input` event runs the
// composer's onChange, which updates its draft + auto-grows the field — instead
// of remounting the composer and losing the user's project/model choices.
function prefillComposer(prompt: string) {
  const ta = document.querySelector<HTMLTextAreaElement>(".home-empty-state .cx-home textarea");
  if (!ta) return;
  const setValue = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
  setValue?.call(ta, prompt);
  ta.dispatchEvent(new Event("input", { bubbles: true }));
  ta.focus();
  const end = ta.value.length;
  ta.setSelectionRange(end, end);
}

export function Home() {
  const { toast } = useStore();
  const sessions = useStore((s) => s.sessions);

  // Headline workspace name: the composer's selected project when one is chosen
  // (reported up via onProjectChange), else the most recent real project from
  // history, else a friendly default — so the greeting is never blank.
  const [selectedProject, setSelectedProject] = useState<string | null>(null);
  const fallbackProject = useMemo(() => {
    for (const s of [...sessions].sort((a, b) => b.id.localeCompare(a.id))) {
      const label = projectLabel(s.workspace);
      if (label && label !== "Scratch" && label !== "Other sessions") return label;
    }
    return "your project";
  }, [sessions]);
  const project = selectedProject || fallbackProject;

  return (
    <div className="home home-welcome home-empty-state">
      <div className="hero">
        <div className="home-empty">
          <div className="home-hero-icon" aria-hidden>
            <Terminal size={28} />
          </div>
          <h2 className="home-empty-headline">
            What should we build in <span className="home-empty-repo">{project}</span>?
          </h2>
          <div className="home-empty-cards">
            {SUGGESTIONS.map((s) => (
              <button key={s.key} type="button" className="home-empty-card" onClick={() => prefillComposer(s.prompt)}>
                <span className={"home-empty-card-icon " + s.tone} aria-hidden>
                  {s.icon}
                </span>
                <span className="home-empty-card-label">{s.label}</span>
              </button>
            ))}
          </div>
        </div>
        <DaemonAlert />
        <Composer variant="home" onError={(m) => toast(m)} onProjectChange={setSelectedProject} />
      </div>
    </div>
  );
}
