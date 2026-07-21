// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// INC-41 DIFF-CP — the review's main action went missing on the screen the user
// actually lands on.
//
// Codex's review header ends in one outlined `⊸ Commit or push ⌄`: read the
// diff, commit it. Ours looked like that button — and then RVW-4 made
// `last-turn` the default scope while the button was still gated on
// `scope === "working-tree"`. On the default screen it rendered nowhere, and
// not in the `…` overflow either (Apply/Remove are working-tree-gated too, so
// the overflow held nothing but Refresh). The commit was never scope-dependent:
// AR.commit stages the workspace, the same workspace whichever diff you're
// reading.
//
// So: resident in every scope, disabled (not absent) when there is nothing to
// stage, and icon-only — never demoted back into a menu — on a narrow bar.

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
import { useStore } from "../store";
import type { DiffResp, DiffScope } from "../types";

const editDiff = `diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -1,2 +1,2 @@
-const a = 1;
+const a = 2;
 const b = 3;
`;

const baseDiff = (over: Partial<DiffResp> = {}): DiffResp => ({
  scope: "working-tree",
  workspace: "/tmp/ws",
  known: true,
  isRepo: true,
  diff: editDiff,
  numstat: "",
  untracked: [],
  ...over,
});

// The backend as it really answers a last-turn request (webui/meta.go): the CLI's
// own JSON, which carries scope/available/diff — and NOT isRepo/worktree. A
// button gated on any of those would be dead on the default scope.
const lastTurnResp = (over: Partial<DiffResp> = {}): DiffResp =>
  ({ scope: "last-turn", available: true, workspace: "/tmp/ws", known: true, diff: editDiff, untracked: [], ...over }) as DiffResp;

// The bar measures itself with a ResizeObserver, which jsdom does not implement.
// Stub it, and give the bar a real width to measure: the component reads
// `clientWidth` (0 for every element in jsdom, which is why it treats a
// zero-width bar as "unmeasurable → keep the label").
let restoreWidth: (() => void) | null = null;
const barWidth = (px: number) => {
  const proto = HTMLElement.prototype as any;
  const original = Object.getOwnPropertyDescriptor(proto, "clientWidth");
  Object.defineProperty(proto, "clientWidth", {
    configurable: true,
    get(this: HTMLElement) {
      return this.classList?.contains("diffbar") ? px : 0;
    },
  });
  (globalThis as any).ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  restoreWidth = () => {
    if (original) Object.defineProperty(proto, "clientWidth", original);
    else delete proto.clientWidth;
    delete (globalThis as any).ResizeObserver;
  };
};

beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  localStorage.clear();
  (window as any).matchMedia = () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  });
  useStore.setState({ toasts: [], prompt: null });
});
afterEach(() => {
  cleanup();
  restoreWidth?.();
  restoreWidth = null;
});

const commitBtn = () => screen.getByLabelText("Commit or push");

