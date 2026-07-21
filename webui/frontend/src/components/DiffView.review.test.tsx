// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// INC-41 RVW-3 / RVW-4 / RVW-6 — three ways the review rail fell short of the
// Codex golden:
//
//  RVW-3 · no copy affordance anywhere in the panel (bar, `…`, per file) — while
//          every fenced code block in the conversation next to it has one. The
//          only way to get a diff you'd just read into an issue was dragging a
//          selection across a virtualized grid.
//  RVW-4 · the panel opened on the working tree while the thread's change card
//          (and its `Review` link into this panel) counts the LAST TURN: one
//          click, two different diffs. Codex defaults to the turn.
//  RVW-6 · it loaded as a single grey sentence, in an app whose sidebar,
//          timeline and 40px change card all draw skeleton bars.

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

// DIFF-SPLIT-ADDED · a purely added file — no old side at all. In split view it
// used to render as one real column beside a half-width empty one, shoving the
// new code off the right of the viewport.
const addedDiff = `diff --git a/newfile.md b/newfile.md
new file mode 100644
--- /dev/null
+++ b/newfile.md
@@ -0,0 +1,3 @@
+first added line
+second added line
+third added line
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

// A backend that answers honestly per scope: `available` is a real capability of
// the session (a durable barrier for its latest human turn), not a guess.
const byScope = (lastTurn: Partial<DiffResp> | null) =>
  vi.fn((_sid: string, scope: DiffScope) =>
    Promise.resolve(
      scope === "last-turn"
        ? lastTurn
          ? baseDiff({ scope: "last-turn", available: true, ...lastTurn })
          : baseDiff({ scope: "last-turn", available: false, reason: "no durable baseline", diff: "" })
        : baseDiff(),
    ),
  );

const writeText = vi.fn(() => Promise.resolve());

beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  localStorage.clear();
  (window as any).matchMedia = () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  });
  writeText.mockClear();
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  useStore.setState({ toasts: [] });
});
afterEach(cleanup);

describe("The review has a copy affordance (INC-41 RVW-3)", () => {
  it("writes the whole unified diff to the clipboard and says so", async () => {
    arMock.diff = () => Promise.resolve(baseDiff());
    const { container } = render(<DiffView sid="c1" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const btn = screen.getByLabelText("Copy diff");
    // …in the toolbar, where Codex's review header carries it.
    expect(container.querySelector(".diffbar")!.contains(btn)).toBe(true);

    fireEvent.click(btn);
    // Verbatim: what `git diff` produced, so it pastes back as a diff.
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(editDiff));
    // Feedback is the app's existing toast, not a silent no-op.
    await waitFor(() => expect(useStore.getState().toasts.map((t) => t.text)).toContain("diff copied"));
  });

  it("offers nothing to copy when there is no diff", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: "" }));
    render(<DiffView sid="c2" />);

    // (the default scope is the turn now — RVW-4 — so this is its empty state)
    await waitFor(() => expect(screen.getByText("No changes this turn")).toBeTruthy());
    expect(screen.queryByLabelText("Copy diff")).toBeNull();
  });
});

describe("The review opens on the last turn (INC-41 RVW-4)", () => {
  it("defaults to last-turn — the scope the thread's change card counts", async () => {
    const diff = byScope({});
    arMock.diff = diff;
    render(<DiffView sid="s1" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(diff).toHaveBeenCalledWith("s1", "last-turn");
    expect(screen.getByLabelText("Change diff scope").textContent).toContain("Last Turn");
  });

  it("falls back to the working tree, silently, when the session has no turn baseline", async () => {
    const diff = byScope(null); // last-turn: available === false
    arMock.diff = diff;
    render(<DiffView sid="s2" />);

    // The default was ours, not the user's — so its failure is not their error
    // card to read. The panel simply shows the working tree's changes.
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(screen.queryByText("Last turn unavailable")).toBeNull();
    expect(diff.mock.calls.map((c) => c[1])).toEqual(["last-turn", "working-tree"]);
    expect(screen.getByLabelText("Change diff scope").textContent).toContain("Working Tree");
    // …and a fallback nobody asked for is not a preference: nothing is persisted.
    expect(localStorage.getItem("ar.diff.scope")).toBeNull();
  });

  it("still answers an explicit Last turn honestly when it is unavailable", async () => {
    arMock.diff = byScope(null);
    render(<DiffView sid="s3" />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy()); // fell back

    // The user asks for the turn anyway: now the empty state is the true answer.
    fireEvent.click(screen.getByLabelText("Change diff scope"));
    fireEvent.click(screen.getByText("Last Turn"));
    await waitFor(() => expect(screen.getByText("Last turn unavailable")).toBeTruthy());
    expect(screen.getByText("no durable baseline")).toBeTruthy();
  });

  it("persists an explicit switch and re-opens on it", async () => {
    const diff = byScope({});
    arMock.diff = diff;
    const first = render(<DiffView sid="s4" />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());

    fireEvent.click(screen.getByLabelText("Change diff scope"));
    fireEvent.click(screen.getByText("Working Tree"));
    await waitFor(() => expect(localStorage.getItem("ar.diff.scope")).toBe("working-tree"));
    first.unmount();

    diff.mockClear();
    render(<DiffView sid="s5" />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(diff).toHaveBeenCalledWith("s5", "working-tree");
    expect(screen.getByLabelText("Change diff scope").textContent).toContain("Working Tree");
  });

  it("shrugs off a storage that refuses to answer", async () => {
    const getItem = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("storage disabled");
    });
    const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("storage disabled");
    });
    const diff = byScope({});
    arMock.diff = diff;
    render(<DiffView sid="s6" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(diff).toHaveBeenCalledWith("s6", "last-turn");
    // and switching still works — it just doesn't persist.
    fireEvent.click(screen.getByLabelText("Change diff scope"));
    fireEvent.click(screen.getByText("Working Tree"));
    await waitFor(() => expect(diff).toHaveBeenCalledWith("s6", "working-tree"));
    getItem.mockRestore();
    setItem.mockRestore();
  });
});

// DIFF-SPLIT-ADDED · an added or deleted file has no opposite side to sit
// beside. Side-by-side split rendered it as one real column next to a half-width
// empty one, pushing the real code past the right edge of the viewport — the
// user saw a blank left pane and had to scroll horizontally to reach any new
// line. Codex renders single-sided files as one column; we fall back to the
// inline (single-column) path so the content is visible from the left. Modified
// files, which have two comparable sides, still split.
describe("Split view falls back to single column for one-sided files (DIFF-SPLIT-ADDED)", () => {
  it("renders a purely added file as single-column inline rows, not a half-empty split", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: addedDiff }));
    const { container } = render(<DiffView sid="sa1" />);

    await waitFor(() => expect(screen.getByText("newfile.md")).toBeTruthy());
    // Ask for split explicitly.
    fireEvent.click(screen.getByTitle("Split view"));

    // No two-column split rows and no empty split halves — the old-code column
    // that would have pushed the content off-screen is simply not there.
    expect(container.querySelector(".dls")).toBeNull();
    expect(container.querySelector(".dls-half")).toBeNull();
    // The added lines render as single-column `.dl` rows inside the inline body,
    // visible from the left.
    expect(container.querySelector(".fd-body")).toBeTruthy();
    expect(container.querySelectorAll(".dl").length).toBeGreaterThan(0);
    expect(container.textContent).toContain("first added line");
  });

  it("still renders a modified file as a two-column split", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff }));
    const { container } = render(<DiffView sid="sa2" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    fireEvent.click(screen.getByTitle("Split view"));

    // A file with both sides keeps the side-by-side grid.
    expect(container.querySelector(".dls")).toBeTruthy();
    expect(container.querySelector(".dls-half")).toBeTruthy();
  });
});

describe("The review loads as a skeleton (INC-41 RVW-6)", () => {
  it("draws file headers over a line-numbered grid instead of a grey sentence", async () => {
    arMock.diff = () => new Promise(() => {}); // still in flight
    const { container } = render(<DiffView sid="k1" />);

    const skel = await waitFor(() => {
      const s = container.querySelector(".diff-skeleton");
      expect(s).toBeTruthy();
      return s!;
    });
    expect(container.textContent).not.toMatch(/Loading changes…/);
    // The shape of the thing being loaded: file headers + a grid of rows.
    expect(skel.querySelectorAll(".dsk-file").length).toBe(3);
    expect(skel.querySelectorAll(".dsk-head").length).toBe(3);
    expect(skel.querySelectorAll(".dsk-row").length).toBe(12);
    // Each row has the line-number gutter cell the real `.dl` has.
    expect(skel.querySelector(".dsk-row .dsk-no")).toBeTruthy();
    expect(skel.getAttribute("role")).toBe("status");
    // The toolbar is still there while it loads (scope, refresh, ✕ all reachable).
    expect(container.querySelector(".diffbar")).toBeTruthy();
  });

  it("replaces itself with the diff once it lands", async () => {
    arMock.diff = () => Promise.resolve(baseDiff());
    const { container } = render(<DiffView sid="k2" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(container.querySelector(".diff-skeleton")).toBeNull();
  });
});
