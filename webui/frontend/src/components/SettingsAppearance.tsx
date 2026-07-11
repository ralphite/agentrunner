import { useState } from "react";
import { Monitor, Sun, Moon } from "@phosphor-icons/react";
import {
  type Appearance,
  type Theme,
  type DiffMarkers,
  loadAppearance,
  saveAppearance,
  UI_FONT_RANGE,
  CODE_FONT_RANGE,
} from "../theme";
import { matchesQuery } from "./SettingsSearch";
import { useStore } from "../store";

// SettingsAppearance is Codex's Settings → Appearance panel (INC-41 H2). Every
// control writes through saveAppearance, which persists to localStorage and
// re-applies the live CSS variables synchronously — so the whole app (this
// panel included) reflects the change on the next paint, no reload.
export function SettingsAppearance({ query }: { query: string }) {
  const [a, setA] = useState<Appearance>(loadAppearance);
  const patch = (p: Partial<Appearance>) => {
    const next = { ...a, ...p };
    setA(next);
    saveAppearance(next);
    // keep the sidebar's theme glyph in sync when the theme changes here
    if (p.theme && p.theme !== a.theme) useStore.setState({ theme: p.theme });
  };

  const show = (label: string, kw = "") => matchesQuery(query, label + " " + kw);
  const any =
    show("Theme system light dark") ||
    show("UI font size text") ||
    show("Code font size mono") ||
    show("Contrast") ||
    show("Diff markers color signs colorblind") ||
    show("Reduce motion animation") ||
    show("Syntax highlighting diff code");

  return (
    <div className="rs-panel">
      <h2 className="rs-panel-title">Appearance</h2>
      <p className="rs-panel-sub">Theme, type scale and diff rendering. Changes apply instantly and are remembered on this device.</p>

      {!any && <div className="rs-noresults">No appearance settings match “{query}”.</div>}

      {show("Theme system light dark") && (
        <section className="rs-row rs-row-block">
          <div className="rs-row-head">
            <div className="rs-row-label">Theme</div>
            <div className="rs-row-desc">Follow the system, or pin a light or dark appearance.</div>
          </div>
          <div className="rs-themecards">
            {THEME_CARDS.map((c) => (
              <button
                key={c.id}
                className={"rs-themecard" + (a.theme === c.id ? " sel" : "")}
                onClick={() => patch({ theme: c.id })}
                aria-pressed={a.theme === c.id}
              >
                <ThemePreview id={c.id} />
                <span className="rs-themecard-name">
                  <c.icon size={13} weight="bold" /> {c.label}
                </span>
              </button>
            ))}
          </div>
        </section>
      )}

      {show("UI font size text") && (
        <FontRow
          label="UI font size"
          desc="Base size for interface text."
          value={a.uiFontSize}
          min={UI_FONT_RANGE.min}
          max={UI_FONT_RANGE.max}
          onChange={(v) => patch({ uiFontSize: v })}
        />
      )}

      {show("Code font size mono monospace") && (
        <FontRow
          label="Code font size"
          desc="Monospace size for diffs and code blocks."
          value={a.codeFontSize}
          min={CODE_FONT_RANGE.min}
          max={CODE_FONT_RANGE.max}
          onChange={(v) => patch({ codeFontSize: v })}
          mono
        />
      )}

      {show("Contrast") && (
        <section className="rs-row">
          <div className="rs-row-head">
            <div className="rs-row-label">Contrast</div>
            <div className="rs-row-desc">Strengthen or soften secondary text and borders.</div>
          </div>
          <div className="rs-slider">
            <input
              type="range"
              min={0}
              max={100}
              step={5}
              value={a.contrast}
              onChange={(e) => patch({ contrast: Number(e.target.value) })}
              aria-label="Contrast"
            />
            <span className="rs-slider-val">{a.contrast === 50 ? "Default" : a.contrast > 50 ? `+${a.contrast - 50}` : `${a.contrast - 50}`}</span>
          </div>
        </section>
      )}

      {show("Diff markers color signs colorblind") && (
        <section className="rs-row">
          <div className="rs-row-head">
            <div className="rs-row-label">Diff markers</div>
            <div className="rs-row-desc">How added and removed lines are distinguished.</div>
          </div>
          <div className="rs-seg">
            {(["color", "signs"] as DiffMarkers[]).map((m) => (
              <button key={m} className={"rs-seg-btn" + (a.diffMarkers === m ? " sel" : "")} onClick={() => patch({ diffMarkers: m })}>
                {m === "color" ? "Color" : "+ / −"}
              </button>
            ))}
          </div>
        </section>
      )}

      {show("Syntax highlighting diff code") && (
        <ToggleRow
          label="Syntax highlighting"
          desc="Highlight keywords, strings and comments in diffs."
          checked={a.syntax}
          onChange={(v) => patch({ syntax: v })}
        />
      )}

      {show("Reduce motion animation") && (
        <ToggleRow
          label="Reduce motion"
          desc="Minimize transitions and animations."
          checked={a.reduceMotion}
          onChange={(v) => patch({ reduceMotion: v })}
        />
      )}
    </div>
  );
}

