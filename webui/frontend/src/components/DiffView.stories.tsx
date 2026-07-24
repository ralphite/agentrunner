import { useState, type ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import { parseFileDiff } from "../diffSummary";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import type { DiffResp, DiffScope } from "../types";
import {
  DiffView,
  FileBody as RenderFileBody,
  FileHead as RenderFileHead,
  UntrackedFile as RenderUntrackedFile,
} from "./DiffView";

type StoryApi = AppServices["api"];
type FixtureMode = "ready" | "request-failure" | "workspace-unavailable";

const SID = "story-diff-view";

const reviewDiff = `diff --git a/src/app/runtime.ts b/src/app/runtime.ts
--- a/src/app/runtime.ts
+++ b/src/app/runtime.ts
@@ -1,4 +1,5 @@
 export function startRuntime() {
-  return boot("legacy");
+  return boot("storybook");
+  // ${"A deliberately long line demonstrates horizontal overflow without losing the close control. ".repeat(3)}
 }
diff --git a/src/storybook/scenarios/coreSession.ts b/src/storybook/scenarios/coreSession.ts
new file mode 100644
--- /dev/null
+++ b/src/storybook/scenarios/coreSession.ts
@@ -0,0 +1,4 @@
+export const steps = [
+  "open",
+  "send",
+];
diff --git a/src/components/ScenarioControls.tsx b/src/components/ScenarioControls.tsx
--- a/src/components/ScenarioControls.tsx
+++ b/src/components/ScenarioControls.tsx
@@ -8,3 +8,3 @@
-export const speed = 1;
+export const speed = 2;
`;

const readyDiff: DiffResp = {
  scope: "working-tree",
  workspace: "/worktrees/storybook-agent-runner",
  known: true,
  isRepo: true,
  nested: false,
  diff: reviewDiff,
  numstat: "",
  untracked: ["notes/review checklist.txt"],
  hiddenUntracked: 17,
  conflicts: ["src/components/ScenarioControls.tsx"],
  branch: "storybook-component-system-with-a-long-branch-name",
};

const unavailableDiff: DiffResp = {
  scope: "working-tree",
  workspace: "",
  known: false,
  isRepo: false,
  diff: "",
  numstat: "",
  untracked: [],
};

const directBody = parseFileDiff([
  "index 2b728b1..1f0677a 100644",
  "--- a/src/runtime.ts",
  "+++ b/src/runtime.ts",
  "@@ -5,3 +5,4 @@ export function startRuntime()",
  "   const services = createServices();",
  '-  return boot("legacy");',
  '+  return boot("storybook");',
  "+  return services.start();",
]);

const directBlob = [
  "import { createServices } from './services';",
  "",
  "export interface Runtime {",
  "  start(): void;",
  "}",
  "export function startRuntime() {",
  "  const services = createServices();",
  '  return boot("storybook");',
  "  return services.start();",
  "}",
  "",
  "export const runtime = startRuntime();",
];

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

function DiffViewFixture({
  mode = "ready",
  onClose,
}: {
  mode?: FixtureMode;
  onClose?: () => void;
}) {
  const [api] = useState(() =>
    isolatedApi({
      diff: fn(async (_sid: string, _scope: DiffScope = "working-tree") => {
        if (mode === "request-failure") {
          throw new Error("The in-memory diff fixture is unavailable");
        }
        return mode === "workspace-unavailable" ? unavailableDiff : readyDiff;
      }),
      blob: fn(async () => ({
        lines: [
          "Review checklist",
          "- validate keyboard order",
          "- validate overflow",
        ],
      })),
      commit: fn(async () => ({ status: "ok" })),
      push: fn(async () => ({ status: "ok", branch: "storybook" })),
      gitInit: fn(async () => ({ status: "ok" })),
      applyWorktree: fn(async () => ({
        status: "ok",
        mainRepo: "/projects/agentrunner",
        applied: "storybook.patch",
      })),
      removeWorktree: fn(async () => ({
        status: "ok",
        mainRepo: "/projects/agentrunner",
      })),
    }),
  );

  return (
    <StoryAppFrame
      initialState={{ currentSid: SID }}
      services={{
        api,
        local: {
          "ar.diff.scope": "working-tree",
        },
      }}
    >
      <div className="session-view h-[720px] min-h-[520px]">
        <aside className="changes-panel session-side flex h-full min-w-0 flex-col overflow-hidden">
          <DiffView
            sid={SID}
            initialScope="working-tree"
            onClose={onClose}
          />
        </aside>
      </div>
    </StoryAppFrame>
  );
}

function LeafFrame({ children }: { children: ReactNode }) {
  const [api] = useState(() =>
    isolatedApi({
      blob: fn(async () => ({ lines: directBlob })),
    }),
  );
  return (
    <StoryAppFrame
      initialState={{ currentSid: SID }}
      services={{ api }}
    >
      <main className="mx-auto grid w-full max-w-[920px] gap-5 p-6">
        {children}
      </main>
    </StoryAppFrame>
  );
}

function PendingLeafFrame({ children }: { children: ReactNode }) {
  const [api] = useState(() =>
    isolatedApi({
      blob: fn(
        () =>
          new Promise<{ lines: string[] }>(() => {
            // Intentionally pending so the loading state remains deterministic.
          }),
      ),
    }),
  );
  return (
    <StoryAppFrame initialState={{ currentSid: SID }} services={{ api }}>
      <main className="mx-auto grid w-full max-w-[920px] gap-5 p-6">
        {children}
      </main>
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Changes/DiffView",
  component: DiffView,
  parameters: {
    layout: "fullscreen",
  },
  args: {
    sid: SID,
    initialScope: "working-tree",
    onClose: fn(),
  },
  render: ({ onClose }) => <DiffViewFixture onClose={onClose} />,
} satisfies Meta<typeof DiffView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.findByText("runtime.ts")).resolves.toBeVisible();
    await expect(canvas.getByRole("button", { name: "Changed files" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Close changes" })).toBeVisible();
  },
};

export const KeyboardNavigation: Story = {
  args: {
    onClose: fn(),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("runtime.ts");
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    const scope = canvas.getByRole("button", { name: "Change diff scope" });
    await expect(scope).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(scope).toHaveAttribute("aria-expanded", "true");
    await userEvent.keyboard("{Escape}");
    await expect(scope).toHaveFocus();

    await userEvent.tab();
    const more = canvas.getByRole("button", {
      name: "More changes actions",
    });
    await expect(more).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(more).toHaveAttribute("aria-expanded", "true");
  },
};

export const RequestFailure: Story = {
  render: ({ onClose }) => (
    <DiffViewFixture mode="request-failure" onClose={onClose} />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.findByText("Couldn’t load changes"),
    ).resolves.toBeVisible();
    await expect(canvas.getByRole("button", { name: "Try again" })).toBeVisible();
  },
};

export const WorkspaceUnavailable: Story = {
  render: ({ onClose }) => (
    <DiffViewFixture mode="workspace-unavailable" onClose={onClose} />
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByText("Workspace unavailable"),
    ).resolves.toBeVisible();
  },
};

