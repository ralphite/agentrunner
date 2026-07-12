// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// INC-41 RV-1/RV-3/RV-5 — the Changes rail's chrome. The panel used to spend
// ~110px above the first diff line (a `Changes ✕` title bar that repeated the
// topbar pill, plus a toolbar that wrapped to two rows in a worktree session),
// folded files gave no hint they were folded, and each file header spent its
// width on a `new file` badge that its green `A` glyph already said.
//
// These tests pin the shape: one toolbar (the low-frequency worktree actions
// live behind `…`), a close affordance inside it, a disclosure caret per file,
// and no badge that duplicates the status glyph.

const { arMock } = vi.hoisted(() => ({ arMock: {} as Record<string, (...args: any[]) => any> }));
vi.mock("../api", () => ({
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

const editDiff = `diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -1,2 +1,2 @@
-const a = 1;
+const a = 2;
 const b = 3;
`;

const newFileDiff = `diff --git a/notes.md b/notes.md
new file mode 100644
--- /dev/null
+++ b/notes.md
@@ -0,0 +1,1 @@
+hello
`;

const worktreeDiff = (over: Partial<DiffResp> = {}) =>
  baseDiff({ diff: editDiff, worktree: true, mainRepo: "/repos/agentrunner", branch: "wt-1", ...over });

beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  (window as any).matchMedia = () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  });
});
afterEach(cleanup);

describe("Changes toolbar (INC-41 RV-1)", () => {
  it("keeps Apply / Remove / Refresh out of the bar and behind the … overflow", async () => {
    arMock.diff = () => Promise.resolve(worktreeDiff());
    const { container } = render(<DiffView sid="s1" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const bar = container.querySelector(".diffbar")!;
    expect(bar).toBeTruthy();
    // Nothing in the bar itself says "Apply"/"Remove"/"Refresh" — those wrapped
    // it onto a second row in every worktree session.
    expect(bar.textContent).not.toMatch(/Apply|Remove|Refresh/);

    fireEvent.click(screen.getByLabelText("More changes actions"));
    expect(screen.getByText("Refresh changes")).toBeTruthy();
    expect(screen.getByText("Apply to project…")).toBeTruthy();
    expect(screen.getByText("Remove worktree…")).toBeTruthy();
  });

  it("omits the worktree-only actions for a plain (non-worktree) workspace", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff }));
    render(<DiffView sid="s2" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    fireEvent.click(screen.getByLabelText("More changes actions"));
    expect(screen.getByText("Refresh changes")).toBeTruthy();
    expect(screen.queryByText("Apply to project…")).toBeNull();
    expect(screen.queryByText("Remove worktree…")).toBeNull();
  });

  it("carries the panel's close affordance now that the title bar is gone", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff }));
    const onClose = vi.fn();
    const { container } = render(<DiffView sid="s3" onClose={onClose} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const close = screen.getByLabelText("Close changes");
    expect(container.querySelector(".diffbar")!.contains(close)).toBe(true);
    fireEvent.click(close);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("offers the close affordance from the states that render before any diff", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ known: false }));
    const onClose = vi.fn();
    render(<DiffView sid="s4" onClose={onClose} />);

    await waitFor(() => expect(screen.getByText("Workspace unavailable")).toBeTruthy());
    fireEvent.click(screen.getByLabelText("Close changes"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("File headers (INC-41 RV-3 / RV-5)", () => {
  it("gives every file a disclosure caret", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff }));
    const { container } = render(<DiffView sid="s5" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const head = container.querySelector("details.filediff > summary.fd-head")!;
    expect(head.querySelector(".fd-caret")).toBeTruthy();
    expect(head.querySelector(".fd-caret svg")).toBeTruthy();
  });

  it("drops the badge the status glyph already states", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: newFileDiff }));
    const { container } = render(<DiffView sid="s6" />);

    await waitFor(() => expect(screen.getByText("notes.md")).toBeTruthy());
    // The glyph still says "added"…
    expect(container.querySelector(".fd-glyph-added")!.textContent).toBe("A");
    // …so the redundant right-edge badge is gone (it was squeezing the path).
    expect(screen.queryByText("new file")).toBeNull();
    expect(container.querySelectorAll(".fd-badge").length).toBe(0);
  });
});
