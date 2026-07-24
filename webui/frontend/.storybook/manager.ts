import { addons } from "storybook/manager-api";
import { create } from "storybook/theming";

const agentRunnerTheme = create({
  base: "light",
  brandTitle: "AgentRunner UI Workbench",

  colorPrimary: "#0d0d0d",
  colorSecondary: "#0d0d0d",

  appBg: "#f7f7f8",
  appContentBg: "#ffffff",
  appHoverBg: "#eeeeee",
  appPreviewBg: "#f3f3f4",
  appBorderColor: "#dcdcdc",
  appBorderRadius: 8,

  fontBase:
    '-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif',
  fontCode: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, monospace',

  textColor: "#0d0d0d",
  textInverseColor: "#ffffff",
  textMutedColor: "#606060",

  barTextColor: "#606060",
  barHoverColor: "#0d0d0d",
  barSelectedColor: "#0d0d0d",
  barBg: "#ffffff",

  buttonBg: "#ffffff",
  buttonBorder: "#dcdcdc",
  booleanBg: "#dcdcdc",
  booleanSelectedBg: "#0d0d0d",

  inputBg: "#ffffff",
  inputBorder: "#dcdcdc",
  inputTextColor: "#0d0d0d",
  inputBorderRadius: 8,
});

addons.setConfig({
  theme: agentRunnerTheme,
  layout: {
    initialActive: "sidebar",
    panelPosition: "bottom",
    showNav: true,
    showPanel: true,
    showTabs: true,
    showToolbar: true,
  },
  sidebar: {
    showRoots: true,
  },
  ui: {
    enableShortcuts: true,
  },
});