export const FileHead: Story = {
  render: () => (
    <LeafFrame>
      <details className="filediff" open>
        <RenderFileHead
          path="src/components/ScenarioControls.tsx"
          status="modified"
          add={18}
          del={4}
          badges={["mode changed", "conflict"]}
        />
      </details>
      <details className="filediff" open>
        <RenderFileHead
          path="assets/demo-recording.mp4"
          status="added"
          add={null}
          del={0}
          badges={["binary"]}
        />
      </details>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("ScenarioControls.tsx")).toBeVisible();
    await expect(canvas.getByText("+18")).toBeVisible();
    await expect(canvas.getByText("-4")).toBeVisible();
    await expect(canvas.getByText("conflict")).toBeVisible();
    await expect(canvas.getByText("binary")).toBeVisible();
    await expect(canvas.queryByText("+…")).not.toBeInTheDocument();
  },
};

export const UntrackedFile: Story = {
  render: () => (
    <LeafFrame>
      <RenderUntrackedFile
        sid={SID}
        path="notes/review-checklist.txt"
        effView="inline"
        defaultOpen
        prefetch={false}
        edgeToEdge={false}
      />
      <RenderUntrackedFile
        sid={SID}
        path="artifacts/demo-recording.mp4"
        effView="inline"
        defaultOpen
        prefetch={false}
        knownReason="binary"
        edgeToEdge={false}
      />
      <RenderUntrackedFile
        sid={SID}
        path="artifacts/full-test-recording.mov"
        effView="inline"
        defaultOpen
        prefetch={false}
        knownReason="large"
        edgeToEdge={false}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.findByText("import { createServices } from './services';")).resolves.toBeVisible();
    await expect(
      canvas.getByText("Content isn’t shown — this file is binary."),
    ).toBeVisible();
    await expect(
      canvas.getByText("Content isn’t shown — this file is too large to display."),
    ).toBeVisible();
    await expect(canvas.getByText("binary")).toBeVisible();
    await expect(canvas.getByText("large")).toBeVisible();
  },
};

export const UntrackedFileLoading: Story = {
  render: () => (
    <PendingLeafFrame>
      <RenderUntrackedFile
        sid={SID}
        path="notes/pending-review.txt"
        effView="inline"
        defaultOpen
        prefetch={false}
        edgeToEdge={false}
      />
    </PendingLeafFrame>
  ),
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByText("Loading…")).toBeVisible();
  },
};

export const FileBody: Story = {
  render: () => (
    <LeafFrame>
      <section aria-label="Inline diff">
        <RenderFileBody
          sid={SID}
          path="src/runtime.ts"
          parsed={directBody}
          lang="ts"
          effView="inline"
          hunkCount={1}
          prefetch
        />
      </section>
      <section aria-label="Split diff">
        <RenderFileBody
          sid={SID}
          path="src/runtime.ts"
          parsed={directBody}
          lang="ts"
          effView="split"
          hunkCount={1}
          prefetch={false}
        />
      </section>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("region", { name: "Inline diff" })).toBeVisible();
    await expect(canvas.getByRole("region", { name: "Split diff" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: /4 unmodified lines/ })).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: /4 unmodified lines/ }));
    await expect(canvas.getByText("export interface Runtime {")).toBeVisible();
    await expect(canvas.getAllByText('return boot("storybook");')).toHaveLength(2);
  },
};

export const FileBodyContextLoading: Story = {
  render: () => (
    <PendingLeafFrame>
      <RenderFileBody
        sid={SID}
        path="src/runtime.ts"
        parsed={directBody}
        lang="ts"
        effView="inline"
        hunkCount={1}
        prefetch={false}
      />
    </PendingLeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const gap = canvas.getByRole("button", {
      name: /unmodified lines to end of file/,
    });
    await userEvent.click(gap);
    await expect(gap).toBeDisabled();
    await expect(gap).toHaveTextContent("Loading…");
  },
};
