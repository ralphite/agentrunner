// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// INC-41 TH-6/7/8 — the "Edited N files" card.
//
// TH-7 is the bug these tests exist for: the card had exactly two outcomes, a
// summary or `null`, so a /diff that FAILED rendered the same as a turn that
// changed nothing — the card vanished silently and the user read "the agent
// touched no files". The three phases (loading / error+retry / genuinely empty)
// are now distinct, and each is pinned below.

const diffMock = vi.fn();
vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    diff: (...args: any[]) => diffMock(...args),
    revert: vi.fn(),
    fileURL: (_sid: string, path: string) => `/f/${path}`,
  },
}));

import { ChangesOutcome } from "./ChangesOutcome";
import { Markdown } from "./Markdown";
import { useStore } from "../store";

// A unified diff with `n` files, each +2 / -1 — enough to drive the counts and
// the preview cap without hand-building FileDiffSummary structs.
const diffText = (n: number) =>
  Array.from({ length: n }, (_, i) =>
    [
      `diff --git a/src/mod${i}.ts b/src/mod${i}.ts`,
      "@@ -1,2 +1,3 @@",
      " keep",
      "-gone",
      "+new one",
      "+new two",
    ].join("\n"),
  ).join("\n");

const okDiff = (n: number) => ({ workspace: "/w", known: true, isRepo: true, diff: diffText(n), untracked: [] });

const renderCard = () =>
  render(<ChangesOutcome sid="s1" refreshKey={0} onReview={() => {}} />);

// braces matter: mockReset() returns the mock itself, and a hook that RETURNS a
// function hands vitest a teardown callback — it would call the mock (returning
// a never-settling promise) and hang the suite.
beforeEach(() => { diffMock.mockReset(); });
afterEach(cleanup);

describe("ChangesOutcome phases (INC-41 TH-7)", () => {
  it("paints a skeleton while the diff is in flight — never a blank slot", async () => {
    diffMock.mockReturnValue(new Promise(() => {})); // never settles
    const { container } = renderCard();
    // the card SHELL is already there (so the thread doesn't reflow when it
    // resolves), with placeholder bars instead of the title.
    expect(screen.getByLabelText("Workspace changes")).toBeTruthy();
    expect(screen.getByLabelText("Loading changes")).toBeTruthy();
    expect(container.querySelector(".changes-outcome-icon svg")).toBeTruthy();
    expect(screen.queryByText(/Edited/)).toBeNull();
  });

  it("keeps the card shell and offers Retry when the diff fetch fails", async () => {
    diffMock.mockRejectedValueOnce(new Error("boom"));
    renderCard();
    await screen.findByText("Couldn't load changes");
    // the shell survives — the card did NOT silently evaporate.
    expect(screen.getByLabelText("Workspace changes")).toBeTruthy();
    expect(screen.getByRole("button", { name: /Retry/ })).toBeTruthy();
    expect(diffMock).toHaveBeenCalledTimes(1);

    // Retry re-fetches; on success the real card replaces the error shell.
    diffMock.mockResolvedValue(okDiff(2));
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    await waitFor(() => expect(diffMock).toHaveBeenCalledTimes(2));
    await screen.findByText("Edited 2 files");
    expect(screen.queryByText("Couldn't load changes")).toBeNull();
    // +2/-1 per file × 2 files.
    expect(screen.getByText("+4")).toBeTruthy();
    expect(screen.getByText("-2")).toBeTruthy();
  });

  it("renders nothing only when the backend really reports no changed files", async () => {
    diffMock.mockResolvedValue({ workspace: "/w", known: true, isRepo: true, diff: "", untracked: [] });
    const { container } = renderCard();
    await waitFor(() => expect(container.querySelector(".changes-outcome")).toBeNull());
    expect(screen.queryByLabelText("Workspace changes")).toBeNull();
  });

  it("renders nothing when the workspace isn't a repo", async () => {
    diffMock.mockResolvedValue({ workspace: "/w", known: true, isRepo: false, diff: "", untracked: [] });
    const { container } = renderCard();
    await waitFor(() => expect(container.querySelector(".changes-outcome")).toBeNull());
  });

  it("does not strobe back to the skeleton when refreshKey ticks mid-stream", async () => {
    diffMock.mockResolvedValue(okDiff(2));
    const { rerender } = render(<ChangesOutcome sid="s1" refreshKey={0} onReview={() => {}} />);
    await screen.findByText("Edited 2 files");
    // a streamed event bumps refreshKey → refetch, but the loaded card must stay
    // on screen while the new fetch is in flight.
    diffMock.mockReturnValue(new Promise(() => {}));
    rerender(<ChangesOutcome sid="s1" refreshKey={1} onReview={() => {}} />);
    expect(screen.getByText("Edited 2 files")).toBeTruthy();
    expect(screen.queryByLabelText("Loading changes")).toBeNull();
  });
});

