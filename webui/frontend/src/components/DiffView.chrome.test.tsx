// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

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
import { useStore } from "../store";
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

// jsdom implements no scrolling at all, so the focus test needs a stub to
// observe (DiffView calls it optionally for exactly this reason).
const scrollSpy = vi.fn();

beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  // INC-41 RVW-4 · the panel's default scope is `last-turn` now. These tests pin
  // the working-tree chrome (Apply / Remove / Commit or push), so they state the
  // scope the way a user would: as a persisted, explicit choice.
  localStorage.setItem("ar.diff.scope", "working-tree");
  (window as any).matchMedia = () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  });
  scrollSpy.mockReset();
  (Element.prototype as any).scrollIntoView = scrollSpy;
  useStore.setState({ diffFocusPath: null });
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
    arMock.blob = vi.fn(() => Promise.reject(new Error("file is too large to expand")));
    const { container } = render(<DiffView sid="u2" />);

    await waitFor(() => expect(screen.getByText("chart.png")).toBeTruthy());
    const card = container.querySelector("details.filediff-untracked")!;
    expect(card.querySelector(".fd-glyph-added")!.textContent).toBe("A");
    // Same shape a *tracked* binary addition has: a "binary" badge and — since
    // DF-D3 — no line counts at all (they'd be a made-up "+0 −0").
    await waitFor(() => expect(card.querySelector(".fd-badge")!.textContent).toBe("binary"));
    expect(card.querySelector(".fd-counts")).toBeNull();
    // …and the reason where the rows would be, instead of a bare path.
    expect(card.querySelector(".fd-nobody")!.textContent).toMatch(/binary or too large/);
    // DF-D7: and it never asked. `chart.png` is bytes by definition, so the card
    // reaches that state without a request the server could only refuse (400).
    expect(arMock.blob).not.toHaveBeenCalled();
  });
});

// INC-41 DF-D7 — the review prefetches each untracked file's blob to state an
// honest `+N −0`. For a binary that prefetch is a *guaranteed* 400 ("file is not
// text", webui/meta.go handleBlob): the card degraded correctly, but every mount
// of it — every filter, fold-all and scope change — still bought a red line in
// the console and a wasted round-trip. A binary is now answered locally.
describe("Binary files are never prefetched (INC-41 DF-D7)", () => {
  it("issues zero blob requests for a binary untracked file, and still says why", async () => {
    const blob = vi.fn(() => Promise.resolve({ lines: ["never"] }));
    // a second (tracked) file, so the review is big enough to offer Expand-all.
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff, untracked: ["qa-inc41-d4/asset.bin"] }));
    arMock.blob = blob;
    const { container } = render(<DiffView sid="b1" />);

    await waitFor(() => expect(screen.getByText("asset.bin")).toBeTruthy());
    // The card is exactly the one the failed fetch used to produce…
    const card = container.querySelector("details.filediff-untracked")!;
    expect(card.querySelector(".fd-badge")!.textContent).toBe("binary");
    expect(card.querySelector(".fd-nobody")!.textContent).toMatch(/binary or too large/);
    expect(card.querySelector(".fd-counts")).toBeNull();
    // …and it cost nothing: not one request for THIS file, before or after the
    // user opens it (its tracked neighbour still prefetches, as it should).
    const binCalls = () => blob.mock.calls.filter((c: any[]) => c[1] === "qa-inc41-d4/asset.bin");
    expect(binCalls()).toHaveLength(0);
    fireEvent.click(screen.getByLabelText("More changes actions"));
    fireEvent.click(screen.getByText("Expand all files"));
    await waitFor(() => expect(container.querySelector("details.filediff-untracked[open]")).toBeTruthy());
    expect(binCalls()).toHaveLength(0);
  });

  it("still prefetches a text file — the guard is about bytes, not about caution", async () => {
    const blob = vi.fn(() => Promise.resolve({ lines: ["alpha", "beta"] }));
    arMock.diff = () => Promise.resolve(baseDiff({ untracked: ["notes.txt"] }));
    arMock.blob = blob;
    const { container } = render(<DiffView sid="b2" />);

    await waitFor(() => expect(container.querySelector(".fd-counts .add")!.textContent).toBe("+2"));
    expect(blob).toHaveBeenCalledTimes(1);
  });
});