const THEME_CARDS: { id: Theme; label: string; icon: typeof Monitor }[] = [
  { id: "system", label: "System", icon: Monitor },
  { id: "light", label: "Light", icon: Sun },
  { id: "dark", label: "Dark", icon: Moon },
];

// ThemePreview draws a tiny fixed-color window mock so each card previews its
// theme regardless of the app's current one; "system" is split down the middle.
function ThemePreview({ id }: { id: Theme }) {
  const light = { bg: "#ffffff", side: "#f4f4f4", ink: "#0d0d0d", line: "#e7e7e7", accent: "#0169cc" };
  const dark = { bg: "#17171a", side: "#141416", ink: "#ececf1", line: "#2a2a30", accent: "#6f9bff" };
  const half = (p: typeof light, side: "l" | "r") => (
    <div className="rs-tp-half" style={{ background: p.bg }}>
      <div className="rs-tp-side" style={{ background: p.side, borderColor: p.line }}>
        <span style={{ background: p.accent }} />
        <span style={{ background: p.line }} />
        <span style={{ background: p.line }} />
      </div>
      <div className="rs-tp-body">
        <span style={{ background: p.ink, opacity: 0.85 }} />
        <span style={{ background: p.line }} />
        <span style={{ background: side === "l" ? p.accent : p.line }} />
      </div>
    </div>
  );
  if (id === "light") return <div className="rs-tp">{half(light, "l")}</div>;
  if (id === "dark") return <div className="rs-tp">{half(dark, "l")}</div>;
  return (
    <div className="rs-tp rs-tp-split">
      <div className="rs-tp-clipL">{half(light, "l")}</div>
      <div className="rs-tp-clipR">{half(dark, "r")}</div>
    </div>
  );
}

function FontRow({
  label,
  desc,
  value,
  min,
  max,
  onChange,
  mono,
}: {
  label: string;
  desc: string;
  value: number;
  min: number;
  max: number;
  onChange: (v: number) => void;
  mono?: boolean;
}) {
  return (
    <section className="rs-row">
      <div className="rs-row-head">
        <div className="rs-row-label">{label}</div>
        <div className="rs-row-desc">{desc}</div>
      </div>
      <div className="rs-slider">
        <button className="rs-step" onClick={() => onChange(Math.max(min, value - 1))} aria-label={`Decrease ${label}`}>
          −
        </button>
        <span className={"rs-fontpreview" + (mono ? " mono" : "")} style={mono ? { fontSize: value } : undefined}>
          {value}px
        </span>
        <button className="rs-step" onClick={() => onChange(Math.min(max, value + 1))} aria-label={`Increase ${label}`}>
          +
        </button>
      </div>
    </section>
  );
}

function ToggleRow({ label, desc, checked, onChange }: { label: string; desc: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <section className="rs-row">
      <div className="rs-row-head">
        <div className="rs-row-label">{label}</div>
        <div className="rs-row-desc">{desc}</div>
      </div>
      <button className={"rs-switch" + (checked ? " on" : "")} role="switch" aria-checked={checked} aria-label={label} onClick={() => onChange(!checked)}>
        <span className="rs-switch-knob" />
      </button>
    </section>
  );
}
