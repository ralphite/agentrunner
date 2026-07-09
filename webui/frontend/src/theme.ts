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