// INC-41 TH-5 — the thread's change card names the files a turn touched; a click
// on one is a navigation. The panel opens AT that file: expanded, and scrolled
// into view — not folded somewhere in a list of thirty.
describe("Changes panel focuses the file the thread asked for (INC-41 TH-5)", () => {
  it("expands and scrolls to the pending focus path when the panel mounts", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff, untracked: ["assets/note.txt"] }));
    arMock.blob = () => Promise.resolve({ lines: ["alpha", "beta", "gamma"] });
    // the click in the thread happened first: the panel is mounted BY it.
    act(() => useStore.getState().focusDiffFile("assets/note.txt"));

    const { container } = render(<DiffView sid="f1" />);
    await waitFor(() => expect(screen.getByText("note.txt")).toBeTruthy());

    // An untracked card is folded by default (DF-2's reasoning) — focus overrides
    // that, and the card is scrolled to.
    await waitFor(() => expect(container.querySelector("details.filediff-untracked[open]")).toBeTruthy());
    expect(scrollSpy).toHaveBeenCalled();
    // one-shot: the request is consumed, so re-opening the panel later doesn't
    // silently re-focus a file the user has moved on from.
    expect(useStore.getState().diffFocusPath).toBeNull();
  });

  it("re-opens a file the user had collapsed when the thread asks for it again", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff + newFileDiff }));
    const { container } = render(<DiffView sid="f2" />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());

    // user folds everything…
    fireEvent.click(screen.getByLabelText("More changes actions"));
    fireEvent.click(screen.getByText("Collapse all files"));
    await waitFor(() => expect(container.querySelector("details.filediff[open]")).toBeNull());

    // …then clicks app.ts in the thread's change card, with the panel already open.
    act(() => useStore.getState().focusDiffFile("app.ts"));
    await waitFor(() => expect(container.querySelectorAll("details.filediff[open]").length).toBe(1));
    // exactly that file — its neighbours keep the fold the user chose.
    expect(container.querySelector("details.filediff[open] .fd-path")!.textContent).toBe("app.ts");
    expect(scrollSpy).toHaveBeenCalled();
    expect(useStore.getState().diffFocusPath).toBeNull();
  });

  it("clears a file filter that would have hidden the focused file", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff + newFileDiff }));
    const { container } = render(<DiffView sid="f3" />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());

    // RD-12 · a path now appears in the file list as well as in its header, so the
    // review's own content is read off the stream.
    const streamPaths = () => [...container.querySelectorAll("summary.fd-head .fd-path")].map((p) => p.textContent);
    fireEvent.click(screen.getByLabelText("Changed files"));
    fireEvent.change(screen.getByLabelText("Filter files by path", { selector: "input" }), {
      target: { value: "notes" },
    });
    await waitFor(() => expect(streamPaths()).toEqual(["notes.md"]));

    act(() => useStore.getState().focusDiffFile("app.ts"));
    await waitFor(() => expect(streamPaths()).toContain("app.ts"));
    expect(container.querySelector("details.filediff[open]")).toBeTruthy();
  });
});