describe("ChangesOutcome badge glyph (INC-41 TH-6)", () => {
  it("is a boxed ± — a box, a plus and a minus — not GitDiff's branch arrows", async () => {
    diffMock.mockResolvedValue(okDiff(2));
    const { container } = renderCard();
    await screen.findByText("Edited 2 files");
    const svg = container.querySelector(".changes-outcome-icon svg")!;
    expect(svg).toBeTruthy();
    // the box
    expect(svg.querySelector("rect")).toBeTruthy();
    // + over − : a vertical stroke, a horizontal one crossing it, and a lone
    // horizontal one below (GitDiff has neither a rect nor these).
    const d = svg.querySelector("path")!.getAttribute("d")!;
    expect(d).toMatch(/^M12 7\.4v4\.3M9\.85 9\.55h4\.3M9\.4 15\.95h5\.2$/);
  });
});

// TH-9 — the same picture used to be painted three times in one screen: inline
// in the answer, again as a thumbnail card, and again as a filename row. The
// thumbnail row now stands down for anything the prose already shows.
describe("ChangesOutcome image artifacts (INC-41 TH-9)", () => {
  const imgDiff = (untracked: string[]) => ({ workspace: "/w", known: true, isRepo: true, diff: "", untracked });

  it("drops the thumbnail card for an image the answer already renders inline", async () => {
    diffMock.mockResolvedValue(imgDiff(["qa/chart.png", "qa/shot.png"]));
    const { container } = render(
      <>
        <Markdown text={"![chart](./qa/chart.png)"} sid="s1" />
        <ChangesOutcome sid="s1" refreshKey={0} onReview={() => {}} />
      </>,
    );
    await screen.findByText("Edited 2 files");
    // the inline image is on screen exactly once …
    expect(container.querySelectorAll("img.md-img").length).toBe(1);
    // … and it does NOT come back as a card. Only the image the prose never
    // showed still earns one.
    expect(screen.getByLabelText("Images produced this turn")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Open chart.png" })).toBeNull();
    expect(screen.getByRole("button", { name: "Open shot.png" })).toBeTruthy();
  });

  it("renders no artifact row at all — not an empty shell — when every image is inline", async () => {
    diffMock.mockResolvedValue(imgDiff(["qa/chart.png", "qa/shot.png"]));
    render(
      <>
        <Markdown text={"![chart](qa/chart.png)\n![shot](qa/shot.png)"} sid="s1" />
        <ChangesOutcome sid="s1" refreshKey={0} onReview={() => {}} />
      </>,
    );
    await screen.findByText("Edited 2 files");
    expect(screen.queryByLabelText("Images produced this turn")).toBeNull();
  });

  it("still shows cards for images no answer mentioned", async () => {
    diffMock.mockResolvedValue(imgDiff(["qa/chart.png"]));
    render(<ChangesOutcome sid="s1" refreshKey={0} onReview={() => {}} />);
    await screen.findByText("Edited 1 file");
    expect(screen.getByRole("button", { name: "Open chart.png" })).toBeTruthy();
  });
});

// TH-13 — a turn that only CREATED files has no ± counts to print. It used to
// print them anyway: "Edited 2 files  +0 −0", green and red and entirely false.
describe("ChangesOutcome header counts (INC-41 TH-13)", () => {
  it("reads `2 new` instead of a fabricated +0 −0 when no counts are known", async () => {
    diffMock.mockResolvedValue({ workspace: "/w", known: true, isRepo: true, diff: "", untracked: ["a.bin", "b.png"] });
    renderCard();
    await screen.findByText("Edited 2 files");
    expect(screen.getByText("2 new")).toBeTruthy();
    expect(screen.queryByText("+0")).toBeNull();
    expect(screen.queryByText("-0")).toBeNull();
  });

  it("counts only the files git counted, and appends the new ones as a suffix", async () => {
    diffMock.mockResolvedValue({ ...okDiff(2), untracked: ["c.png"] });
    renderCard();
    await screen.findByText("Edited 3 files");
    // +2/−1 per counted file × 2 — the untracked file adds nothing to the pair.
    expect(screen.getByText("+4")).toBeTruthy();
    expect(screen.getByText("-2")).toBeTruthy();
    expect(screen.getByText("· 1 new")).toBeTruthy();
  });
});

// TH-5 — the file rows were dead text. The card names the files the turn
// touched, and Codex's card sends you to a file's diff when you click its row;
// ours rendered the same three columns and swallowed the click, so the obvious
// follow-up question ("what changed in THAT file?") had no answer.
describe("ChangesOutcome file rows navigate to the file (INC-41 TH-5)", () => {
  beforeEach(() => useStore.setState({ diffFocusPath: null }));

  it("opens the Changes panel AT the clicked file", async () => {
    diffMock.mockResolvedValue(okDiff(3));
    const onReview = vi.fn();
    const { container } = render(<ChangesOutcome sid="s1" refreshKey={0} onReview={onReview} />);
    await screen.findByText("Edited 3 files");

    const row = screen.getByRole("button", { name: "Review changes to mod1.ts" });
    fireEvent.click(row);

    // …the same panel the header's Review button opens…
    expect(onReview).toHaveBeenCalledTimes(1);
    // …with a pending focus on exactly the file that was clicked.
    expect(useStore.getState().diffFocusPath).toBe("src/mod1.ts");
    // the row is reachable and operable from the keyboard too.
    expect((row as HTMLElement).tabIndex).toBe(0);
    expect(container.querySelector(".changes-outcome-files > div")!.className).toMatch(/cursor-pointer/);
  });

  it("is driven by the keyboard as well as the mouse", async () => {
    diffMock.mockResolvedValue(okDiff(2));
    const onReview = vi.fn();
    render(<ChangesOutcome sid="s1" refreshKey={0} onReview={onReview} />);
    await screen.findByText("Edited 2 files");

    fireEvent.keyDown(screen.getByRole("button", { name: "Review changes to mod0.ts" }), { key: "Enter" });
    expect(onReview).toHaveBeenCalledTimes(1);
    expect(useStore.getState().diffFocusPath).toBe("src/mod0.ts");
  });

  it("leaves the header's Review button pathless — it still opens the panel as it always did", async () => {
    diffMock.mockResolvedValue(okDiff(2));
    const onReview = vi.fn();
    render(<ChangesOutcome sid="s1" refreshKey={0} onReview={onReview} />);
    await screen.findByText("Edited 2 files");

    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(onReview).toHaveBeenCalledTimes(1);
    expect(useStore.getState().diffFocusPath).toBeNull();
  });
});

describe("ChangesOutcome preview cap (INC-41 TH-8)", () => {
  it("previews 3 file rows and folds the rest behind Show N more files", async () => {
    diffMock.mockResolvedValue(okDiff(9));
    const { container } = renderCard();
    await screen.findByText("Edited 9 files");
    const rows = () => container.querySelectorAll(".changes-outcome-files > div");
    expect(rows().length).toBe(3);
    expect(screen.getByText("mod0.ts")).toBeTruthy();
    expect(screen.queryByText("mod3.ts")).toBeNull();
    // N = total − 3
    const more = screen.getByRole("button", { name: /Show 6 more files/ });
    fireEvent.click(more);
    expect(rows().length).toBe(9);
    expect(screen.getByText("mod8.ts")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Show less/ }));
    expect(rows().length).toBe(3);
  });

  it("shows no toggle at all when the turn touched 3 files or fewer", async () => {
    diffMock.mockResolvedValue(okDiff(3));
    const { container } = renderCard();
    await screen.findByText("Edited 3 files");
    expect(container.querySelectorAll(".changes-outcome-files > div").length).toBe(3);
    expect(container.querySelector(".changes-outcome-files > button")).toBeNull();
  });

  // INC-41 TR-4 — one hidden file used to read "Show 1 more fileS".
  it("counts the hidden files in the singular when exactly one is folded away", async () => {
    diffMock.mockResolvedValue(okDiff(4));
    renderCard();
    await screen.findByText("Edited 4 files");
    expect(screen.getByRole("button", { name: /Show 1 more file$/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /more files/ })).toBeNull();
  });

  it("keeps the plural for two or more hidden files", async () => {
    diffMock.mockResolvedValue(okDiff(5));
    renderCard();
    await screen.findByText("Edited 5 files");
    expect(screen.getByRole("button", { name: /Show 2 more files$/ })).toBeTruthy();
  });
});

