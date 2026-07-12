// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// INC-41 DF-1 — the Changes toolbar overflowed in exactly the sessions we use
// most (a worktree with more than one changed file): measured at 1440 the bar's
// scrollWidth was 692 against a 658px panel, which flexbox paid for by crushing
// the split/unified toggle to 2px and pushing the ✕ to x=1447 — outside a panel
// whose right edge is 1440. The close button was, literally, unreachable.
//
// The layout half of the fix lives in CSS (every control `flex: 0 0 auto`; only
// the spacer and the worktree chip give way) and is verified with Playwright.
// What jsdom can pin is the *composition* that made the bar fit: four resident
// controls (`…`, filter, split toggle, Commit or push) plus the ✕ — no resident
// 150px filter input, no separate Expand/Collapse-all button — and a worktree
// chip that states the branch, not a 195px unshrinkable sentence.

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
import type { DiffResp } from "../types";

const twoFileDiff = `diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -1,2 +1,2 @@
-const a = 1;
+const a = 2;
 const b = 3;
diff --git a/notes.md b/notes.md
--- a/notes.md
+++ b/notes.md
@@ -1,1 +1,1 @@
-old
+new
`;

const worktreeDiff = (over: Partial<DiffResp> = {}): DiffResp => ({
  scope: "working-tree",
  workspace: "/tmp/ws",
  known: true,
  isRepo: true,
  diff: twoFileDiff,
  numstat: "",
  untracked: [],
  worktree: true,
  mainRepo: "/repos/agentrunner",
  branch: "main",
  ...over,
});

beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  // INC-41 RVW-4 · the default scope is `last-turn` now; the bar's composition
  // here (Commit or push, the worktree actions) is the working tree's, so these
  // tests state that scope as an explicit, persisted user choice.
  localStorage.setItem("ar.diff.scope", "working-tree");
  (window as any).matchMedia = () => ({
    matches: false, // wide window: the chip shows its text, split view is allowed
    addEventListener: () => {},
    removeEventListener: () => {},
  });
});
afterEach(cleanup);