// INC-41 DF-D3 / DF-D6 — a binary file header used to read
// `A bin/ar          +0 −0                                       [binary]`:
// two numbers nobody measured (a binary has no lines), and the one badge that
// says so exiled ~475px away by the elastic spacer, hard against the panel's
// right edge, where it read as a column of its own.
describe("Binary file headers (INC-41 DF-D3 / DF-D6)", () => {
  const binaryDiff = `diff --git a/bin/ar b/bin/ar
new file mode 100755
Binary files /dev/null and b/bin/ar differ
`;

  it("prints no counts for a binary file, and keeps its badge next to the name", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: binaryDiff }));
    const { container } = render(<DiffView sid="b1" />);

    await waitFor(() => expect(screen.getByText("ar")).toBeTruthy());
    const head = container.querySelector("summary.fd-head")!;
    // DF-D3 · no fabricated +0 −0 — the badge is the whole statement.
    expect(head.querySelector(".fd-counts")).toBeNull();
    expect(head.textContent).not.toMatch(/[+−]0/);
    const badge = head.querySelector(".fd-badge")!;
    expect(badge.textContent).toBe("binary");
    // DF-D6 · badge before the elastic gap, i.e. it travels with the filename
    // instead of being pushed to the far right edge of the header.
    const kids = [...head.children].map((el) => el.className);
    expect(kids.indexOf("fd-badge")).toBeLessThan(kids.indexOf("fd-spacer"));
  });

  it("still counts the lines of an ordinary (non-binary) file", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff }));
    const { container } = render(<DiffView sid="b2" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const head = container.querySelector("summary.fd-head")!;
    expect(head.querySelector(".fd-counts .add")!.textContent).toBe("+1");
    expect(head.querySelector(".fd-counts .del")!.textContent).toBe("−1");
  });
});

// INC-41 DF-D5 / RD-12 — the hidden-files note. It used to be the review's first
// line: before you saw a single changed file, you read what wasn't there. It is
// the file list's footnote now — same count, same sentence, same tooltip — and the
// review opens on its first file header, the way the golden's does.
describe("Hidden-files note (INC-41 DF-D5 / RD-12)", () => {
  it("is a footnote of the file list, not the review's first line", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: editDiff, hiddenUntracked: 812 }));
    const { container } = render(<DiffView sid="h1" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    // RD-12 · not in the stream…
    expect(container.querySelector(".diffwrap > .diff-hidden-note")).toBeNull();
    const wrap = container.querySelector(".diffwrap")!;
    expect(wrap.children[0].className).toMatch(/diffbar/); // the toolbar
    expect(wrap.children[1].className).toMatch(/filediff/); // …then the first file

    // …but nothing is lost: it is under the file list, and it is still one click
    // away (the popover renders even for a single-file review, because the count
    // it carries is about files that are *not* in that review).
    fireEvent.click(screen.getByLabelText("Changed files"));
    const note = container.querySelector(".pop-panel .diff-hidden-note")!;
    expect(note.querySelector("b")!.textContent).toBe("812 generated files hidden");
    const tail = note.querySelector("span")!.textContent!;
    expect(tail).toBe("Source files all still shown.");
    expect(note.getAttribute("title")).toMatch(/source file remains visible/i);
  });
});

