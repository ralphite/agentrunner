import type { Preview } from "@storybook/react-vite";
import { initialize, mswLoader } from "msw-storybook-addon";
import { useEffect, type ReactNode } from "react";
import { addons } from "storybook/preview-api";
import { STORY_FINISHED } from "storybook/internal/core-events";
import {
  setStoryPlaybackPace,
  type StoryPlaybackPace,
} from "../src/storybook/humanPlayback";
import { applyTheme, type Theme } from "../src/theme";
import "../src/tw.css";

type ReviewSurface =
  | "component"
  | "panel"
  | "overlay"
  | "full-page"
  | "mobile-sheet";

const REVIEW_SURFACE_CLASS: Record<ReviewSurface, string> = {
  component: "contents",
  panel: "min-h-[100dvh] bg-bg p-6 text-ink",
  overlay:
    "grid min-h-[100dvh] place-items-center bg-panel-2 p-6 text-ink",
  "full-page": "h-[100dvh] min-h-0 overflow-clip bg-bg text-ink",
  "mobile-sheet":
    "mx-auto min-h-[100dvh] w-full max-w-[430px] border-x border-line bg-bg p-4 text-ink",
};

initialize({
  onUnhandledRequest(request, print) {
    if (new URL(request.url).pathname.startsWith("/api/")) {
      print.error();
    }
  },
});

const QA_FRAME_MESSAGE = "agentrunner-story-frame";

function reportNestedStoryFrame(
  storyId: string,
  status: "ready" | "failed",
) {
  if (globalThis.window?.parent === globalThis.window) return;
  const frameRevision =
    new URLSearchParams(globalThis.window.location.search).get("qaDemo") ??
    "";
  globalThis.window?.parent.postMessage(
    { type: QA_FRAME_MESSAGE, storyId, frameRevision, status },
    globalThis.window.location.origin,
  );
}

// Mirror Storybook's terminal result into the parent QA playlist. A render can
// briefly enter an errored phase while Storybook retries it, so only the
// canonical finished event is authoritative.
addons
  .getChannel()
  .on(
    STORY_FINISHED,
    ({
      status,
      storyId,
    }: {
      status: "success" | "error";
      storyId: string;
    }) => {
      reportNestedStoryFrame(storyId, status === "success" ? "ready" : "failed");
    },
  );

function StorySurface({
  children,
  frameRevision,
  reviewSurface,
  storyId,
  theme,
}: {
  children: ReactNode;
  frameRevision: string;
  reviewSurface: ReviewSurface;
  storyId: string;
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
      className={REVIEW_SURFACE_CLASS[reviewSurface]}
      data-qa-frame-revision={frameRevision}
      data-review-surface={reviewSurface}
      data-story-id={storyId}
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
    playbackPace: {
      description: "Story interaction playback pace",
      toolbar: {
        icon: "timer",
        items: [
          { value: "human", title: "Human playback" },
          { value: "automated", title: "Automated QA" },
          { value: "instant", title: "Instant" },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: {
    playbackPace: "human",
    theme: "light",
    viewport: { value: "responsive", isRotated: false },
  },
  decorators: [
    (Story, context) => {
      const theme = (context.globals.theme as Theme | undefined) ?? "light";
      const reviewSurface = (
        context.parameters.reviewSurface ??
        (context.parameters.fullHeight === true ? "full-page" : "component")
      ) as ReviewSurface;
      const playbackPace =
        (context.globals.playbackPace as StoryPlaybackPace | undefined) ??
        "human";
      const frameRevision =
        globalThis.window == null
          ? ""
          : new URLSearchParams(globalThis.window.location.search).get(
              "qaDemo",
            ) ?? "";
      setStoryPlaybackPace(playbackPace);
      applyTheme(theme);
      return (
        <StorySurface
          key={`${context.id}:${frameRevision}`}
          frameRevision={frameRevision}
          reviewSurface={reviewSurface}
          storyId={context.id}
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
      // color-contrast is OFF by product decision (operator, 2026-07-30): the
      // palette's dim text is deliberately low-contrast, so axe flagged it on
      // ~387 stories and the browser gate had been RED continuously for days.
      // A gate that is always red catches nothing — muting this one rule is
      // what makes the remaining a11y checks (roles, labels, focus order,
      // duplicate ids) meaningful again. Everything else stays at "error".
      config: {
        rules: [{ id: "color-contrast", enabled: false }],
      },
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
      layout: {
        showNav: true,
        showPanel: true,
      },
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
        phoneCompact: {
          name: "Phone · compact",
          styles: { width: "320px", height: "640px" },
          type: "mobile",
        },
        phoneNarrow: {
          name: "Phone · narrow",
          styles: { width: "360px", height: "640px" },
          type: "mobile",
        },
        phoneSmall: {
          name: "Phone · small",
          styles: { width: "375px", height: "667px" },
          type: "mobile",
        },
        phone: {
          name: "Phone",
          styles: { width: "390px", height: "844px" },
          type: "mobile",
        },
        phoneShort: {
          name: "Phone · short",
          styles: { width: "390px", height: "500px" },
          type: "mobile",
        },
        phoneWide: {
          name: "Phone · wide",
          styles: { width: "430px", height: "932px" },
          type: "mobile",
        },
      },
    },
  },
};

export default preview;
