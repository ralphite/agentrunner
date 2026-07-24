// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// DIFF-SPLIT-FOLD-BAND — the split (side-by-side) view used to render each hunk
// boundary as a bare `.dl-hunk` section heading: no "N unmodified lines" count
// and no caret, so a reader in split view could neither see how much context was
// folded nor expand it. The inline view has always shown Codex's collapsible
// band there. These tests pin that split view now renders the same `.fd-gap`
// band (count + caret), keyed back to the shared gap map by the hunk's row index.

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

// A hunk that starts at line 5 → lines 1-4 are a leading gap (4 unmodified lines
// with a known end, so the band renders its exact count without a blob fetch).
const gappedDiff = `diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -5,3 +5,3 @@
 const b = 3;
-const c = 1;
+const c = 2;
 const d = 4;
`;

// Not narrow (a real desktop window); split is allowed.
const wideMatchMedia = () => ({
  matches: false,
  addEventListener: () => {},
  removeEventListener: () => {},
});

// jsdom reports clientWidth 0; report a comfortable panel width so nothing
// demotes and the resident inline/split toggle is on the bar.
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
  for (const key of Object.keys(arMock)) delete arMock[key];
  localStorage.setItem("ar.diff.scope", "working-tree");
  (window as any).matchMedia = wideMatchMedia;
  (window as any).ResizeObserver = ResizeObserverStub;
  Object.defineProperty(HTMLDivElement.prototype, "clientWidth", {
    configurable: true,
    get: () => 900,
  });
});
afterEach(() => {
  cleanup();
  delete (HTMLDivElement.prototype as any).clientWidth;
  delete (window as any).ResizeObserver;
});

async function renderSplit(sid: string) {
  arMock.diff = () => Promise.resolve(baseDiff({ diff: gappedDiff }));
  const view = render(<DiffView sid={sid} />);
  await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
  // Switch to split view (resident toggle or the … overflow both expose it).
  fireEvent.click(screen.getByLabelText("Split view"));
  await waitFor(() => expect(view.container.querySelector(".fd-body.fd-split")).toBeTruthy());
  return view;
}

describe("Split view fold band (DIFF-SPLIT-FOLD-BAND)", () => {
  it("renders the collapsible N-unmodified-lines band inside the split grid", async () => {
    const { container } = await renderSplit("sf1");

    // The band lives in the split body — not a bare .dl-hunk heading anymore.
    const band = container.querySelector(".fd-body.fd-split .fd-gap");
    expect(band).toBeTruthy();
    // It states the exact count (leading gap = the 4 lines before the hunk) and
    // carries the caret the inline band has, so it reads as an expandable fold.
    expect(band!.textContent).toContain("4 unmodified lines");
    expect(band!.querySelector(".fd-gap-caret")).toBeTruthy();
    // It is a real control (button), i.e. clickable to reveal — the affordance
    // split view was missing entirely.
    expect((band as HTMLElement).tagName).toBe("BUTTON");
  });

  it("keeps rendering the side-by-side rows alongside the band", async () => {
    const { container } = await renderSplit("sf2");
    // The changed pair still renders as split rows (old left / new right).
    expect(container.querySelector(".fd-body.fd-split .dls-half.del")).toBeTruthy();
    expect(container.querySelector(".fd-body.fd-split .dls-half.add")).toBeTruthy();
  });
});