// INC-41 RD-12 — the review's file list. Our toolbar could only answer "which
// files match what I type"; the golden's file-tree button answers "what did this
// change touch" — every file, its `+N −M`, one click to walk the review to it.
describe("Changed-files list (INC-41 RD-12)", () => {
  const listDiff = editDiff + newFileDiff;

  it("lists every changed file — tracked and untracked — with its counts", async () => {
    arMock.diff = () =>
      Promise.resolve(baseDiff({ diff: listDiff, untracked: ["assets/logo.png", "data/big.csv"] }));
    arMock.blob = () => Promise.resolve({ lines: ["one", "two"] });
    const { container } = render(<DiffView sid="l1" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    fireEvent.click(screen.getByLabelText("Changed files"));

    const rows = [...container.querySelectorAll(".diff-fileitem")];
    expect(rows.map((r) => r.getAttribute("title"))).toEqual([
      // untracked first — the order the panel renders them in
      "assets/logo.png",
      "data/big.csv",
      "app.ts",
      "notes.md",
    ]);
    // …with the same M/A/D glyph vocabulary as the file headers below.
    expect(rows[2].querySelector(".fd-glyph")!.className).toMatch(/fd-glyph-modified/);
    expect(rows[3].querySelector(".fd-glyph")!.className).toMatch(/fd-glyph-added/);
    // …and each file's own `+N −M` (app.ts: one line replaced).
    expect(rows[2].querySelector(".fd-counts .add")!.textContent).toBe("+1");
    expect(rows[2].querySelector(".fd-counts .del")!.textContent).toBe("−1");
    expect(rows[3].querySelector(".fd-counts .add")!.textContent).toBe("+1");
    expect(rows[3].querySelector(".fd-counts .del")!.textContent).toBe("−0");
    // A binary blob has no lines to count, so it states none — exactly as its
    // header does (DF-D3); a not-yet-read text blob says "+…", never a made-up 0.
    expect(rows[0].querySelector(".fd-counts")).toBeNull();
    expect(rows[1].querySelector(".fd-counts .add")!.textContent).toBe("+…");
  });

  it("walks the review to the file that was clicked", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: listDiff }));
    const { container } = render(<DiffView sid="l2" />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());

    // fold everything, so landing on the file means opening it too
    fireEvent.click(screen.getByLabelText("More changes actions"));
    fireEvent.click(screen.getByText("Collapse all files"));
    await waitFor(() => expect(container.querySelector("details.filediff[open]")).toBeNull());
    scrollSpy.mockReset();

    fireEvent.click(screen.getByLabelText("Changed files"));
    const row = [...container.querySelectorAll(".diff-fileitem")].find(
      (r) => r.getAttribute("title") === "notes.md",
    )!;
    fireEvent.click(row);

    // the click closes the popover, opens that one file, and scrolls to it
    await waitFor(() => expect(container.querySelectorAll("details.filediff[open]").length).toBe(1));
    expect(container.querySelector("details.filediff[open] .fd-path")!.textContent).toBe("notes.md");
    expect(scrollSpy).toHaveBeenCalled();
    expect(container.querySelector(".pop-panel")).toBeNull();
  });

  it("filters the list and the review together, and keeps the query when a file is picked", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: listDiff }));
    const { container } = render(<DiffView sid="l3" />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());

    fireEvent.click(screen.getByLabelText("Changed files"));
    expect(container.querySelectorAll(".diff-fileitem").length).toBe(2);

    fireEvent.change(screen.getByPlaceholderText("Filter files…"), { target: { value: "notes" } });
    // the list narrows…
    await waitFor(() => expect(container.querySelectorAll(".diff-fileitem").length).toBe(1));
    expect(container.querySelector(".diff-fileitem")!.getAttribute("title")).toBe("notes.md");
    // …and so does the review behind it (the filter's original job, unchanged).
    expect(screen.queryByText("app.ts")).toBeNull();

    // Picking a file from a filtered list keeps the filter: the user narrowed the
    // review on purpose, and the file they picked is *in* the narrowed set.
    fireEvent.click(container.querySelector(".diff-fileitem")!);
    await waitFor(() => expect(container.querySelector(".pop-panel")).toBeNull());
    expect(screen.queryByText("app.ts")).toBeNull();
    expect(screen.getByLabelText("Changed files").className).toMatch(/active/);
  });

  it("says so when nothing matches, instead of showing an empty list", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: listDiff }));
    const { container } = render(<DiffView sid="l4" />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());

    fireEvent.click(screen.getByLabelText("Changed files"));
    fireEvent.change(screen.getByPlaceholderText("Filter files…"), { target: { value: "zzz" } });
    await waitFor(() => expect(container.querySelector(".diff-filelist")).toBeNull());
    expect(container.querySelector(".diff-filelist-empty")!.textContent).toMatch(/zzz/);
  });
});

