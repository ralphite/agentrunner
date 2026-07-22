import { useState } from "react";
import { ArrowsClockwise, Bug, Hammer } from "@phosphor-icons/react";
import { useStore } from "../store";
import { Composer } from "./Composer";
import { DaemonAlert } from "./DaemonAlert";

// Codex's Explore card uses a telescope, not binoculars — but Phosphor (our
// icon set) has no telescope glyph, so we inline the line-drawn telescope path
// (viewBox 0 0 24 24, currentColor stroke) to match the reference exactly. It
// takes the same `size` prop shape as the Phosphor icons around it.
function Telescope({ size = 24 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      focusable="false"
    >
      <path d="m10.065 12.493-6.18 1.318a.934.934 0 0 1-1.108-.702l-.537-2.15a1.07 1.07 0 0 1 .691-1.265l13.504-4.44" />
      <path d="m13.56 11.747 4.332-.924" />
      <path d="m16 21-3.105-6.21" />
      <path d="M16.485 5.94a2 2 0 0 1 1.455-2.425l1.09-.272a1 1 0 0 1 1.212.727l1.515 6.06a1 1 0 0 1-.727 1.213l-1.09.272a2 2 0 0 1-2.425-1.455z" />
      <path d="m6.158 8.633 1.114 4.456" />
      <path d="m8 21 3.105-6.21" />
      <circle cx="12" cy="13" r="2" />
    </svg>
  );
}

// The empty state's brand mark. Codex draws a lobed cloud outline with a `>_`
// terminal prompt tucked inside — a *shape*, the one graphic anchor on an
// otherwise blank page. We used to render a bare Phosphor <Terminal/> glyph in a
// transparent box, which left a stray `>_` floating mid-page like a typo (HM-3).
// Phosphor has no cloud-with-prompt glyph, so we inline it: seven outward arcs
// around a r=8 circle make the lobes, then the chevron and underscore sit inside
// (viewBox 0 0 24 24, currentColor stroke — so .home-hero-icon keeps owning the
// size and the quiet watermark gray).
function CloudMark({ size = 24 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      focusable="false"
    >
      <path d="M12 3a4.5 4.5 0 0 1 7.036 3.389a4.5 4.5 0 0 1 1.738 7.614a4.5 4.5 0 0 1-4.869 6.106a4.5 4.5 0 0 1-7.81 0a4.5 4.5 0 0 1-4.869-6.106a4.5 4.5 0 0 1 1.738-7.614a4.5 4.5 0 0 1 7.036-3.389z" />
      <path d="M7.1 9.1 10.2 12.2 7.1 15" />
      <path d="M12.7 15h4.2" />
    </svg>
  );
}

// Codex keeps session navigation in the sidebar. The landing page has one job:
// start a session without asking users to understand AgentRunner internals first.
// It greets the way Codex does — a soft brand mark over a project-aware
// headline, four suggestion cards, and the composer pinned just beneath (W1).

// One suggestion card: a colored line icon, a short label, and the starter
// prompt it drops into the composer draft when clicked.
interface Suggestion {
  key: string;
  tone: "blue" | "teal" | "violet" | "green" | "orange";
  icon: React.ReactNode;
  label: string;
  prompt: string;
}

const SUGGESTIONS: Suggestion[] = [
  {
    key: "explore",
    tone: "blue",
    icon: <Telescope size={16} />,
    label: "Explore and understand code",
    prompt:
      "Explore this codebase and help me understand it. Walk me through the overall structure, the main components, and how they fit together.",
  },
  {
    key: "build",
    tone: "violet",
    icon: <Hammer size={16} />,
    label: "Build a new feature, app, or tool",
    prompt: "Help me build a new feature. Let's scope out what it should do, then implement it step by step.",
  },
  {
    key: "review",
    tone: "green",
    icon: <ArrowsClockwise size={16} />,
    label: "Review code and suggest changes",
    prompt: "Review the recent changes in this project and suggest improvements to correctness, clarity, and structure.",
  },
  {
    key: "fix",
    tone: "orange",
    icon: <Bug size={16} />,
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
  const { toast, newSessionProject } = useStore();

  // The headline names EXACTLY what the composer's project chip has selected —
  // one source of truth, reported up from the composer's `ws` via
  // onProjectChange (RH-1). It used to guess from session history instead, which
  // let the greeting say "build in cx3-ws" while the chip said "Select project"
  // and the session actually ran in a scratch dir. With no project selected the
  // greeting drops the name rather than inventing one (Codex parity).
  const [project, setProject] = useState<string | null>(null);

  return (
    <div className="home home-welcome home-empty-state">
      <div className="hero max-[680px]:[@media(max-height:560px)]:py-2">
        {/* Codex's landing is a pinned-composer chat layout: the hero (mark +
            headline + cards) sits centered-ish in the upper space while the
            composer docks to the bottom of the viewport, with clear whitespace
            between. This flex-1 wrapper claims the vertical slack and centers
            the hero within it, pushing the composer — the .hero's last child —
            to the bottom on both desktop and mobile (HOME-COMPOSER-DOCK). */}
        <div className="flex w-full min-h-0 flex-1 flex-col items-center justify-center gap-5">
          <div className="home-empty">
            <div className="home-hero-icon" aria-hidden>
              <CloudMark size={35} />
            </div>
            <h2 className="home-empty-headline">
              {project ? (
                <>
                  What should we build in <span className="home-empty-repo underline decoration-dotted decoration-dim underline-offset-4">{project}</span>?
                </>
              ) : (
                <>What should we build?</>
              )}
            </h2>
            <div className="home-empty-cards max-[680px]:gap-1.5">
              {SUGGESTIONS.map((s) => (
                <button
                  key={s.key}
                  type="button"
                  className="home-empty-card max-[680px]:min-h-[76px] max-[680px]:gap-1 max-[680px]:px-2.5 max-[680px]:py-2"
                  onClick={() => prefillComposer(s.prompt)}
                >
                  <span className={"home-empty-card-icon " + s.tone} aria-hidden>
                    {s.icon}
                  </span>
                  <span className="home-empty-card-label">{s.label}</span>
                </button>
              ))}
            </div>
          </div>
          <DaemonAlert />
        </div>
        {/* Codex's compact composer keeps the primary mobile action row to
            add/access/model/mic/send. Once a starter fills our draft, the
            desktop-only optimize shortcut otherwise pushes Send off-screen. */}
        <div className="home-composer w-full max-[480px]:[&_.cx-optimize]:hidden max-[680px]:[@media(max-height:560px)]:[&_.cx-input-wrap]:pt-1.5 max-[680px]:[@media(max-height:560px)]:[&_.cx-input-wrap_textarea]:min-h-8">
          <Composer
            variant="home"
            onError={(m) => toast(m)}
            onProjectChange={setProject}
            projectSeed={newSessionProject || undefined}
          />
        </div>
      </div>
    </div>
  );
}
