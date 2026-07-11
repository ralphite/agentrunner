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
  Archive,
} from "@phosphor-icons/react";
import { matchesQuery } from "./SettingsSearch";
import { SettingsGeneral } from "./SettingsGeneral";
import { SettingsAppearance } from "./SettingsAppearance";
import { SettingsShortcuts } from "./SettingsShortcuts";
import { SettingsGit } from "./SettingsGit";
import { SettingsWorktrees } from "./SettingsWorktrees";
import { SettingsConfiguration } from "./SettingsConfiguration";
import { SettingsArchived } from "./SettingsArchived";

// Settings is Codex's full-window Settings surface (INC-41 H1): a left nav rail
// (grouped sections + "Search settings…"), a "← Back to app" affordance, and a
// single content pane. Opened with ⌘, and closed with Escape / Back. Each
// section is its own panel component so panels stay independently owned.
export type SettingsSection = "general" | "appearance" | "shortcuts" | "git" | "worktrees" | "configuration" | "archived";

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
  { id: "archived", label: "Archived tasks", group: "Archived", icon: Archive, keywords: "tasks conversations history project search unarchive restore" },
];

export function Settings({ onClose, initialSection = "appearance" }: { onClose: () => void; initialSection?: SettingsSection }) {
  const [section, setSection] = useState<SettingsSection>(initialSection);
  const [query, setQuery] = useState("");
  const [rev, setRev] = useState(0); // bump to remount panels after a reset
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    searchRef.current?.focus();
  }, []);

  const visible = useMemo(() => SECTIONS.filter((s) => s.id === section || matchesQuery(query, `${s.label} ${s.group} ${s.keywords}`)), [query, section]);

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
      className="fixed inset-0 z-[60] flex bg-bg text-ink font-sans text-[length:var(--ui-font-size)] leading-[1.55] max-[720px]:flex-col"
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
      <aside className="shrink-0 grow-0 basis-[264px] flex flex-col gap-[10px] px-[12px] py-[16px] border-r border-line bg-sidebar overflow-y-auto max-[720px]:basis-auto max-[720px]:border-r-0 max-[720px]:border-b max-[720px]:max-h-[45vh]">
        <button className="inline-flex items-center gap-[7px] self-start pt-[5px] pr-[10px] pb-[5px] pl-[7px] border-0 bg-transparent text-ink-2 text-[13px] rounded-[8px] hover:bg-panel-2 hover:text-ink" onClick={onClose}>
          <ArrowLeft size={15} weight="bold" /> Back to app
        </button>
        <div className="flex items-center gap-[8px] px-[11px] py-[8px] border border-line rounded-app bg-panel text-dim focus-within:border-[var(--rs-accent)]">
          <MagnifyingGlass size={14} />
          <input
            ref={searchRef}
            className="flex-1 min-w-0 border-0 bg-transparent p-0 text-[13.5px] text-ink focus:outline-none"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search settings…"
            aria-label="Search settings"
          />
        </div>
        <nav className="flex flex-col gap-[12px] mt-[2px]">
          {groups.length === 0 && <div className="px-[10px] py-[12px] text-dim text-[13px]">No settings match</div>}
          {groups.map((g) => (
            <div key={g.group} className="flex flex-col gap-[2px]">
              <div className="text-[10.5px] uppercase tracking-[0.6px] text-dim px-[10px] py-[4px]">{g.group}</div>
              {g.items.map((s) => (
                <button
                  key={s.id}
                  className={"flex items-center gap-[10px] w-full px-[10px] py-[8px] border-0 rounded-[9px] text-[13.5px] text-left " + (s.id === active ? "bg-[var(--rs-accent-soft)] text-[var(--rs-accent)] font-[550]" : "bg-transparent text-ink-2 hover:bg-panel-2 hover:text-ink")}
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

      <main className="flex-1 min-w-0 flex flex-col">
        <header className="flex items-center justify-between gap-[12px] px-[26px] py-[14px] border-b border-line max-[720px]:px-[18px] max-[720px]:py-[12px]">
          <div className="inline-flex items-center gap-[7px] text-[13px] text-dim">
            {activeDef && <activeDef.icon size={15} />} Settings <span className="opacity-60">›</span> {activeDef?.label}
          </div>
          <button className="px-[14px] py-[5px] border border-line rounded-full bg-panel text-ink text-[12.5px] hover:bg-panel-2" onClick={onClose} aria-label="Close settings">
            Done
          </button>
        </header>
        <div className="flex-1 overflow-y-auto p-[26px] max-[720px]:p-[18px]" key={active + ":" + rev}>
          {active === "general" && <SettingsGeneral query={query} onReset={() => setRev((r) => r + 1)} />}
          {active === "appearance" && <SettingsAppearance query={query} />}
          {active === "shortcuts" && <SettingsShortcuts query={query} />}
          {active === "git" && <SettingsGit query={query} />}
          {active === "worktrees" && <SettingsWorktrees query={query} />}
          {active === "configuration" && <SettingsConfiguration query={query} />}
          {active === "archived" && <SettingsArchived query={query} onClose={onClose} />}
        </div>
      </main>
    </div>
  );
}