// INC-41 DF-6 — the toolbar summary used to drop whichever half was zero
// (`totalDel > 0 &&`), while the file headers below it never did. One panel,
// two ways of counting: the bar said `+1`, the header under it said `+1 −0`.
describe("Toolbar counts always show both halves (INC-41 DF-6)", () => {
  it("renders −0 when nothing was deleted", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: newFileDiff }));
    const { container } = render(<DiffView sid="c1" />);

    await waitFor(() => expect(screen.getByText("notes.md")).toBeTruthy());
    const summary = container.querySelector(".diffbar .diff-summary")!;
    expect(summary.querySelector(".add")!.textContent).toBe("+1");
    expect(summary.querySelector(".del")!.textContent).toBe("−0");
    // …and it agrees, digit for digit, with the file header underneath it.
    const head = container.querySelector("summary.fd-head")!;
    expect(head.querySelector(".fd-counts .add")!.textContent).toBe("+1");
    expect(head.querySelector(".fd-counts .del")!.textContent).toBe("−0");
  });

  it("renders +0 when the change is a pure deletion", async () => {
    const delOnly = `diff --git a/gone.ts b/gone.ts
deleted file mode 100644
--- a/gone.ts
+++ /dev/null
@@ -1,2 +0,0 @@
-const a = 1;
-const b = 2;
`;
    arMock.diff = () => Promise.resolve(baseDiff({ diff: delOnly }));
    const { container } = render(<DiffView sid="c2" />);

    await waitFor(() => expect(screen.getByText("gone.ts")).toBeTruthy());
    const summary = container.querySelector(".diffbar .diff-summary")!;
    expect(summary.querySelector(".add")!.textContent).toBe("+0");
    expect(summary.querySelector(".del")!.textContent).toBe("−2");
  });
});

// INC-41 DF-5 — the "N unmodified lines" band was a flex row 10px in from the
// rail's edge: its caret aligned with nothing and its label started 27px left of
// the code column, so it read as a button bolted onto the diff. It is a row of
// the code grid now — caret in the line-number gutter cell, label at the code
// column — and it still expands.
describe("Collapse band sits on the code grid (INC-41 DF-5)", () => {
  // A hunk that starts at line 5 leaves lines 1–4 hidden → a leading band whose
  // length is known from the diff alone (no blob needed to render it).
  const gappedDiff = `diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -5,2 +5,2 @@
-const a = 1;
+const a = 2;
 const b = 3;
`;

  it("gives the band the gutter cell + code-column label the rows have", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: gappedDiff }));
    arMock.blob = () => Promise.resolve({ lines: ["l1", "l2", "l3", "l4", "const a = 2;", "const b = 3;"] });
    const { container } = render(<DiffView sid="g1" />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const band = await waitFor(() => {
      const b = container.querySelector<HTMLButtonElement>(".fd-body .fd-gap");
      expect(b).toBeTruthy();
      return b!;
    });
    expect(band.textContent).toContain("4 unmodified lines");
    // Two cells: the caret's gutter box (which the grid sizes to the line-number
    // column) and the label, which starts at the code column's left edge.
    const caret = band.querySelector(".fd-gap-caret")!;
    expect(caret.querySelector("svg")).toBeTruthy();
    expect(band.querySelector(".fd-gap-label")!.textContent).toBe("4 unmodified lines");
    // The caret is decoration; the accessible name is the band's label + title.
    expect(caret.getAttribute("aria-hidden")).toBe("true");
  });

  it("still reveals the hidden lines when clicked, and folds them again", async () => {
    arMock.diff = () => Promise.resolve(baseDiff({ diff: gappedDiff }));
    arMock.blob = () => Promise.resolve({ lines: ["l1", "l2", "l3", "l4", "const a = 2;", "const b = 3;"] });
    const { container } = render(<DiffView sid="g2" />);

    const band = await waitFor(() => {
      const b = container.querySelector<HTMLButtonElement>(".fd-body .fd-gap");
      expect(b).toBeTruthy();
      return b!;
    });
    fireEvent.click(band);
    await waitFor(() => expect(screen.getByText("l1")).toBeTruthy());
    expect(screen.getByText("l4")).toBeTruthy();

    fireEvent.click(container.querySelector<HTMLButtonElement>(".fd-body .fd-gap")!);
    await waitFor(() => expect(screen.queryByText("l1")).toBeNull());
  });
});
