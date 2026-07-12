// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";

// INC-41 L4 — the Changes panel's "nothing to show" used to be a bare grey
// sentence while the timeline's empty state was a full icon + title + guidance
// card. These tests pin the shared spec so the panel can't regress to a
// one-liner: an icon renders, both copy lines render, and none of it shows up
// once there are real changes to review.

const { arMock } = vi.hoisted(() => ({ arMock: {} as Record<string, (...args: any[]) => any> }));
vi.mock("../api", async () => ({
  // the real module's helpers (isBinaryPath, ApiError, …) stay real — only the
  // network surface `AR` is stubbed.
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy(
    {},
    {
      get: (_target, prop: string) => (...args: any[]) =>
        arMock[prop] ? arMock[prop](...args) : new Promise(() => {}),
    },
  ),
  uploadURL: (path: string) => path,
  diffPath: () => "",
}));

import { DiffView } from "./DiffView";
import type { DiffResp } from "../types";

const baseDiff = (over: Partial<DiffResp> = {}): DiffResp => ({
  scope: "working-tree",
  workspace: "/tmp/ws",
  known: true,
  isRepo: true,
  diff: "",
  numstat: "",
  untracked: [],
  ...over,
});

const oneFileDiff = `diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -1,2 +1,2 @@
-const a = 1;
+const a = 2;
 const b = 3;
`;

beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  // INC-41 RVW-4 · the default scope is `last-turn` now; this file pins the
  // working tree's empty state, so it says so explicitly (the last turn's own
  // empty state — "No changes this turn" — is pinned in DiffView.review.test).
  localStorage.setItem("ar.diff.scope", "working-tree");
  // DiffView reads matchMedia on mount to decide inline vs split; jsdom has none.
  (window as any).matchMedia = () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  });
});
afterEach(cleanup);

describe("Changes panel empty state (INC-41 L4)", () => {
  it("renders icon + title + guidance when the workspace has no changes", async () => {
    arMock.diff = () => Promise.resolve(baseDiff());
    const { container } = render(<DiffView sid="s1" />);

    await waitFor(() => expect(screen.getByText("No changes yet")).toBeTruthy());
    expect(screen.getByText("Edits the agent makes to the workspace will show up here.")).toBeTruthy();

    const card = container.querySelector(".diff-empty");
    expect(card).toBeTruthy();
    // phosphor renders an <svg>; its absence is exactly the old bald state.
    expect(card!.querySelector("svg")).toBeTruthy();
  });

  it("shows the file list and no empty state when there are changes", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: oneFileDiff }));
    const { container } = render(<DiffView sid="s2" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(container.querySelector(".diff-empty")).toBeNull();
    expect(screen.queryByText("No changes yet")).toBeNull();
    expect(container.querySelectorAll(".filediff").length).toBe(1);
  });

  it("keeps untracked-only workspaces out of the empty state", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ untracked: ["new.md"] }));
    const { container } = render(<DiffView sid="s3" />);

    await waitFor(() => expect(screen.getByText("new.md")).toBeTruthy());
    expect(container.querySelector(".diff-empty")).toBeNull();
  });
});
