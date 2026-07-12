import { SHORTCUT_GROUPS, keyLabel } from "../shortcuts";
import { matchesQuery } from "./SettingsSearch";

// SettingsShortcuts is Codex's Settings → Keyboard shortcuts (INC-41 H3): a
// read-only, grouped view of every binding the app actually wires. The catalog
// lives in shortcuts.ts (single source of truth shared with the ? overlay);
// rebinding is out of scope, so each row is an action + its key chips.
export function SettingsShortcuts({ query }: { query: string }) {
  const groups = SHORTCUT_GROUPS.map((g) => ({
    ...g,
    items: g.items.filter((it) => matchesQuery(query, `${g.title} ${it.label} ${it.desc || ""} ${it.keys.map(keyLabel).join(" ")}`)),
  })).filter((g) => g.items.length);

  return (
    <div className="rs-panel max-w-[660px] mx-auto">
      <h2 className="rs-panel-title m-0 mb-[4px] text-[19px] font-[650]">Keyboard shortcuts</h2>
      <p className="rs-panel-sub m-0 mb-[22px] text-dim text-[13px] leading-[1.5]">Every shortcut the app binds today. This list is read-only — rebinding isn’t supported yet.</p>

      {groups.length === 0 && <div className="rs-noresults text-dim text-[13px] py-[8px]">No shortcuts match “{query}”.</div>}

      {groups.map((g) => (
        <section key={g.title} className="rs-sc-group mb-[14px]">
          <div className="rs-sc-grouptitle py-[6px] text-[10.5px] uppercase tracking-[0.6px] text-dim">{g.title}</div>
          {g.items.map((it, i) => (
            <div key={i} className="rs-sc-row flex items-center gap-[14px] px-[10px] py-[8px] rounded-[9px] hover:bg-panel-2">
              <div className="rs-sc-label flex-1 min-w-0 flex flex-col gap-px">
                <span className="text-[13.5px] text-ink">{it.label}</span>
                {it.desc && <span className="rs-sc-desc text-[11.5px] text-dim">{it.desc}</span>}
              </div>
              <div className="rs-sc-keys flex items-center gap-[4px] shrink-0">
                {it.keys.map((k, j) => (
                  <kbd key={j} className="rs-kbd kbd">
                    {keyLabel(k)}
                  </kbd>
                ))}
              </div>
            </div>
          ))}
        </section>
      ))}
    </div>
  );
}