describe("Changes toolbar fits its panel (INC-41 DF-1)", () => {
  it("keeps the file filter behind an icon instead of a resident input", async () => {
    arMock.diff = () => Promise.resolve(worktreeDiff());
    const { container } = render(<DiffView sid="s1" onClose={() => {}} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const bar = container.querySelector(".diffbar")!;
    // The resident input was the second-widest thing on a bar that did not fit.
    expect(bar.querySelector("input")).toBeNull();

    fireEvent.click(screen.getByLabelText("Filter files by path"));
    const input = screen.getByPlaceholderText("Filter files…");
    // …and when it does open, it opens in the popover's own (absolutely
    // positioned) panel — so it never takes width from the bar's flex row.
    expect(input.closest(".pop-panel")).toBeTruthy();

    fireEvent.change(input, { target: { value: "notes" } });
    await waitFor(() => expect(screen.queryByText("app.ts")).toBeNull());
    expect(screen.getByText("notes.md")).toBeTruthy();
  });

  it("lights the filter trigger while a query is hiding files", async () => {
    arMock.diff = () => Promise.resolve(worktreeDiff());
    render(<DiffView sid="s2" onClose={() => {}} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const trigger = screen.getByLabelText("Filter files by path");
    expect(trigger.className).not.toMatch(/active/);

    fireEvent.click(trigger);
    fireEvent.change(screen.getByPlaceholderText("Filter files…"), { target: { value: "notes" } });
    // Closing the popover must not make a filtered review look like a full one.
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.getByLabelText("Filter files by path").className).toMatch(/active/);
  });

  it("moves Expand / Collapse-all into the … overflow", async () => {
    arMock.diff = () => Promise.resolve(worktreeDiff());
    const { container } = render(<DiffView sid="s3" onClose={() => {}} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(screen.queryByLabelText("Collapse all files")).toBeNull();
    expect(screen.queryByLabelText("Expand all files")).toBeNull();

    fireEvent.click(screen.getByLabelText("More changes actions"));
    fireEvent.click(screen.getByText("Collapse all files"));
    await waitFor(() => expect(container.querySelectorAll("details.filediff[open]").length).toBe(0));
  });

  it("leaves the ✕, the split toggle and Commit or push in the bar", async () => {
    arMock.diff = () => Promise.resolve(worktreeDiff());
    const onClose = vi.fn();
    const { container } = render(<DiffView sid="s4" onClose={onClose} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const bar = container.querySelector(".diffbar")!;
    expect(bar.contains(screen.getByLabelText("Close changes"))).toBe(true);
    expect(bar.querySelector(".diff-viewtoggle")).toBeTruthy();
    expect(bar.contains(screen.getByLabelText("Commit or push"))).toBe(true);

    fireEvent.click(screen.getByLabelText("Close changes"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("states the worktree's branch in the chip and its repo in the tooltip", async () => {
    arMock.diff = () => Promise.resolve(worktreeDiff());
    const { container } = render(<DiffView sid="s5" onClose={() => {}} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const badge = container.querySelector(".diff-wt-badge")!;
    expect(badge.textContent).toContain("worktree");
    expect(badge.textContent).toContain("main");
    // The repo name used to sit in the bar as an unshrinkable 195px sentence.
    expect(badge.textContent).not.toContain("agentrunner");
    expect(badge.getAttribute("title")).toContain("/repos/agentrunner");
  });

  it("says 'detached' when the worktree has no branch", async () => {
    arMock.diff = () => Promise.resolve(worktreeDiff({ branch: "" }));
    const { container } = render(<DiffView sid="s6" onClose={() => {}} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(container.querySelector(".diff-wt-badge")!.textContent).toContain("detached");
  });
});

// INC-41 DF-4 — the review hard-clipped long lines (`.dl-text{white-space:pre}`)
// behind a horizontal scrollbar per file, while a fenced code block in the
// *conversation* has had a Wrap toggle all along. Same product, two long-line
// policies. The rail carries the switch now, with the same aria-pressed contract
// as Markdown's CodeBlock; the wrapping itself is one class on .diffwrap
// (styles.diff.css), so it applies to inline rows, split halves and hunk headers
// at once.
describe("Diff line wrap (INC-41 DF-4)", () => {
  beforeEach(() => localStorage.clear());

  it("wraps every diff surface when the toolbar's Wrap switch is on", async () => {
    arMock.diff = () => Promise.resolve(worktreeDiff());
    const { container } = render(<DiffView sid="w1" onClose={() => {}} />);

    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    const wrapBtn = screen.getByLabelText("Wrap long lines");
    expect(container.querySelector(".diffbar")!.contains(wrapBtn)).toBe(true);
    // Off by default — Codex's own default is a single non-wrapping code surface.
    expect(wrapBtn.getAttribute("aria-pressed")).toBe("false");
    expect(container.querySelector(".diffwrap")!.className).not.toMatch(/diff-wrap\b/);

    fireEvent.click(wrapBtn);
    expect(screen.getByLabelText("Wrap long lines").getAttribute("aria-pressed")).toBe("true");
    expect(container.querySelector(".diffwrap")!.className).toMatch(/diff-wrap\b/);

    fireEvent.click(screen.getByLabelText("Wrap long lines"));
    expect(container.querySelector(".diffwrap")!.className).not.toMatch(/diff-wrap\b/);
  });

  it("remembers the preference across mounts (one switch for the whole review)", async () => {
    arMock.diff = () => Promise.resolve(worktreeDiff());
    const first = render(<DiffView sid="w2" onClose={() => {}} />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    fireEvent.click(screen.getByLabelText("Wrap long lines"));
    expect(localStorage.getItem("ar.diff.wrap")).toBe("1");
    first.unmount();

    const { container } = render(<DiffView sid="w3" onClose={() => {}} />);
    await waitFor(() => expect(screen.getByText("app.ts")).toBeTruthy());
    expect(container.querySelector(".diffwrap")!.className).toMatch(/diff-wrap\b/);
    expect(screen.getByLabelText("Wrap long lines").getAttribute("aria-pressed")).toBe("true");
  });
});