describe("ChangesOutcome mobile layout parity (INC-48)", () => {
  beforeEach(() => {
    Object.defineProperty(window, "innerWidth", { value: 390, configurable: true });
    Object.defineProperty(window, "innerHeight", { value: 844, configurable: true });
  });

  it("keeps the icon, two-line summary, and trailing actions in one horizontal header", async () => {
    diffMock.mockResolvedValue(okDiff(5));
    const { container } = renderCard();
    await screen.findByText("Edited 5 files");

    const card = screen.getByLabelText("Workspace changes");
    const header = card.querySelector(":scope > header")!;
    const title = header.querySelector(".changes-outcome-title")!;
    const actions = header.querySelector(".changes-outcome-actions")!;

    expect(card.className).toMatch(/overflow-hidden/);
    expect(header.className).toMatch(/flex min-w-0 items-center/);
    expect(header.children).toHaveLength(3);
    expect(title.className).toMatch(/grid min-w-0 flex-1/);
    expect(title.children).toHaveLength(2);
    expect(actions.className).toMatch(/ml-auto flex shrink-0/);
    expect(actions.querySelectorAll("button")).toHaveLength(2);
    expect(container.querySelector(".changes-outcome-icon")!.className).toMatch(/h-\[38px\].*w-\[38px\].*shrink-0/);
  });

  it("gives a long path the shrinking column and keeps counts in a separate fixed column", async () => {
    const path = "src/features/changes/mobile/an-extremely-long-file-name-that-must-truncate.ts";
    diffMock.mockResolvedValue({
      workspace: "/w",
      known: true,
      isRepo: true,
      diff: [
        `diff --git a/${path} b/${path}`,
        "@@ -1,2 +1,3 @@",
        " keep",
        "-gone",
        "+new one",
        "+new two",
      ].join("\n"),
      untracked: [],
    });
    renderCard();
    await screen.findByText("Edited 1 file");

    const row = screen.getByRole("button", { name: "Review changes to an-extremely-long-file-name-that-must-truncate.ts" });
    const pathColumn = row.children[0] as HTMLElement;
    const countColumn = row.children[1] as HTMLElement;

    expect(row.className).toMatch(/flex min-h-\[38px\] min-w-0/);
    expect(pathColumn.title).toBe(path);
    expect(pathColumn.className).toMatch(/min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap/);
    expect(countColumn.className).toMatch(/flex shrink-0/);
    expect(countColumn.textContent).toBe("+2-1");
  });
});

