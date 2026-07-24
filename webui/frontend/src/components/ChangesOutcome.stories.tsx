import { useState, type ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import type { FileDiffSummary } from "../diffSummary";
import type { DiffResp, DiffScope } from "../types";
import {
  ArtifactChips as RenderArtifactChips,
  ArtifactRow as RenderArtifactRow,
  ChangesOutcome,
  ChangesShell as RenderChangesShell,
  ImageArtifacts as RenderImageArtifacts,
  ImageCard as RenderImageCard,
} from "./ChangesOutcome";

type StoryApi = AppServices["api"];
type FixtureMode = "ready" | "workspace-fallback" | "failure-then-ready";

const SID = "story-changes-outcome";
const svgPreview = (label: string) =>
  `data:image/svg+xml;charset=utf-8,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="336" height="208" viewBox="0 0 336 208">
      <rect width="336" height="208" fill="#dbeafe"/>
      <path d="M0 162 82 86l54 49 50-61 150 134H0Z" fill="#60a5fa"/>
      <text x="18" y="34" fill="#172554" font-family="sans-serif" font-size="18">${label}</text>
    </svg>`,
  )}`;

const diffText = (paths: readonly string[]) =>
  paths
    .map(
      (path, index) => `diff --git a/${path} b/${path}
--- a/${path}
+++ b/${path}
@@ -1,3 +1,4 @@
 export const value = ${index};
