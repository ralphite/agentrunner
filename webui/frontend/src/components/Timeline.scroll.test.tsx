// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { TimelineView } from "./Timeline";
import type { BubbleItem } from "../timeline";

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as any).ResizeObserver ??= NoopResizeObserver;

const assistant = (key: string): BubbleItem => ({
  kind: "assistant",
  key,
  text: key,
  ts: "2026-07-23T12:00:00Z",
});

let tops: WeakMap<HTMLElement, number>;
let scrollHeight: number;
let clientHeight: number;
let originalTop: PropertyDescriptor | undefined;
let originalHeight: PropertyDescriptor | undefined;
let originalClient: PropertyDescriptor | undefined;
let originalScrollTo: typeof HTMLElement.prototype.scrollTo;

beforeEach(() => {
  sessionStorage.clear();
  tops = new WeakMap();
  scrollHeight = 3000;
  clientHeight = 500;
  originalTop = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "scrollTop");
  originalHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "scrollHeight");
  originalClient = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientHeight");
  originalScrollTo = HTMLElement.prototype.scrollTo;
  Object.defineProperty(HTMLElement.prototype, "scrollTop", {
    configurable: true,
    get() {
      return tops.get(this) ?? 0;
    },
    set(value: number) {
      tops.set(this, Math.min(value, scrollHeight - clientHeight));
    },
  });
  Object.defineProperty(HTMLElement.prototype, "scrollHeight", {
    configurable: true,
    get() {
      return this.classList.contains("timeline") ? scrollHeight : 0;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get() {
      return this.classList.contains("timeline") ? clientHeight : 0;
    },
  });
  HTMLElement.prototype.scrollTo = vi.fn(function (
    this: HTMLElement,
    optionsOrX?: ScrollToOptions | number,
    y?: number,
  ) {
    this.scrollTop = typeof optionsOrX === "number" ? (y ?? 0) : (optionsOrX?.top ?? 0);
  }) as typeof HTMLElement.prototype.scrollTo;
});

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  if (originalTop) Object.defineProperty(HTMLElement.prototype, "scrollTop", originalTop);
  if (originalHeight) Object.defineProperty(HTMLElement.prototype, "scrollHeight", originalHeight);
  if (originalClient) Object.defineProperty(HTMLElement.prototype, "clientHeight", originalClient);
  HTMLElement.prototype.scrollTo = originalScrollTo;
});

describe("long-thread reading position", () => {
  it("restores a scrolled-away position after remount instead of snapping to latest", () => {
    const props = {
      sessionKey: "long-session",
      items: [assistant("a1")],
      pending: [],
      typing: "",
      showSys: false,
    };
    const first = render(<TimelineView {...props} />);
    const timeline = first.container.querySelector(".timeline") as HTMLElement;
    timeline.scrollTop = 1150;
    fireEvent.scroll(timeline);
    expect(sessionStorage.getItem("arwebui.timelineScroll.long-session")).toBe("1150");
    first.unmount();

    const second = render(<TimelineView {...props} />);
    const restored = second.container.querySelector(".timeline") as HTMLElement;
    expect(restored.scrollTop).toBe(1150);
    expect(second.getByRole("button", { name: "Jump to latest" })).toBeTruthy();
  });

  it("removes the saved position when the reader returns to the bottom", () => {
    const { container } = render(
      <TimelineView sessionKey="bottom-session" items={[assistant("a1")]} pending={[]} typing="" showSys={false} />,
    );
    const timeline = container.querySelector(".timeline") as HTMLElement;
    timeline.scrollTop = 900;
    fireEvent.scroll(timeline);
    expect(sessionStorage.getItem("arwebui.timelineScroll.bottom-session")).toBe("900");

    timeline.scrollTop = 2470;
    fireEvent.scroll(timeline);
    expect(sessionStorage.getItem("arwebui.timelineScroll.bottom-session")).toBeNull();
    expect(container.querySelector(".tl-jump")).toBeNull();
  });

  it("keeps the reading anchor and counts new visible updates until jump-to-latest", () => {
    const base = {
      sessionKey: "live-session",
      pending: [],
      typing: "",
      showSys: false,
    };
    const view = render(<TimelineView {...base} items={[assistant("a1")]} />);
    const timeline = view.container.querySelector(".timeline") as HTMLElement;
    timeline.scrollTop = 1000;
    fireEvent.scroll(timeline);

    view.rerender(<TimelineView {...base} items={[assistant("a1"), assistant("a2")]} />);
    expect(timeline.scrollTop).toBe(1000);
    expect(view.getByRole("button", { name: "1 new update; jump to latest" }).textContent).toContain("1");

    view.rerender(<TimelineView {...base} items={[assistant("a1"), assistant("a2"), assistant("a3")]} />);
    const jump = view.getByRole("button", { name: "2 new updates; jump to latest" });
    expect(timeline.scrollTop).toBe(1000);
    fireEvent.click(jump);
    expect(timeline.scrollTop).toBe(2500);
    expect(sessionStorage.getItem("arwebui.timelineScroll.live-session")).toBeNull();
    expect(view.container.querySelector(".tl-jump")).toBeNull();
  });

  it("restores each session when switching without unmounting the timeline", () => {
    const common = { pending: [], typing: "", showSys: false };
    const view = render(<TimelineView {...common} sessionKey="session-a" items={[assistant("a1")]} />);
    const timeline = view.container.querySelector(".timeline") as HTMLElement;
    timeline.scrollTop = 1250;
    fireEvent.scroll(timeline);

    view.rerender(<TimelineView {...common} sessionKey="session-b" items={[assistant("b1")]} />);
    expect(timeline.scrollTop).toBe(2500);

    view.rerender(<TimelineView {...common} sessionKey="session-a" items={[assistant("a1")]} />);
    expect(timeline.scrollTop).toBe(1250);
    expect(view.getByRole("button", { name: "Jump to latest" })).toBeTruthy();
  });

  it("treats an explicit send as authority to return to latest", () => {
    const view = render(
      <TimelineView sessionKey="send-session" items={[assistant("a1")]} pending={[]} typing="" showSys={false} />,
    );
    const timeline = view.container.querySelector(".timeline") as HTMLElement;
    timeline.scrollTop = 1050;
    fireEvent.scroll(timeline);

    view.rerender(
      <TimelineView
        sessionKey="send-session"
        items={[assistant("a1")]}
        pending={[{ id: 1, text: "new turn", imgs: [], files: 0 }]}
        typing=""
        showSys={false}
      />,
    );
    expect(timeline.scrollTop).toBe(2500);
    expect(sessionStorage.getItem("arwebui.timelineScroll.send-session")).toBeNull();
    expect(view.container.querySelector(".tl-jump")).toBeNull();
  });
});
