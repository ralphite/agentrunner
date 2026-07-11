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
    <div className="rs-panel">
      <h2 className="rs-panel-title">Keyboard shortcuts</h2>
      <p className="rs-panel-sub">Every shortcut the app binds today. This list is read-only — rebinding isn’t supported yet.</p>

      {groups.length === 0 && <div className="rs-noresults">No shortcuts match “{query}”.</div>}

      {groups.map((g) => (
        <section key={g.title} className="rs-sc-group">
          <div className="rs-sc-grouptitle">{g.title}</div>
          {g.items.map((it, i) => (
            <div key={i} className="rs-sc-row">
              <div className="rs-sc-label">
                <span>{it.label}</span>
                {it.desc && <span className="rs-sc-desc">{it.desc}</span>}
              </div>
              <div className="rs-sc-keys">
                {it.keys.map((k, j) => (
                  <kbd key={j} className="rs-kbd">
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
