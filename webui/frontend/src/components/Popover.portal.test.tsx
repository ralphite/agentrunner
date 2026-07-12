// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Popover, PopItem } from "./Popover";

// INC-41 ENV-CLIP — round 36 turned the Environment rail into a floating card
// with `overflow: auto` (tw.css). The `Commit or push` menu it hosts
// was an absolutely positioned *descendant* of that card, so the card's overflow
// box cut 125px — 56% — off it: `Commit & push` and `Push` were not merely
// half-drawn, they were unclickable (elementFromPoint landed on the timeline
// behind them), leaving one of the three git actions reachable.
//
// The fix is that a Popover panel is positioned against the viewport
// (`position: fixed` + measured coordinates), which no ancestor `overflow` can
// clip, whatever the panel is nested in. These tests pin that contract — the
// geometry AND the plumbing that has to survive it (outside-click, Escape,
// keyboard nav), because a menu that is visible but no longer closable is not a
// fix.

const VW = 1440;
const VH = 900;
const PANEL_W = 264;

/** Anchor rect in viewport coordinates — what the trigger button reports. */
type Rect = { left: number; right: number; top: number; bottom: number };

function setViewport() {
  Object.defineProperty(window, "innerWidth", { value: VW, configurable: true });
  Object.defineProperty(window, "innerHeight", { value: VH, configurable: true });
}

/** jsdom lays nothing out: hand the component the rect/width it would measure. */
function measure(anchor: Rect) {
  Object.defineProperty(HTMLElement.prototype, "offsetWidth", {
    configurable: true,
    get(this: HTMLElement) {
      return this.classList.contains("pop-panel") ? PANEL_W : 0;
    },
  });
  const wrap = document.querySelector(".pop-wrap") as HTMLElement;
  wrap.getBoundingClientRect = () =>
    ({ ...anchor, width: anchor.right - anchor.left, height: anchor.bottom - anchor.top, x: anchor.left, y: anchor.top }) as DOMRect;
  return wrap;
}

/**
 * The Environment rail: a scrolling card (`overflow: auto`) that hosts the
 * `Commit or push` popover — the exact nesting that produced the bug.
 */
function renderInScrollingCard(anchor: Rect, align: "left" | "right" = "left") {
  render(
    <div className="supervision-panel" style={{ position: "absolute", height: 265, overflow: "auto" }}>
      <Popover
        align={align}
        trigger={(open, toggle) => (
          <button onClick={toggle} aria-expanded={open}>
            Commit or push
          </button>
        )}
      >
        {(close) => (
          <>
            <PopItem title="Commit" onClick={close} />
            <PopItem title="Commit & push" onClick={close} />
            <PopItem title="Push" onClick={close} />
          </>
        )}
      </Popover>
    </div>,
  );
  return measure(anchor); // the .pop-wrap, so a test can move the anchor under it
}

const openMenu = () => fireEvent.click(screen.getByRole("button", { name: "Commit or push" }));
const panel = () => document.querySelector(".pop-panel") as HTMLElement;

afterEach(cleanup);
setViewport();

describe("Popover panel escapes its ancestors' overflow", () => {
  it("pins the panel to the viewport, not to the scrolling card it lives in", () => {
    // The live rect from the bug report: the `Commit or push` trigger ends at
    // y=313, inside a card whose overflow box ends at y=321 — the menu drops down
    // into 8px of card and 587px of screen. Absolutely positioned, it lost every
    // pixel past 321.
    renderInScrollingCard({ left: 1196, right: 1420, top: 285, bottom: 313 });
    openMenu();

    // `position: fixed` is the whole fix: the containing block is the viewport,
    // so the card's `overflow: auto` has nothing to clip.
    expect(panel().style.position).toBe("fixed");

    // 8px under the trigger, in viewport coordinates — the card's bottom edge is
    // no longer part of the conversation.
    expect(panel().style.top).toBe(`${313 + 8}px`);
    expect(panel().style.bottom).toBe("auto");
    expect(panel().className).toContain("pop-down");
    // …and capped to the room the viewport actually has, so it cannot run off the
    // bottom of the screen instead.
    expect(panel().style.maxHeight).toBe(`${VH - 313 - 16}px`);

    // Left-aligned to the anchor — except a 264px panel hung off x=1196 would end
    // at 1460, past the 1440px window, so it slides back to the gutter. The
    // assertion that matters for clicking: every pixel of it is on-screen.
    const left = Number.parseFloat(panel().style.left);
    expect(left).toBe(VW - 8 - PANEL_W); // 1168
    expect(left + PANEL_W).toBeLessThanOrEqual(VW - 8);

    // None of the stylesheet's absolute-era offsets survive: `.pop-up { bottom:
    // calc(100% + 8px) }` and `.pop-right { right: 0 }` would mean the *viewport's*
    // edge on a fixed box, which is how a menu ends up glued to the wrong corner.
    expect(panel().style.right).toBe("auto");
  });

  it("still drops up from a composer chip at the bottom of the screen", () => {
    renderInScrollingCard({ left: 200, right: 320, top: 820, bottom: 848 });
    openMenu();
    expect(panel().className).toContain("pop-up");
    // bottom edge 8px above the chip, measured from the viewport's bottom.
    expect(panel().style.bottom).toBe(`${VH - 820 + 8}px`);
    expect(panel().style.top).toBe("auto");
    expect(panel().style.maxHeight).toBe(`${820 - 16}px`);
  });

  it("keeps a right-aligned panel's right edge on the anchor, and on-screen", () => {
    renderInScrollingCard({ left: 1380, right: 1420, top: 285, bottom: 313 }, "right");
    openMenu();
    // right-aligned: left = anchor.right - width = 1420 - 264 = 1156
    expect(panel().style.left).toBe("1156px");
    expect(panel().style.right).toBe("auto");
  });

  it("clamps a panel whose anchor sits against the viewport edge", () => {
    // A left-aligned anchor 100px from the right edge would push 164px of the
    // panel off-screen; it slides back to the 8px gutter instead.
    renderInScrollingCard({ left: 1340, right: 1420, top: 285, bottom: 313 });
    openMenu();
    expect(panel().style.left).toBe(`${VW - 8 - PANEL_W}px`); // 1168
  });

  it("drops down when the anchor is near the top and caps the panel to the room below", () => {
    renderInScrollingCard({ left: 40, right: 200, top: 60, bottom: 88 });
    openMenu();
    expect(panel().className).toContain("pop-down");
    expect(panel().style.top).toBe("96px"); // anchor bottom + 8
    expect(panel().style.bottom).toBe("auto");
    expect(panel().style.maxHeight).toBe(`${VH - 88 - 16}px`);
  });
});

