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
    diffMock.mockResolvedValueOnce(okDiff(2));
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    await waitFor(() => expect(diffMock).toHaveBeenCalledTimes(2));
    await screen.findByText("Edited 2 files");
    expect(screen.queryByText("Couldn't load changes")).toBeNull();
    // +2/-1 per file × 2 files.
    expect(screen.getByText("+4")).toBeTruthy();
    expect(screen.getByText("−2")).toBeTruthy();
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
});
