import type { Preview } from "@storybook/react-vite";
import { initialize, mswLoader } from "msw-storybook-addon";
import { useEffect, type ReactNode } from "react";
import { applyTheme, type Theme } from "../src/theme";
import "../src/tw.css";

initialize({
  onUnhandledRequest(request, print) {
    if (new URL(request.url).pathname.startsWith("/api/")) {
      print.error();
    }
  },
});

function StorySurface({
  children,
  fullHeight,
  theme,
}: {
  children: ReactNode;
  fullHeight: boolean;
  theme: Theme;
}) {
  // Full-page Stories run production appearance effects that restore persisted
  // preferences after the decorator renders. Re-apply the toolbar selection
  // from the outer effect so the Storybook control remains authoritative.
  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  return (
    <div
      className={
        fullHeight
          ? "h-[100dvh] min-h-0 overflow-clip bg-bg text-ink"
          : "min-h-screen bg-bg text-ink"
      }
    >
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
    viewport: { value: "responsive", isRotated: false },
  },
  decorators: [
    (Story, context) => {
      const theme = (context.globals.theme as Theme | undefined) ?? "light";
      applyTheme(theme);
      return (
        <StorySurface
          fullHeight={context.parameters.fullHeight === true}
          theme={theme}
        >
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
      // Page Stories deliberately hide manager chrome so the production shell
      // can use the full canvas. Storybook persists those option overrides
      // across navigation, so restate the normal component-story defaults here.
      showNav: true,
      showPanel: true,
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
    viewport: {
      options: {
        responsive: {
          name: "Responsive canvas",
          styles: { width: "100%", height: "100%" },
          type: "desktop",
        },
        desktop: {
          name: "Desktop",
          styles: { width: "1280px", height: "720px" },
          type: "desktop",
        },
        phone: {
          name: "Phone",
          styles: { width: "390px", height: "844px" },
          type: "mobile",
        },
      },
    },
  },
};

export default preview;