describe("Commit or push is the review's resident main action (INC-41 DIFF-CP)", () => {
  it("sits in the bar on the DEFAULT scope — the last turn, where it used to vanish", async () => {
    const diff = vi.fn((_sid: string, scope: DiffScope) =>
      Promise.resolve(scope === "last-turn" ? lastTurnResp() : baseDiff()),
    );
    arMock.diff = diff;
    const { container } = render(<DiffView sid="cp1" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    // The panel opened on the turn (RVW-4) — the scope that used to have no
    // commit affordance anywhere on screen.
    expect(diff).toHaveBeenCalledWith("cp1", "last-turn");
    expect(screen.getByLabelText("Change diff scope").textContent).toContain("Last Turn");

    const btn = commitBtn();
    expect(container.querySelector(".diffbar")!.contains(btn)).toBe(true);
    expect((btn as HTMLButtonElement).disabled).toBe(false);
    // First-class: a word, not just a glyph, at this width.
    expect(btn.textContent).toContain("Commit or push");
  });

  it("still sits there on the working tree", async () => {
    localStorage.setItem("ar.diff.scope", "working-tree");
    arMock.diff = () => Promise.resolve(baseDiff());
    const { container } = render(<DiffView sid="cp2" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(container.querySelector(".diffbar")!.contains(commitBtn())).toBe(true);
  });

  it("opens the same Commit / Commit & push / Push menu, and Push still pushes", async () => {
    const push = vi.fn(() => Promise.resolve({ branch: "main" }));
    arMock.diff = (_sid: string, scope: DiffScope) =>
      Promise.resolve(scope === "last-turn" ? lastTurnResp() : baseDiff());
    arMock.push = push;
    render(<DiffView sid="cp3" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    fireEvent.click(commitBtn());

    // One click from the bar to the whole existing menu — no `…` detour.
    expect(screen.getByText("Commit")).toBeTruthy();
    expect(screen.getByText("Commit & push")).toBeTruthy();
    expect(screen.getByText("Push")).toBeTruthy();

    fireEvent.click(screen.getByText("Push"));
    await waitFor(() => expect(push).toHaveBeenCalledWith("cp3"));
    await waitFor(() => expect(useStore.getState().toasts.map((t) => t.text)).toContain("pushed main"));
  });

  it("routes Commit through the existing message prompt (the commit logic is untouched)", async () => {
    arMock.diff = (_sid: string, scope: DiffScope) =>
      Promise.resolve(scope === "last-turn" ? lastTurnResp() : baseDiff());
    render(<DiffView sid="cp4" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    fireEvent.click(commitBtn());
    fireEvent.click(screen.getByText("Commit"));

    await waitFor(() => expect(useStore.getState().prompt?.title).toBe("Commit changes"));
  });

  it("stays put — disabled, not gone — when there is nothing to commit", async () => {
    arMock.diff = (_sid: string, scope: DiffScope) =>
      Promise.resolve(scope === "last-turn" ? lastTurnResp({ diff: "" }) : baseDiff({ diff: "" }));
    render(<DiffView sid="cp5" />);

    await waitFor(() => expect(screen.getByText("No changes this turn")).toBeTruthy());
    // The golden greys this button out; it does not remove it. A control that
    // disappears teaches the user it was never there.
    const btn = commitBtn() as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    // …and says why, rather than leaving a dead control unexplained.
    expect(btn.title).toContain("switch to Working tree");
  });

  it("is not duplicated in the … overflow", async () => {
    arMock.diff = (_sid: string, scope: DiffScope) =>
      Promise.resolve(scope === "last-turn" ? lastTurnResp() : baseDiff());
    render(<DiffView sid="cp6" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    fireEvent.click(screen.getByLabelText("More changes actions"));
    // One exit, not two: the overflow offers the low-frequency workspace actions.
    expect(screen.getByText("Refresh changes")).toBeTruthy();
    expect(screen.queryByText("Commit")).toBeNull();
    expect(screen.queryByText("Commit & push")).toBeNull();
  });
});

describe("…and it survives a narrow bar without deserting it (INC-41 DIFF-CP)", () => {
  it("drops its label for its glyph — still resident, still one click, never back into …", async () => {
    barWidth(520); // below BAR_TIGHT_PX
    arMock.diff = (_sid: string, scope: DiffScope) =>
      Promise.resolve(scope === "last-turn" ? lastTurnResp() : baseDiff());
    const { container } = render(<DiffView sid="cp7" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const btn = commitBtn();
    // Icon-only in the bar — not a menu item, not gone.
    expect(container.querySelector(".diffbar")!.contains(btn)).toBe(true);
    expect(btn.className).toMatch(/diff-commit-compact/);
    expect(btn.textContent).not.toContain("Commit or push");
    // The name it lost from the surface it keeps for anyone who can't see the glyph.
    expect(btn.getAttribute("aria-label")).toBe("Commit or push");
    // Still one click to the same menu.
    fireEvent.click(btn);
    expect(screen.getByText("Commit & push")).toBeTruthy();
  });

  it("keeps the label when the bar has room for it", async () => {
    barWidth(700); // above BAR_TIGHT_PX
    arMock.diff = (_sid: string, scope: DiffScope) =>
      Promise.resolve(scope === "last-turn" ? lastTurnResp() : baseDiff());
    render(<DiffView sid="cp8" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const btn = commitBtn();
    expect(btn.className).not.toMatch(/diff-commit-compact/);
    expect(btn.textContent).toContain("Commit or push");
  });

  // The width the resident button is paid for with. D4 already held that "split
  // needs room" — but it measured the *window*, so at a 1100px window it still
  // offered split view for a 415px panel (two ~190px columns of code). Measured
  // on the bar, that toggle stands down exactly where it was never usable, and
  // the 58px it gives back is what keeps DF-1's one-row contract intact.
  it("spends the useless split toggle on the button, and falls back to inline", async () => {
    barWidth(520);
    localStorage.setItem("ar.diff.scope", "working-tree");
    arMock.diff = () => Promise.resolve(baseDiff());
    const { container } = render(<DiffView sid="cp9" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(container.querySelector(".diff-viewtoggle")).toBeNull();
    // …and the view it would have toggled is the one a 520px bar can render.
    expect(container.querySelector(".fd-split")).toBeNull();
    // The bar keeps the control that matters.
    expect(container.querySelector(".diffbar")!.contains(commitBtn())).toBe(true);
  });

  it("keeps the split toggle on a bar with room for it", async () => {
    barWidth(700);
    localStorage.setItem("ar.diff.scope", "working-tree");
    arMock.diff = () => Promise.resolve(baseDiff());
    const { container } = render(<DiffView sid="cp10" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(container.querySelector(".diff-viewtoggle")).toBeTruthy();
  });
});

// INC-41 RD-8 — …and DIFF-CP stopped one control short.
//
// With the resident Commit pill back on the bar (150px that never shrinks), a
// 339px panel — a 1024px window — still needed 367px of controls. Every one of
// them is `flex: 0 0 auto`, so flexbox does not arbitrate: the row simply
// overflows, and what hangs off the end is whatever is last. That was the ✕,
// measured at x=1051.9 against a panel whose right edge is 1024
// (qa/runs/2026-07-12-r33/after-rd89/before.json) — the user could read the diff
// and had no way to close it.
//
// The bar has to be short enough. So below BAR_TIGHT_PX the low-frequency
// controls (Copy, Wrap — and the split toggle, above) move into the `…` they
// were always candidates for, and the ✕ keeps its 28px.
describe("the ✕ never leaves the panel (INC-41 RD-8)", () => {
  const bar = (c: HTMLElement) => c.querySelector(".diffbar")!;

  it("demotes Copy and Wrap into … on a tight bar — and keeps the ✕ last", async () => {
    barWidth(520); // below BAR_TIGHT_PX
    localStorage.setItem("ar.diff.scope", "working-tree");
    arMock.diff = () => Promise.resolve(baseDiff());
    const { container } = render(<DiffView sid="rd8a" onClose={() => {}} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    // Off the bar…
    expect(screen.queryByLabelText("Copy diff")).toBeNull();
    expect(screen.queryByLabelText("Wrap long lines")).toBeNull();
    expect(container.querySelector(".diff-viewtoggle")).toBeNull();
    // …but not gone: same two actions, one click away in the overflow.
    fireEvent.click(screen.getByLabelText("More changes actions"));
    expect(screen.getByText("Wrap long lines")).toBeTruthy();
    expect(screen.getByText("Copy diff")).toBeTruthy();

    // The exit is the last thing on the bar, and it is still there.
    const close = screen.getByLabelText("Close changes");
    expect(bar(container).contains(close)).toBe(true);
    expect(bar(container).lastElementChild).toBe(close);
  });

  it("the demoted Wrap is the same switch, not a second one", async () => {
    barWidth(520);
    localStorage.setItem("ar.diff.scope", "working-tree");
    arMock.diff = () => Promise.resolve(baseDiff());
    const { container } = render(<DiffView sid="rd8b" onClose={() => {}} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(container.querySelector(".diffwrap")!.className).not.toMatch(/diff-wrap\b/);

    fireEvent.click(screen.getByLabelText("More changes actions"));
    fireEvent.click(screen.getByText("Wrap long lines"));
    // It flips the panel's wrap state and persists it, exactly as the bar button does.
    await waitFor(() => expect(container.querySelector(".diffwrap")!.className).toMatch(/diff-wrap\b/));
    expect(localStorage.getItem("ar.diff.wrap")).toBe("1");
    // …and now offers the reverse.
    fireEvent.click(screen.getByLabelText("More changes actions"));
    expect(screen.getByText("Disable line wrap")).toBeTruthy();
  });

  it("keeps Copy and Wrap resident — and out of … — on a bar with room", async () => {
    barWidth(700); // above BAR_TIGHT_PX
    localStorage.setItem("ar.diff.scope", "working-tree");
    arMock.diff = () => Promise.resolve(baseDiff());
    const { container } = render(<DiffView sid="rd8c" onClose={() => {}} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(bar(container).contains(screen.getByLabelText("Copy diff"))).toBe(true);
    expect(bar(container).contains(screen.getByLabelText("Wrap long lines"))).toBe(true);
    fireEvent.click(screen.getByLabelText("More changes actions"));
    // No duplicate doors: a control that is on the bar is not also in the menu.
    expect(screen.queryByText("Copy diff")).toBeNull();
    expect(screen.getByText("Refresh changes")).toBeTruthy();
    expect(screen.getByLabelText("Close changes")).toBeTruthy();
  });
});

describe("narrow reviews use the full code width", () => {
  it("runs tracked and untracked file sections edge-to-edge at 390px", async () => {
    barWidth(390);
    (window as any).matchMedia = (query: string) => ({
      matches: query === "(max-width: 900px)",
      addEventListener: () => {},
      removeEventListener: () => {},
    });
    localStorage.setItem("ar.diff.scope", "working-tree");
    arMock.diff = () => Promise.resolve(baseDiff({ untracked: ["assets/logo.png"] }));
    const { container } = render(<DiffView sid="mobile-edge" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const cards = [...container.querySelectorAll("details.filediff")];
    expect(cards).toHaveLength(2);
    expect(cards.some((card) => card.classList.contains("filediff-untracked"))).toBe(true);
    for (const card of cards) {
      expect(card.className).toContain("!m-0");
      expect(card.className).toContain("!rounded-none");
      expect(card.className).toContain("!border-x-0");
    }
  });

  it("keeps desktop cards framed when the split review rail is 604px", async () => {
    barWidth(604);
    localStorage.setItem("ar.diff.scope", "working-tree");
    arMock.diff = () => Promise.resolve(baseDiff({ untracked: ["assets/logo.png"] }));
    const { container } = render(<DiffView sid="desktop-card" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const cards = [...container.querySelectorAll("details.filediff")];
    expect(cards).toHaveLength(2);
    for (const card of cards) {
      expect(card.className).not.toContain("!m-0");
      expect(card.className).not.toContain("!rounded-none");
      expect(card.className).not.toContain("!border-x-0");
    }
  });
});
