import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  MagnifyingGlass,
  Gear,
  Palette,
  Keyboard,
  GitBranch,
  TreeStructure,
  SlidersHorizontal,
} from "@phosphor-icons/react";
import { matchesQuery } from "./SettingsSearch";
import { SettingsGeneral } from "./SettingsGeneral";
import { SettingsAppearance } from "./SettingsAppearance";
import { SettingsShortcuts } from "./SettingsShortcuts";
import { SettingsGit } from "./SettingsGit";
import { SettingsWorktrees } from "./SettingsWorktrees";
import { SettingsConfiguration } from "./SettingsConfiguration";

// Settings is Codex's full-window Settings surface (INC-41 H1): a left nav rail
// (grouped sections + "Search settings…"), a "← Back to app" affordance, and a
// single content pane. Opened with ⌘, and closed with Escape / Back. Each
// section is its own panel component so panels stay independently owned.
export type SettingsSection = "general" | "appearance" | "shortcuts" | "git" | "worktrees" | "configuration";

interface SectionDef {
  id: SettingsSection;
  label: string;
  group: string;
  icon: typeof Gear;
  keywords: string;
}

const SECTIONS: SectionDef[] = [
  { id: "general", label: "General", group: "App", icon: Gear, keywords: "status reset defaults daemon" },
  { id: "appearance", label: "Appearance", group: "App", icon: Palette, keywords: "theme dark light font size contrast diff markers motion syntax" },
  { id: "shortcuts", label: "Keyboard shortcuts", group: "App", icon: Keyboard, keywords: "keys bindings command palette composer" },
  { id: "git", label: "Git", group: "Coding", icon: GitBranch, keywords: "commit branch prefix pull request merge" },
  { id: "worktrees", label: "Worktrees", group: "Coding", icon: TreeStructure, keywords: "workspace repo conversations" },
  { id: "configuration", label: "Configuration", group: "Coding", icon: SlidersHorizontal, keywords: "version runtime daemon policy sandbox" },
];

export function Settings({ onClose, initialSection = "appearance" }: { onClose: () => void; initialSection?: SettingsSection }) {
  const [section, setSection] = useState<SettingsSection>(initialSection);
  const [query, setQuery] = useState("");
  const [rev, setRev] = useState(0); // bump to remount panels after a reset
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    searchRef.current?.focus();
  }, []);

  const visible = useMemo(() => SECTIONS.filter((s) => matchesQuery(query, `${s.label} ${s.group} ${s.keywords}`)), [query]);

  // If a search hides the active section, follow the first remaining one so the
  // pane never shows a section the rail no longer lists.
  const active: SettingsSection = visible.some((s) => s.id === section) ? section : visible[0]?.id ?? section;

  const groups = useMemo(() => {
    const order: string[] = [];
    const byGroup = new Map<string, SectionDef[]>();
    for (const s of visible) {
      if (!byGroup.has(s.group)) {
        byGroup.set(s.group, []);
        order.push(s.group);
      }
      byGroup.get(s.group)!.push(s);
    }
    return order.map((g) => ({ group: g, items: byGroup.get(g)! }));
  }, [visible]);

  const activeDef = SECTIONS.find((s) => s.id === active);

  return (
    <div
      className="rs-settings"
      role="dialog"
      aria-label="Settings"
      tabIndex={-1}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.stopPropagation();
          onClose();
        }
      }}
    >
      <aside className="rs-nav">
        <button className="rs-back" onClick={onClose}>
          <ArrowLeft size={15} weight="bold" /> Back to app
        </button>
        <div className="rs-search">
          <MagnifyingGlass size={14} />
          <input
            ref={searchRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search settings…"
            aria-label="Search settings"
          />
        </div>
        <nav className="rs-navlist">
          {groups.length === 0 && <div className="rs-nav-empty">No settings match</div>}
          {groups.map((g) => (
            <div key={g.group} className="rs-navgroup">
              <div className="rs-navgroup-title">{g.group}</div>
              {g.items.map((s) => (
                <button
                  key={s.id}
                  className={"rs-navitem" + (s.id === active ? " sel" : "")}
                  onClick={() => setSection(s.id)}
                  aria-current={s.id === active}
                >
                  <s.icon size={16} weight={s.id === active ? "fill" : "regular"} />
                  <span>{s.label}</span>
                </button>
              ))}
            </div>
          ))}
        </nav>
      </aside>

      <main className="rs-content">
        <header className="rs-content-head">
          <div className="rs-crumb">
            {activeDef && <activeDef.icon size={15} />} Settings <span className="rs-crumb-sep">›</span> {activeDef?.label}
          </div>
          <button className="rs-close" onClick={onClose} aria-label="Close settings">
            Done
          </button>
        </header>
        <div className="rs-content-scroll" key={active + ":" + rev}>
          {active === "general" && <SettingsGeneral query={query} onReset={() => setRev((r) => r + 1)} />}
          {active === "appearance" && <SettingsAppearance query={query} />}
          {active === "shortcuts" && <SettingsShortcuts query={query} />}
          {active === "git" && <SettingsGit query={query} />}
          {active === "worktrees" && <SettingsWorktrees query={query} />}
          {active === "configuration" && <SettingsConfiguration query={query} />}
        </div>
      </main>
    </div>
  );
}
