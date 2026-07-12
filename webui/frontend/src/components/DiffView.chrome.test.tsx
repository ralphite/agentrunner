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

// INC-41 DF-3 — untracked files used to render as a grey `new files (untracked)
// · N` strip of bare paths, above every real file: no A glyph, no `+N −0`, no
// line numbers, nothing to open. Two visual languages for changed files in one
// panel. They are the same card as everything else now, body included.
describe("Untracked files are ordinary file cards (INC-41 DF-3)", () => {
  it("gives an untracked file the same header, counts and expandable body", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff, untracked: ["assets/note.txt"] }));
    arMock.blob = () => Promise.resolve({ lines: ["alpha", "beta", "gamma"] });
    const { container } = render(<DiffView sid="u1" />);

    await waitFor(() => expect(screen.getByText("note.txt")).toBeTruthy());
    // The old text strip is gone…
    expect(container.textContent).not.toMatch(/new files \(untracked\)/);

    // …and the file is a details card with the same summary as a tracked file.
    const card = container.querySelector("details.filediff-untracked")!;
    expect(card).toBeTruthy();
    const head = card.querySelector("summary.fd-head")!;
    expect(head.querySelector(".fd-caret")).toBeTruthy();
    expect(head.querySelector(".fd-glyph-added")!.textContent).toBe("A");
    expect(head.querySelector(".fd-path")!.textContent).toBe("assets/note.txt");
    // Counts: a new file is all additions (prefetched from the workspace blob).
    await waitFor(() => expect(head.querySelector(".fd-counts .add")!.textContent).toBe("+3"));
    expect(head.querySelector(".fd-counts .del")!.textContent).toBe("−0");

    // Expandable: Expand-all opens it and the body is the file, as added lines.
    fireEvent.click(screen.getByLabelText("More changes actions"));
    fireEvent.click(screen.getByText("Expand all files"));
    await waitFor(() => expect(container.querySelector("details.filediff-untracked[open]")).toBeTruthy());
    const rows = container.querySelectorAll("details.filediff-untracked .fd-body .dl.add");
    expect(rows.length).toBe(3);
    expect(rows[0].textContent).toContain("alpha");
    expect(rows[0].querySelector(".dl-no")!.textContent).toBe("1");
  });

  it("keeps the card for a file it cannot show, and says why", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ untracked: ["chart.png"] }));
    arMock.blob = () => Promise.reject(new Error("file is too large to expand"));
    const { container } = render(<DiffView sid="u2" />);

    await waitFor(() => expect(screen.getByText("chart.png")).toBeTruthy());
    const card = container.querySelector("details.filediff-untracked")!;
    expect(card.querySelector(".fd-glyph-added")!.textContent).toBe("A");
    // Same shape a *tracked* binary addition has: +0 −0 plus a "binary" badge…
    await waitFor(() => expect(card.querySelector(".fd-badge")!.textContent).toBe("binary"));
    expect(card.querySelector(".fd-counts .add")!.textContent).toBe("+0");
    // …and the reason where the rows would be, instead of a bare path.
    expect(card.querySelector(".fd-nobody")!.textContent).toMatch(/binary or too large/);
  });
});
