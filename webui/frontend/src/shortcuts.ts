// Catalog for the keyboard-shortcuts reference (Codex's Settings → Keyboard
// shortcuts). Every entry here is a shortcut the app actually binds — keep this
// in sync with the real handlers (App.tsx, CommandPalette.tsx, Composer.tsx).
export interface Shortcut {
  keys: string[]; // tokens; "mod" renders ⌘ on mac / Ctrl elsewhere
  label: string;
  desc?: string;
}
export interface ShortcutGroup {
  title: string;
  items: Shortcut[];
}

const isMac =
  (typeof navigator !== "undefined" &&
    (/mac/i.test(navigator.platform || "") || /mac/i.test(navigator.userAgent || ""))) ||
  false;

export const modLabel = isMac ? "⌘" : "Ctrl";

// keyLabel maps a token to its display glyph. Non-tokens (letters, "/", "?")
// pass through unchanged.
export function keyLabel(k: string): string {
  switch (k) {
    case "mod":
      return modLabel;
    case "shift":
      return isMac ? "⇧" : "Shift";
    case "alt":
      return isMac ? "⌥" : "Alt";
    case "enter":
      return "Enter";
    case "esc":
      return "Esc";
    case "up":
      return "↑";
    case "down":
      return "↓";
    case "tab":
      return "Tab";
    default:
      return k;
  }
}

export const SHORTCUT_GROUPS: ShortcutGroup[] = [
  {
    title: "Global",
    items: [
      { keys: ["mod", "K"], label: "Command palette", desc: "Search sessions and run commands" },
      { keys: ["mod", "alt", "up"], label: "Previous task", desc: "Select the task above in the sidebar" },
      { keys: ["mod", "alt", "down"], label: "Next task", desc: "Select the task below in the sidebar" },
      { keys: ["mod", "F"], label: "Find in conversation", desc: "Search the current task's messages" },
      { keys: ["mod", "B"], label: "Toggle sidebar", desc: "Show or hide the task sidebar" },
      { keys: ["mod", "enter"], label: "Approve request", desc: "Approve the pending approval in the open task" },
      { keys: ["mod", "⌫"], label: "Deny request", desc: "Deny the pending approval in the open task" },
      { keys: ["?"], label: "Keyboard shortcuts", desc: "Show this reference" },
    ],
  },
  {
    title: "Command palette",
    items: [
      { keys: ["up"], label: "Previous result" },
      { keys: ["down"], label: "Next result" },
      { keys: ["enter"], label: "Open selection" },
      { keys: ["esc"], label: "Close palette" },
    ],
  },
  {
    title: "Composer",
    items: [
      { keys: ["enter"], label: "Send message" },
      { keys: ["shift", "enter"], label: "New line" },
      { keys: ["/"], label: "Slash commands", desc: "Type / at the start of the line" },
    ],
  },
  {
    title: "Slash menu & dialogs",
    items: [
      { keys: ["up"], label: "Previous item" },
      { keys: ["down"], label: "Next item" },
      { keys: ["tab"], label: "Complete slash command" },
      { keys: ["esc"], label: "Close dialog or popover" },
    ],
  },
];