-export const status = "old";
+export const status = "reviewed";
+export const note = "${"long-content-".repeat(index + 1)}";
`,
    )
    .join("");

const paths = [
  "src/app/runtime.ts",
  "src/components/ScenarioControls.tsx",
  "src/storybook/scenarios/coreSession.ts",
  "src/styles/very-long-component-name-that-must-truncate-on-small-screens.css",
  "tests/coreSession.browser.test.ts",
  "docs/demo-playback.ts",
] as const;

const documentFiles: FileDiffSummary[] = [
  "docs/release-notes.md",
  "reports/a11y-results.pdf",
  "notes/browser-qa.txt",
  "docs/component-inventory.rst",
  "exports/review-summary.docx",
].map((path) => ({
  path,
  lines: [],
  add: 12,
  del: 2,
  countsKnown: true,
}));

const imageFiles: FileDiffSummary[] = [
  "qa/desktop-light.png",
  "qa/desktop-dark.png",
  "qa/phone-light.png",
  "qa/phone-dark.png",
  "qa/review-panel.png",
  "qa/timeline.png",
  "qa/settings.png",
].map((path) => ({
  path,
  lines: [],
  add: 0,
  del: 0,
  countsKnown: false,
}));

const readyDiff: DiffResp = {
  scope: "last-turn",
  workspace: "/worktrees/storybook-agent-runner",
  known: true,
  isRepo: true,
  diff: diffText(paths),
  numstat: "",
  untracked: [],
  conflicts: ["src/components/ScenarioControls.tsx"],
};

const emptyTurn: DiffResp = {
  ...readyDiff,
  diff: "",
  conflicts: [],
};

function isolatedApi(overrides: Partial<StoryApi>): StoryApi {
  return new Proxy(overrides as StoryApi, {
    get(target, property, receiver) {
      if (Reflect.has(target, property)) {
        return Reflect.get(target, property, receiver);
      }
      return () =>
        Promise.reject(
          new Error(`Unexpected Story API call: ${String(property)}`),
        );
    },
  });
}

function ChangesOutcomeFixture({
  mode = "ready",
  onReview,
}: {
  mode?: FixtureMode;
  onReview: (scope: "turn" | "workspace") => void;
}) {
  const [api] = useState(() => {
    let request = 0;
    return isolatedApi({
      diff: fn(async (_sid: string, scope: DiffScope = "working-tree") => {
        request += 1;
        if (mode === "failure-then-ready" && request === 1) {
          throw new Error("The in-memory diff fixture failed");
        }
        if (mode === "workspace-fallback") {
          return scope === "last-turn"
            ? emptyTurn
            : {
                ...readyDiff,
                scope: "working-tree" as const,
                conflicts: [],
              };
        }
        return readyDiff;
      }),
      revert: fn(async () => ({ status: "ok" })),
      fileURL: (_sid: string, path: string) => svgPreview(path),
    });
  });

  return (
    <StoryAppFrame
      initialState={{ currentSid: SID }}
      services={{ api }}
    >
      <main className="mx-auto w-full max-w-[760px] p-6">
        <ChangesOutcome sid={SID} refreshKey={0} onReview={onReview} />
      </main>
    </StoryAppFrame>
  );
}

function LeafFrame({ children }: { children: ReactNode }) {
  const [api] = useState(() =>
    isolatedApi({
      fileURL: (_sid: string, path: string) => svgPreview(path),
    }),
  );
  return (
    <StoryAppFrame initialState={{ currentSid: SID }} services={{ api }}>
      <main className="mx-auto w-full max-w-[760px] p-6">{children}</main>
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Changes/ChangesOutcome",
  component: ChangesOutcome,
  parameters: {
    layout: "fullscreen",
  },
  args: {
    sid: SID,
    refreshKey: 0,
    onReview: fn(),
  },
  render: ({ onReview }) => <ChangesOutcomeFixture onReview={onReview} />,
} satisfies Meta<typeof ChangesOutcome>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.findByText("Edited 6 files")).resolves.toBeVisible();
    await expect(canvas.getByText("1 merge conflict")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Show 3 more files" }),
    ).toBeVisible();
  },
};

export const KeyboardNavigation: Story = {
  args: {
    onReview: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Edited 6 files");
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: /Undo/ })).toHaveFocus();
    await userEvent.tab();
    const review = canvas.getByRole("button", { name: "Review" });
    await expect(review).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onReview).toHaveBeenCalledWith("turn");

    await userEvent.tab();
    const firstFile = canvas.getByRole("button", {
      name: "Review changes to runtime.ts",
    });
    await expect(firstFile).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onReview).toHaveBeenCalledTimes(2);
  },
};

export const WorkspaceFallback: Story = {
  render: ({ onReview }) => (
    <ChangesOutcomeFixture
      mode="workspace-fallback"
      onReview={onReview}
    />
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByText("Changes in workspace"),
    ).resolves.toBeVisible();
  },
};

export const RequestFailure: Story = {
  render: ({ onReview }) => (
    <ChangesOutcomeFixture
      mode="failure-then-ready"
      onReview={onReview}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.findByText("Couldn't load changes"),
    ).resolves.toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: /Retry/ }));
    await expect(canvas.findByText("Edited 6 files")).resolves.toBeVisible();
  },
};

const openImage = fn();

export const ImageCard: Story = {
  render: () => (
    <LeafFrame>
      <RenderImageCard
        sid={SID}
        path="qa/desktop-light.png"
        onOpen={openImage}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const card = canvas.getByRole("button", { name: "Open desktop-light.png" });
    const preview = canvasElement.querySelector("img");
    await expect(preview).toBeVisible();
    await expect(preview).toHaveAttribute("alt", "");
    await userEvent.click(card);
    await expect(openImage).toHaveBeenCalled();
  },
};

export const ImageArtifacts: Story = {
  render: () => (
    <LeafFrame>
      <RenderImageArtifacts sid={SID} files={imageFiles} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getAllByRole("button", { name: /^Open / })).toHaveLength(6);
    await userEvent.click(canvas.getByRole("button", { name: "Show 1 more" }));
    await expect(canvas.getAllByRole("button", { name: /^Open / })).toHaveLength(7);
    await expect(canvas.getByRole("button", { name: "Show less" })).toBeVisible();
  },
};

export const ImageLightboxOpen: Story = {
  render: () => (
    <LeafFrame>
      <RenderImageArtifacts sid={SID} files={imageFiles} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Open desktop-light.png" }),
    );
    const body = within(canvasElement.ownerDocument.body);
    const dialog = body.getByRole("dialog", { name: "Image viewer" });
    await expect(dialog).toBeVisible();
    await expect(body.getByText("1 / 6")).toBeVisible();
    await expect(
      body.getByRole("img", { name: "desktop-light.png" }),
    ).toBeVisible();
  },
};

export const ArtifactRow: Story = {
  render: () => (
    <LeafFrame>
      <div className="overflow-hidden rounded-[14px] border border-line bg-panel">
        <RenderArtifactRow
          sid={SID}
          file={documentFiles[1]}
          ext="pdf"
          label="PDF"
          divider={false}
        />
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Open a11y-results.pdf" });
    await userEvent.click(trigger);
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await expect(canvas.getByRole("menuitem", { name: "New tab" })).toBeVisible();
    await expect(canvas.getByRole("menuitem", { name: "Download" })).toBeVisible();
  },
};

export const ArtifactChips: Story = {
  render: () => (
    <LeafFrame>
      <RenderArtifactChips sid={SID} files={documentFiles} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("release-notes.md")).toBeVisible();
    await expect(canvas.queryByText("review-summary.docx")).not.toBeInTheDocument();
    await userEvent.click(canvas.getByRole("button", { name: "Show 1 more" }));
    await expect(canvas.getByText("review-summary.docx")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Show less" })).toBeVisible();
  },
};

export const ChangesShell: Story = {
  render: () => (
    <LeafFrame>
      <RenderChangesShell>
        <div className="changes-outcome-title">
          <b>Loading workspace changes</b>
          <span className="text-[13px] text-dim">Checking the last turn</span>
        </div>
      </RenderChangesShell>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("region", { name: "Workspace changes" })).toBeVisible();
    await expect(canvas.getByText("Checking the last turn")).toBeVisible();
  },
};
