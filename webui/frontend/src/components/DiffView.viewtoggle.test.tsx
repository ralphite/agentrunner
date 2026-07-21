// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// DIFF-SPLIT-TOGGLE-GONE — the inline/split toggle used to be gated on
// `!barTight`, so on every mainstream laptop width (1280–1512 → a 538–635px
// diff panel, all < BAR_TIGHT_PX=640) the toggle vanished entirely and the user
// could not reach split view at all — while Wrap/Copy demoted gracefully into
// the `…` overflow beside it. Codex keeps the view switch reachable at these
// widths. These tests pin the fix: the toggle demotes into `…` on a tight bar
// (never vanishes), `barTight` no longer forces inline, and the `!narrow`
// availability guard still refuses split for a truly narrow (≤900px) window.

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

// jsdom has no ResizeObserver and reports clientWidth 0, so the bar's measure()
// can never see a tight panel there (see BAR_TIGHT_PX's comment). We stand in a
// stub that runs measure() at observe-time and a clientWidth getter that reports
// a mid-width (605px < 640) panel — the 1440-laptop case the fix is about.
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

// matches:false for every breakpoint → not narrow (a real desktop window whose
// diff panel is nonetheless < 640). Individual tests override to go narrow.
const wideMatchMedia = () => ({
  matches: false,
  addEventListener: () => {},
  removeEventListener: () => {},
});

beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  localStorage.setItem("ar.diff.scope", "working-tree");
  (window as any).matchMedia = wideMatchMedia;
  (window as any).ResizeObserver = ResizeObserverStub;
  barWidth = 605;
  Object.defineProperty(HTMLDivElement.prototype, "clientWidth", {
    configurable: true,
    get: () => barWidth,
  });
});
afterEach(() => {
  cleanup();
  delete (HTMLDivElement.prototype as any).clientWidth;
  delete (window as any).ResizeObserver;
});

// A sanity anchor: the stub really does put the panel into the tight state that
// used to hide the toggle — otherwise these tests would pass vacuously.
async function renderTight(sid: string) {
  arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff }));
  const view = render(<DiffView sid={sid} />);
  await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
  // barTight demoted Copy/Wrap out of the bar, proving the tight state is live.
  await waitFor(() =>
    expect(view.container.querySelector(".diffbar .diff-wrap-btn")).toBeNull(),
  );
  return view;
}

describe("Split toggle stays reachable on a tight bar (DIFF-SPLIT-TOGGLE-GONE)", () => {
  it("demotes the inline/split toggle into the … overflow instead of hiding it", async () => {
    const { container } = await renderTight("vt1");

    // The resident toggle group is gone on a tight bar…
    expect(container.querySelector(".diff-viewtoggle")).toBeNull();

    // …but the view switch is not: it is one item in `…`, pointing at split.
    fireEvent.click(screen.getByLabelText("More changes actions"));
    const item = screen.getByText("Split view");
    expect(item).toBeTruthy();
    expect(screen.getByText("Show old and new side by side")).toBeTruthy();
  });

  it("switches the review to split view when the … item is clicked", async () => {
    const { container } = await renderTight("vt2");
    // Default view is inline: no side-by-side body yet.
    expect(container.querySelector(".fd-body.fd-split")).toBeNull();

    fireEvent.click(screen.getByLabelText("More changes actions"));
    fireEvent.click(screen.getByText("Split view"));

    // barTight no longer forces inline (effView === view now), so a modified
    // file renders its side-by-side body — the thing that was unreachable.
    await waitFor(() => expect(container.querySelector(".fd-body.fd-split")).toBeTruthy());

    // And the item now points the other way, like the Wrap toggle does.
    fireEvent.click(screen.getByLabelText("More changes actions"));
    expect(screen.getByText("Inline view")).toBeTruthy();
    expect(screen.queryByText("Split view")).toBeNull();
  });

  it("omits the toggle item when the window itself is too narrow for two columns", async () => {
    // ≤900px window → narrow → split is genuinely refused (mirrors the resident
    // split button's disabled={narrow}); the … item stands down with it.
    (window as any).matchMedia = (query: string) => ({
      matches: query === "(max-width: 900px)" || query === "(max-width: 680px)",
      addEventListener: () => {},
      removeEventListener: () => {},
    });
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff }));
    render(<DiffView sid="vt3" />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());

    fireEvent.click(screen.getByLabelText("More changes actions"));
    // Wrap/Copy still demote here (they don't need width), but the view switch
    // does not — there is nothing to switch to on a phone-width window.
    expect(screen.queryByText("Split view")).toBeNull();
    expect(screen.queryByText("Inline view")).toBeNull();
    // A modified file stays inline no matter what `view` says.
    expect(document.querySelector(".fd-body.fd-split")).toBeNull();
  });
});
