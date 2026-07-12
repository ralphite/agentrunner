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
    <div className="rs-panel max-w-[660px] mx-auto">
      <h2 className="rs-panel-title m-0 mb-[4px] text-[19px] font-[650]">Appearance</h2>
      <p className="rs-panel-sub m-0 mb-[22px] text-dim text-[13px] leading-[1.5]">Theme, type scale and diff rendering. Changes apply instantly and are remembered on this device.</p>

      {!any && <div className="rs-noresults text-dim text-[13px] py-[8px]">No appearance settings match “{query}”.</div>}

      {show("Theme system light dark") && (
        <section className="rs-row rs-row-block flex flex-col items-stretch justify-between gap-[12px] py-[16px] border-t border-line-2 first-of-type:border-t-0">
          <div className="rs-row-head min-w-0">
            <div className="rs-row-label flex items-center gap-[8px] text-[14px] text-ink">Theme</div>
            <div className="rs-row-desc mt-[3px] text-[12.5px] text-dim leading-[1.5]">Follow the system, or pin a light or dark appearance.</div>
          </div>
          <div className="rs-themecards grid grid-cols-3 gap-[12px] max-[720px]:grid-cols-1">
            {THEME_CARDS.map((c) => (
              <button
                key={c.id}
                className={
                  "rs-themecard flex flex-col gap-[8px] p-[10px] border rounded-[12px] bg-panel " +
                  (a.theme === c.id ? "sel border-rs-accent shadow-[0_0_0_1px_var(--rs-accent)]" : "border-line hover:border-dim")
                }
                onClick={() => patch({ theme: c.id })}
                aria-pressed={a.theme === c.id}
              >
                <ThemePreview id={c.id} />
                <span className="rs-themecard-name inline-flex items-center gap-[6px] text-[12.5px] text-ink">
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
        <section className="rs-row flex items-center justify-between gap-[22px] py-[16px] border-t border-line-2 first-of-type:border-t-0">
          <div className="rs-row-head min-w-0">
            <div className="rs-row-label flex items-center gap-[8px] text-[14px] text-ink">Contrast</div>
            <div className="rs-row-desc mt-[3px] text-[12.5px] text-dim leading-[1.5]">Strengthen or soften secondary text and borders.</div>
          </div>
          <div className="rs-slider inline-flex items-center gap-[10px] shrink-0">
            <input
              type="range"
              className="w-[160px] accent-rs-accent"
              min={0}
              max={100}
              step={5}
              value={a.contrast}
              onChange={(e) => patch({ contrast: Number(e.target.value) })}
              aria-label="Contrast"
            />
            <span className="rs-slider-val min-w-[52px] text-right text-[12.5px] text-dim tabular-nums">{a.contrast === 50 ? "Default" : a.contrast > 50 ? `+${a.contrast - 50}` : `${a.contrast - 50}`}</span>
          </div>
        </section>
      )}

      {show("Diff markers color signs colorblind") && (
        <section className="rs-row flex items-center justify-between gap-[22px] py-[16px] border-t border-line-2 first-of-type:border-t-0">
          <div className="rs-row-head min-w-0">
            <div className="rs-row-label flex items-center gap-[8px] text-[14px] text-ink">Diff markers</div>
            <div className="rs-row-desc mt-[3px] text-[12.5px] text-dim leading-[1.5]">How added and removed lines are distinguished.</div>
          </div>
          <div className="rs-seg inline-flex shrink-0 border border-line rounded-[9px] overflow-hidden">
            {(["color", "signs"] as DiffMarkers[]).map((m) => (
              <button
                key={m}
                className={SEG_BTN + (a.diffMarkers === m ? " sel bg-rs-accent-soft text-rs-accent font-[550]" : " bg-panel text-ink-2")}
                onClick={() => patch({ diffMarkers: m })}
              >
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

// Shared utility strings for the rs-seg segmented control (kept alongside the
// semantic class names — see SettingsGit for the other consumer).
export const SEG_BTN =
  "rs-seg-btn px-[16px] py-[6px] border-0 rounded-none text-[13px] [&:not(:first-child)]:border-l [&:not(:first-child)]:border-line";

const TP = "rs-tp flex h-[56px] rounded-[7px] overflow-hidden border border-line-2";

// ThemePreview draws a tiny fixed-color window mock so each card previews its
// theme regardless of the app's current one; "system" is split down the middle.
function ThemePreview({ id }: { id: Theme }) {
  const light = { bg: "#ffffff", side: "#f4f4f4", ink: "#0d0d0d", line: "#e7e7e7", accent: "#0169cc" };
  const dark = { bg: "#17171a", side: "#141416", ink: "#ececf1", line: "#2a2a30", accent: "#6f9bff" };
  const bar = "h-[4px] rounded-[2px]";
  const half = (p: typeof light, side: "l" | "r") => (
    <div className="rs-tp-half flex-1 flex h-full" style={{ background: p.bg }}>
      <div className="rs-tp-side w-[26%] border-r flex flex-col gap-[4px] px-[5px] py-[6px]" style={{ background: p.side, borderColor: p.line }}>
        <span className="h-[5px] rounded-[2px]" style={{ background: p.accent }} />
        <span className={bar} style={{ background: p.line }} />
        <span className={bar} style={{ background: p.line }} />
      </div>
      <div className="rs-tp-body flex-1 flex flex-col gap-[5px] px-[6px] py-[7px]">
        <span className={bar + " w-[80%]"} style={{ background: p.ink, opacity: 0.85 }} />
        <span className={bar + " w-[62%]"} style={{ background: p.line }} />
        <span className={bar + " w-[40%]"} style={{ background: side === "l" ? p.accent : p.line }} />
      </div>
    </div>
  );
  if (id === "light") return <div className={TP}>{half(light, "l")}</div>;
  if (id === "dark") return <div className={TP}>{half(dark, "l")}</div>;
  return (
    <div className={TP + " rs-tp-split relative"}>
      <div className="rs-tp-clipL w-1/2 overflow-hidden [&>.rs-tp-half]:w-[200%]">{half(light, "l")}</div>
      <div className="rs-tp-clipR w-1/2 overflow-hidden [&>.rs-tp-half]:w-[200%] [&>.rs-tp-half]:-translate-x-1/2">{half(dark, "r")}</div>
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
  const step = "rs-step w-[26px] h-[26px] p-0 border border-line rounded-[7px] bg-panel text-ink text-[15px] leading-none hover:bg-panel-2";
  return (
    <section className="rs-row flex items-center justify-between gap-[22px] py-[16px] border-t border-line-2 first-of-type:border-t-0">
      <div className="rs-row-head min-w-0">
        <div className="rs-row-label flex items-center gap-[8px] text-[14px] text-ink">{label}</div>
        <div className="rs-row-desc mt-[3px] text-[12.5px] text-dim leading-[1.5]">{desc}</div>
      </div>
      <div className="rs-slider inline-flex items-center gap-[10px] shrink-0">
        <button className={step} onClick={() => onChange(Math.max(min, value - 1))} aria-label={`Decrease ${label}`}>
          −
        </button>
        <span
          className={"rs-fontpreview min-w-[52px] text-center text-[12.5px] text-ink-2" + (mono ? " mono" : "")}
          style={mono ? { fontSize: value } : undefined}
        >
          {value}px
        </span>
        <button className={step} onClick={() => onChange(Math.min(max, value + 1))} aria-label={`Increase ${label}`}>
          +
        </button>
      </div>
    </section>
  );
}

function ToggleRow({ label, desc, checked, onChange }: { label: string; desc: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <section className="rs-row flex items-center justify-between gap-[22px] py-[16px] border-t border-line-2 first-of-type:border-t-0">
      <div className="rs-row-head min-w-0">
        <div className="rs-row-label flex items-center gap-[8px] text-[14px] text-ink">{label}</div>
        <div className="rs-row-desc mt-[3px] text-[12.5px] text-dim leading-[1.5]">{desc}</div>
      </div>
      <button
        className={
          "rs-switch relative shrink-0 w-[40px] h-[24px] p-0 border-0 rounded-full transition-colors duration-150 " +
          (checked ? "on bg-rs-accent shadow-none" : "bg-panel-2 shadow-[inset_0_0_0_1px_var(--line)]")
        }
        role="switch"
        aria-checked={checked}
        aria-label={label}
        onClick={() => onChange(!checked)}
      >
        <span
          className={
            "rs-switch-knob absolute top-[3px] left-[3px] w-[18px] h-[18px] rounded-full bg-white shadow-[0_1px_2px_rgba(0,0,0,0.25)] transition-transform duration-150" +
            (checked ? " translate-x-[16px]" : "")
          }
        />
      </button>
    </section>
  );
}
