// Manual theme override — Codex's Appearance System / Light / Dark. "system"
// clears data-theme so the @media(prefers-color-scheme) default applies; an
// explicit choice sets data-theme, which the CSS treats as authoritative.
export type Theme = "system" | "light" | "dark";

const KEY = "arwebui.theme";

export function loadTheme(): Theme {
  try {
    const t = localStorage.getItem(KEY);
    if (t === "light" || t === "dark" || t === "system") return t;
  } catch {
    /* ignore */
  }
  return "system";
}

export function applyTheme(t: Theme) {
  const root = document.documentElement;
  if (t === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", t);
}

export function saveTheme(t: Theme) {
  try {
    localStorage.setItem(KEY, t);
  } catch {
    /* ignore */
  }
  applyTheme(t);
}

export const nextTheme = (t: Theme): Theme =>
  t === "system" ? "light" : t === "light" ? "dark" : "system";

export const themeIcon = (t: Theme) => (t === "system" ? "🖥" : t === "light" ? "☀️" : "🌙");

// ============================================================================
// INC-41 H2 · Appearance settings (Codex Settings → Appearance)
// theme.ts is the webui's settings-persistence hub: theme, the appearance
// blob below, and the git prefs at the bottom all live here so the one module
// main.tsx already boots can restore everything before first paint.
// ============================================================================

export type DiffMarkers = "color" | "signs"; // Codex's Diff markers: Color / +-

export interface Appearance {
  theme: Theme;
  uiFontSize: number; // px — base UI text (Codex default 14)
  codeFontSize: number; // px — mono / diff / code blocks (Codex default 12)
  contrast: number; // 0–100, 50 = the theme's own values (Codex default ~45)
  diffMarkers: DiffMarkers;
  reduceMotion: boolean;
  syntax: boolean; // diff syntax highlighting (D3)
}

export const APPEARANCE_DEFAULTS: Appearance = {
  theme: "system",
  uiFontSize: 14,
  codeFontSize: 12,
  contrast: 50,
  diffMarkers: "color",
  reduceMotion: false,
  syntax: true,
};

export const UI_FONT_RANGE = { min: 12, max: 18 } as const;
export const CODE_FONT_RANGE = { min: 10, max: 16 } as const;

const APPEARANCE_KEY = "arwebui.appearance";

export function loadAppearance(): Appearance {
  let stored: Partial<Appearance> = {};
  try {
    const raw = JSON.parse(localStorage.getItem(APPEARANCE_KEY) || "{}");
    if (raw && typeof raw === "object") stored = raw;
  } catch {
    /* ignore */
  }
  // The sidebar theme toggle writes the legacy theme key directly, so treat it
  // as canonical for `theme` — saveAppearance keeps it in sync from this side.
  return { ...APPEARANCE_DEFAULTS, ...stored, theme: loadTheme() };
}

export function saveAppearance(a: Appearance) {
  try {
    localStorage.setItem(APPEARANCE_KEY, JSON.stringify(a));
  } catch {
    /* ignore quota */
  }
  saveTheme(a.theme); // keep the legacy theme key + boot path in agreement
  applyAppearance(a);
}

// mixHex linearly blends two "#rrggbb" colors; `amt` 0→a, 1→b. Returns `a`
// unchanged if either side isn't a plain hex triple (so var() indirection or
// named colors never produce garbage).
export function mixHex(a: string, b: string, amt: number): string {
  const parse = (s: string) => {
    const m = /^#?([0-9a-f]{6})$/i.exec(s.trim());
    if (!m) return null;
    const v = parseInt(m[1], 16);
    return [(v >> 16) & 255, (v >> 8) & 255, v & 255] as [number, number, number];
  };
  const ca = parse(a);
  const cb = parse(b);
  if (!ca || !cb) return a;
  const t = Math.max(0, Math.min(1, amt));
  const ch = (x: number, y: number) => Math.round(x + (y - x) * t);
  const hex = (x: number) => x.toString(16).padStart(2, "0");
  return `#${hex(ch(ca[0], cb[0]))}${hex(ch(ca[1], cb[1]))}${hex(ch(ca[2], cb[2]))}`;
}

// applyContrast nudges the three "quiet" palette entries toward ink (higher
// contrast) or toward the background (lower) around the 50 pivot. Inline vars
// on :root beat any stylesheet rule, so this wins regardless of CSS order and
// creates no new stacking context (unlike a filter).
function applyContrast(pct: number) {
  const root = document.documentElement;
  const vars = ["--dim", "--ink-2", "--line"];
  for (const v of vars) root.style.removeProperty(v); // read the theme base cleanly
  if (pct === 50) return;
  const cs = getComputedStyle(root);
  const ink = cs.getPropertyValue("--ink").trim();
  const bg = cs.getPropertyValue("--bg").trim();
  const f = (pct - 50) / 50; // -1 (softer) … +1 (stronger)
  const target = f >= 0 ? ink : bg;
  const amt = Math.abs(f) * 0.6;
  for (const v of vars) {
    const base = cs.getPropertyValue(v).trim();
    root.style.setProperty(v, mixHex(base, target, amt));
  }
}

// applyAppearance writes every live CSS variable / root attribute the
// stylesheet reads. Pure DOM side-effect; idempotent; safe to call on boot and
// on every settings change.
export function applyAppearance(a: Appearance) {
  const root = document.documentElement;
  applyTheme(a.theme);
  root.style.setProperty("--ui-font-size", a.uiFontSize + "px");
  root.style.setProperty("--code-font-size", a.codeFontSize + "px");
  applyContrast(a.contrast);
  root.setAttribute("data-diff-markers", a.diffMarkers);
  root.toggleAttribute("data-reduce-motion", a.reduceMotion);
  root.toggleAttribute("data-syntax", a.syntax);
}

// ============================================================================
// INC-41 H4 · Git preferences
// Only commitTemplate has a wired effect today (it seeds the DiffView commit
// prompt). branchPrefix / prMergeMethod are recorded but not yet consumed —
// the worktree/PR flows that would read them live outside this slice (marked
// "not wired yet" in the Settings › Git panel rather than faked).
// ============================================================================

export interface GitPrefs {
  commitTemplate: string;
  branchPrefix: string;
  prMergeMethod: "merge" | "squash";
}

export const GIT_DEFAULTS: GitPrefs = {
  commitTemplate: "changes from agent session",
  branchPrefix: "",
  prMergeMethod: "squash",
};

const GIT_KEY = "arwebui.git";

export function loadGitPrefs(): GitPrefs {
  try {
    const raw = JSON.parse(localStorage.getItem(GIT_KEY) || "{}");
    if (raw && typeof raw === "object") return { ...GIT_DEFAULTS, ...raw };
  } catch {
    /* ignore */
  }
  return { ...GIT_DEFAULTS };
}

export function saveGitPrefs(g: GitPrefs) {
  try {
    localStorage.setItem(GIT_KEY, JSON.stringify(g));
  } catch {
    /* ignore quota */
  }
}

// resetAll restores factory appearance + git prefs (Settings › General).
export function resetAll() {
  try {
    localStorage.removeItem(APPEARANCE_KEY);
    localStorage.removeItem(GIT_KEY);
  } catch {
    /* ignore */
  }
  saveAppearance({ ...APPEARANCE_DEFAULTS });
}
