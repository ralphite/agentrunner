// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";

// DIFF-COLLAPSE-ALL-ICON — the review's collapse-all/expand-all control was fully
// wired (onToggleAll / allShownOpen) but rendered ONLY inside the `…` overflow
// menu, at every width — unlike Copy/Wrap/Split, which are first-class toolbar
// icons on a wide bar and only demote into `…` when the bar is tight. Codex's
// Review toolbar shows a dedicated one-click collapse-all icon. These tests pin
// the fix: on a WIDE multi-file review the control is a resident toolbar icon
// (not buried in `…`), and it still demotes into `…` on a TIGHT bar.

const { arMock } = vi.hoisted(() => ({ arMock: {} as Record<string, (...args: any[]) => any> }));
vi.mock("../api", async () => ({
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

const wideMatchMedia = () => ({
  matches: false,
  addEventListener: () => {},
  removeEventListener: () => {},
});

beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  localStorage.setItem("ar.diff.scope", "working-tree");
  (window as any).matchMedia = wideMatchMedia;
});
afterEach(cleanup);

// --- WIDE bar: no ResizeObserver stub → jsdom clientWidth 0 → barTight false ---
describe("Collapse-all is a first-class toolbar icon on a wide review (DIFF-COLLAPSE-ALL-ICON)", () => {
  it("renders the collapse-all button directly in the toolbar, not in the … overflow", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff + newFileDiff }));
    const { container } = render(<DiffView sid="ca-wide-1" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    // Files start expanded, so the action is "collapse".
    const btn = screen.getByLabelText("Collapse all files");
    expect(container.querySelector(".diffbar")!.contains(btn)).toBe(true);

    // …and it is NOT duplicated inside the `…` menu on a wide bar (that PopItem
    // now only appears when the bar is tight, like Copy/Wrap/Split).
    fireEvent.click(screen.getByLabelText("More changes actions"));
    expect(screen.queryByText("Fold every file down to its header")).toBeNull();
    fireEvent.keyDown(document, { key: "Escape" });
  });

  it("toggles its icon/label between collapse and expand and folds every file", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff + newFileDiff }));
    const { container } = render(<DiffView sid="ca-wide-2" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    // Both files open by default.
    await waitFor(() => expect(container.querySelectorAll("details.filediff[open]").length).toBe(2));

    fireEvent.click(screen.getByLabelText("Collapse all files"));
    // onToggleAll folded every file…
    await waitFor(() => expect(container.querySelector("details.filediff[open]")).toBeNull());
    // …and the resident icon flips to the expand action.
    const expandBtn = screen.getByLabelText("Expand all files");
    expect(container.querySelector(".diffbar")!.contains(expandBtn)).toBe(true);

    fireEvent.click(expandBtn);
    await waitFor(() => expect(container.querySelectorAll("details.filediff[open]").length).toBe(2));
  });

  it("shows no collapse-all control for a single-file review", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff }));
    render(<DiffView sid="ca-single" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(screen.queryByLabelText("Collapse all files")).toBeNull();
    expect(screen.queryByLabelText("Expand all files")).toBeNull();
    // …and not in the menu either.
    fireEvent.click(screen.getByLabelText("More changes actions"));
    expect(screen.queryByText("Fold every file down to its header")).toBeNull();
  });
});

// --- TIGHT bar: ResizeObserver stub + clientWidth 605 (< BAR_TIGHT_PX=640) ---
describe("Collapse-all demotes into the … overflow on a tight bar (DIFF-COLLAPSE-ALL-ICON)", () => {
  let barWidth = 605;
  class ResizeObserverStub {
    cb: () => void;
    constructor(cb: () => void) {
      this.cb = cb;
    }
    observe() {
      this.cb();
    }
    unobserve() {}
    disconnect() {}
  }

  beforeEach(() => {
    (window as any).ResizeObserver = ResizeObserverStub;
    barWidth = 605;
    Object.defineProperty(HTMLDivElement.prototype, "clientWidth", {
      configurable: true,
      get: () => barWidth,
    });
  });
  afterEach(() => {
    delete (HTMLDivElement.prototype as any).clientWidth;
    delete (window as any).ResizeObserver;
  });

  it("keeps collapse-all out of the tight toolbar but reachable in the … menu", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff + newFileDiff }));
    const { container } = render(<DiffView sid="ca-tight-1" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    // Sanity: the tight state is live — Copy/Wrap demoted out of the bar too.
    await waitFor(() =>
      expect(container.querySelector(".diffbar .diff-wrap-btn")).toBeNull(),
    );
    // The resident collapse-all icon is NOT in the tight toolbar (the popover is
    // closed, so no menu PopItem is rendered yet either).
    expect(screen.queryByLabelText("Collapse all files")).toBeNull();

    // …but it is one item in `…`, unchanged reachability.
    fireEvent.click(screen.getByLabelText("More changes actions"));
    const menu = document.querySelector(".diff-more-menu") as HTMLElement;
    expect(within(menu).getByText("Collapse all files")).toBeTruthy();
    expect(within(menu).getByText("Fold every file down to its header")).toBeTruthy();
  });
});