describe("ChangesOutcome scope pairing (QA-0719)", () => {
  // The card's title names a scope; the Review link must carry that same scope
  // into the diff panel. A "Changes in workspace" card whose Review opened a
  // "No changes this turn" panel is the claim/view mismatch QA-76 exists for.
  const emptyDiff = { workspace: "/w", known: true, isRepo: true, diff: "", untracked: [] };

  it("reports scope 'turn' from an Edited-N-files card", async () => {
    diffMock.mockImplementation((_sid: string, scope: string) =>
      Promise.resolve(scope === "last-turn" ? okDiff(1) : emptyDiff));
    const onReview = vi.fn();
    render(<ChangesOutcome sid="s1" refreshKey={0} onReview={onReview} />);
    fireEvent.click(await screen.findByText("Review"));
    expect(onReview).toHaveBeenCalledWith("turn");
  });

  it("reports scope 'workspace' from the workspace-fallback card — Review button and file rows alike", async () => {
    diffMock.mockImplementation((_sid: string, scope: string) =>
      Promise.resolve(scope === "last-turn" ? emptyDiff : okDiff(1)));
    const onReview = vi.fn();
    render(<ChangesOutcome sid="s1" refreshKey={0} onReview={onReview} />);
    await screen.findByText("Changes in workspace");
    fireEvent.click(screen.getByText("Review"));
    expect(onReview).toHaveBeenCalledWith("workspace");
    fireEvent.click(screen.getByLabelText("Review changes to mod0.ts"));
    expect(onReview).toHaveBeenLastCalledWith("workspace");
  });
});

describe("ChangesOutcome merge-conflict disclosure (INC-98.4j)", () => {
  const emptyDiff = { workspace: "/w", known: true, isRepo: true, diff: "", untracked: [] };

  it("shows a workspace conflict on the main timeline even when last-turn has ordinary changes", async () => {
    diffMock.mockImplementation((_sid: string, scope: string) =>
      Promise.resolve(scope === "last-turn"
        ? { ...okDiff(1), conflicts: ["README.md"] }
        : { ...okDiff(2), conflicts: ["README.md"] }));

    renderCard();

    await screen.findByText("Edited 1 file");
    const warning = screen.getByText("1 merge conflict");
    expect(warning).toBeTruthy();
    expect(warning.closest("em")?.getAttribute("title")).toBe("README.md");
  });

  it("keeps shared workspace conflicts visible when the current turn itself changed nothing", async () => {
    diffMock.mockImplementation((_sid: string, scope: string) =>
      Promise.resolve(scope === "last-turn"
        ? emptyDiff
        : { ...okDiff(1), conflicts: ["app.ts", "package.json"] }));

    renderCard();

    await screen.findByText("Changes in workspace");
    expect(screen.getByText("2 merge conflicts")).toBeTruthy();
  });
});
