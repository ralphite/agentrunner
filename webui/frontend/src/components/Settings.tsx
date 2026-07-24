import { useMemo, useRef, useState } from "react";
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
import { FocusScope, type FocusTarget } from "../ui/FocusScope";
import { SearchField } from "../ui/Field";
import { Button } from "../ui/Button";

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
  { id: "archived", label: "Archived sessions", group: "Archived", icon: Archive, keywords: "sessions conversations history project search unarchive restore" },
];

export function Settings({
  onClose,
  initialSection = "general",
  restoreFocus,
}: {
  onClose: () => void;
  initialSection?: SettingsSection;
  restoreFocus?: FocusTarget;
}) {
  const [section, setSection] = useState<SettingsSection>(initialSection);
  const [query, setQuery] = useState("");
  const [rev, setRev] = useState(0); // bump to remount panels after a reset
  const searchRef = useRef<HTMLInputElement>(null);

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
  // A section-name/group match means "show this page", not "find a row named
  // Git inside the Git page". Keep row-level filtering for terms such as
  // "commit" and "contrast".
  const panelQuery = activeDef && matchesQuery(query, `${activeDef.label} ${activeDef.group}`) ? "" : query;

  return (
    <FocusScope
      className="fixed inset-0 z-[60] flex h-[100dvh] overflow-hidden bg-bg text-ink font-sans text-[length:var(--ui-font-size)] leading-[1.55] max-[720px]:flex-col"
      role="dialog"
      aria-label="Settings"
      initialFocus={searchRef}
      restoreFocus={restoreFocus ?? true}
      onEscape={onClose}
    >
      <aside className="shrink-0 grow-0 basis-[264px] flex flex-col gap-[10px] px-[12px] py-[16px] border-r border-line bg-sidebar overflow-y-auto max-[720px]:basis-auto max-[720px]:grid max-[720px]:grid-cols-1 max-[720px]:items-center max-[720px]:gap-[7px] max-[720px]:px-[12px] max-[720px]:py-[8px] max-[720px]:border-r-0 max-[720px]:border-b max-[720px]:overflow-hidden">
        <Button variant="ghost" className="hidden items-center gap-[7px] self-start pt-[5px] pr-[10px] pb-[5px] pl-[7px] border-0 bg-transparent text-ink-2 text-[13px] rounded-[8px] hover:bg-panel-2 hover:text-ink max-[720px]:inline-flex max-[720px]:self-auto max-[720px]:whitespace-nowrap" onClick={onClose}>
          <ArrowLeft size={15} weight="bold" /> Back to app
        </Button>
        <SearchField
          ref={searchRef}
          type="text"
          className="text-[13.5px]"
          containerClassName="rounded-app px-[11px] py-[8px]"
          icon={<MagnifyingGlass size={14} />}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search settings…"
          aria-label="Search settings"
        />
        <nav className="flex flex-col gap-[12px] mt-[2px] max-[720px]:flex-row max-[720px]:flex-wrap max-[720px]:gap-[4px] max-[720px]:mt-0 max-[720px]:overflow-visible max-[720px]:pb-[1px]">
          {groups.length === 0 && <div className="px-[10px] py-[12px] text-dim text-[13px]">No settings match</div>}
          {groups.map((g) => (
            <div key={g.group} className="flex flex-col gap-[2px] max-[720px]:contents">
              <div className="text-[10.5px] uppercase tracking-[0.6px] text-dim px-[10px] py-[4px] max-[720px]:hidden">{g.group}</div>
              {g.items.map((s) => (
                <Button
                  variant="ghost"
                  pressed={s.id === active}
                  key={s.id}
                  className={"flex items-center gap-[10px] w-full px-[10px] py-[8px] border-0 rounded-[9px] text-[13.5px] text-left max-[720px]:w-auto max-[720px]:shrink-0 max-[720px]:gap-[7px] max-[720px]:px-[9px] max-[720px]:py-[6px] max-[720px]:text-[13px] max-[720px]:whitespace-nowrap " + (s.id === active ? "bg-[var(--rs-accent-soft)] text-[var(--rs-accent)] font-[550] max-[720px]:bg-panel-2 max-[720px]:text-ink" : "bg-transparent text-ink-2 hover:bg-panel-2 hover:text-ink")}
                  onClick={() => setSection(s.id)}
                  aria-current={s.id === active}
                >
                  <s.icon size={16} weight={s.id === active ? "fill" : "regular"} />
                  <span>{s.label}</span>
                </Button>
              ))}
            </div>
          ))}
        </nav>
      </aside>

      <main className="flex-1 min-w-0 min-h-0 flex flex-col overflow-hidden">
        <header className="shrink-0 flex items-center justify-between gap-[12px] px-[26px] py-[14px] border-b border-line max-[720px]:hidden">
          <div className="inline-flex items-center gap-[7px] text-[13px] text-dim">
            {activeDef && <activeDef.icon size={15} />} Settings <span className="opacity-60">›</span> {activeDef?.label}
          </div>
          <Button variant="outline" className="px-[14px] py-[5px] border border-line rounded-full bg-panel text-ink text-[12.5px] hover:bg-panel-2" onClick={onClose} aria-label="Close settings">
            Done
          </Button>
        </header>
        <div className="flex-1 min-h-0 overflow-y-auto overscroll-contain p-[26px] max-[720px]:p-[18px]" key={active + ":" + rev}>
          {active === "general" && <SettingsGeneral query={panelQuery} onReset={() => setRev((r) => r + 1)} />}
          {active === "appearance" && <SettingsAppearance query={panelQuery} />}
          {active === "shortcuts" && <SettingsShortcuts query={panelQuery} />}
          {active === "git" && <SettingsGit query={panelQuery} />}
          {active === "worktrees" && <SettingsWorktrees query={panelQuery} />}
          {active === "configuration" && <SettingsConfiguration query={panelQuery} />}
          {active === "archived" && <SettingsArchived query={panelQuery} onClose={onClose} />}
        </div>
      </main>
    </FocusScope>
  );
}
