import type { Preview } from "@storybook/react-vite";
import { initialize, mswLoader } from "msw-storybook-addon";
import type { ReactNode } from "react";
import { applyTheme, type Theme } from "../src/theme";
import "../src/tw.css";

initialize({
  onUnhandledRequest(request, print) {
    if (new URL(request.url).pathname.startsWith("/api/")) {
      print.error();
    }
  },
});

function StorySurface({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen bg-bg text-ink">
      {children}
      <div id="modal-root" />
      <div id="popover-root" />
    </div>
  );
}

const preview: Preview = {
  globalTypes: {
    theme: {
      description: "AgentRunner theme",
      toolbar: {
        icon: "paintbrush",
        items: [
          { value: "light", title: "Light" },
          { value: "dark", title: "Dark" },
          { value: "system", title: "System" },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: {
    theme: "light",
  },
  decorators: [
    (Story, context) => {
      applyTheme((context.globals.theme as Theme | undefined) ?? "light");
      return (
        <StorySurface>
          <Story />
        </StorySurface>
      );
    },
  ],
  loaders: [mswLoader],
  parameters: {
    a11y: {
      test: "error",
    },
    backgrounds: {
      disable: true,
    },
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    options: {
      storySort: {
        order: [
          "Foundations",
          "Components",
          "Features",
          "Pages",
          "CUJs",
          "Demos",
          "Future",
        ],
      },
    },
    layout: "fullscreen",
  },
};

export default preview;