describe("Popover follows or lets go when the page moves", () => {
  it("re-measures on scroll — a viewport-pinned panel does not ride its scroller", async () => {
    const wrap = renderInScrollingCard({ left: 1196, right: 1420, top: 285, bottom: 313 });
    openMenu();
    expect(panel().style.top).toBe("321px");

    // the pane under it scrolls 100px: the anchor moves up, the panel with it.
    wrap.getBoundingClientRect = () => ({ left: 1196, right: 1420, top: 185, bottom: 213, width: 224, height: 28, x: 1196, y: 185 }) as DOMRect;
    await act(async () => {
      fireEvent.scroll(document.querySelector(".supervision-panel")!);
      await new Promise((r) => requestAnimationFrame(r));
    });
    expect(panel().style.top).toBe("221px");
  });

  it("closes when its anchor scrolls out of the viewport", async () => {
    const wrap = renderInScrollingCard({ left: 1196, right: 1420, top: 285, bottom: 313 });
    openMenu();
    expect(panel()).toBeTruthy();

    wrap.getBoundingClientRect = () => ({ left: 1196, right: 1420, top: -80, bottom: -52, width: 224, height: 28, x: 1196, y: -80 }) as DOMRect;
    await act(async () => {
      fireEvent.scroll(document.querySelector(".supervision-panel")!);
      await new Promise((r) => requestAnimationFrame(r));
    });
    // A menu whose trigger is gone has nothing to hang off — it must not float on.
    expect(panel()).toBeNull();
  });
});

describe("Popover plumbing survives the reposition", () => {
  it("still closes on Escape, on outside click, and not on a click inside itself", () => {
    renderInScrollingCard({ left: 1196, right: 1420, top: 285, bottom: 313 });

    openMenu();
    fireEvent.mouseDown(panel()); // inside: stays open (the row must be clickable!)
    expect(panel()).toBeTruthy();

    fireEvent.mouseDown(document.body); // outside: closes
    expect(panel()).toBeNull();

    openMenu();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(panel()).toBeNull();
  });

  it("still walks the menu with the arrow keys", () => {
    renderInScrollingCard({ left: 1196, right: 1420, top: 285, bottom: 313 });
    openMenu();

    fireEvent.keyDown(document, { key: "ArrowDown" });
    expect(document.activeElement).toBe(screen.getByRole("menuitem", { name: "Commit" }));
    fireEvent.keyDown(document, { key: "ArrowDown" });
    expect(document.activeElement).toBe(screen.getByRole("menuitem", { name: "Commit & push" }));
    fireEvent.keyDown(document, { key: "End" });
    expect(document.activeElement).toBe(screen.getByRole("menuitem", { name: "Push" }));
  });

  it("hands every git action a clickable row — the bug was that two of three were not", () => {
    const clicked: string[] = [];
    render(
      <div className="supervision-panel" style={{ overflow: "auto" }}>
        <Popover trigger={(_o, toggle) => <button onClick={toggle}>Commit or push</button>}>
          {(close) => (
            <>
              {["Commit", "Commit & push", "Push"].map((label) => (
                <PopItem key={label} title={label} onClick={() => { clicked.push(label); close(); }} />
              ))}
            </>
          )}
        </Popover>
      </div>,
    );
    measure({ left: 1196, right: 1420, top: 285, bottom: 313 });

    for (const label of ["Commit", "Commit & push", "Push"]) {
      openMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: label }));
    }
    expect(clicked).toEqual(["Commit", "Commit & push", "Push"]);
  });
});
