// @vitest-environment jsdom
import { useState } from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContextMenu } from "./ContextMenu";
import { MenuItem, MenuLabel } from "./Menu";
import { FocusScope } from "../ui/FocusScope";

afterEach(cleanup);

function FocusContractFixture({
  action,
}: {
  action?: () => void;
}) {
  const [open, setOpen] = useState(true);
  const [returnFocus, setReturnFocus] =
    useState<HTMLButtonElement | null>(null);

  return (
    <>
      <button>Before menu</button>
      <button ref={setReturnFocus} onClick={() => setOpen(true)}>
        Invoking row
      </button>
      {open && returnFocus && (
        <ContextMenu
          x={24}
          y={24}
          ariaLabel="Session actions"
          returnFocus={returnFocus}
          onClose={() => setOpen(false)}
        >
          <MenuLabel>Session actions</MenuLabel>
          <div hidden>
            <MenuItem onClick={() => {}}>Hidden action</MenuItem>
          </div>
          <div aria-hidden="true">
            <MenuItem onClick={() => {}}>ARIA hidden action</MenuItem>
          </div>
          <div style={{ display: "none" }}>
            <MenuItem onClick={() => {}}>CSS hidden action</MenuItem>
          </div>
          <details>
            <MenuItem onClick={() => {}}>Closed details action</MenuItem>
          </details>
          <MenuItem onClick={() => {}} disabled>
            Disabled action
          </MenuItem>
          <MenuItem onClick={action || (() => {})}>First action</MenuItem>
          <MenuItem onClick={() => {}}>Last action</MenuItem>
        </ContextMenu>
      )}
      <button>After menu</button>
    </>
  );
}

describe("ContextMenu focus contract", () => {
  it("owns one enabled roving item and skips every unavailable state", async () => {
    render(<FocusContractFixture />);
    const first = await screen.findByRole("menuitem", {
      name: "First action",
    });
    const last = screen.getByRole("menuitem", { name: "Last action" });

    await waitFor(() => expect(document.activeElement).toBe(first));
    const all = screen.getAllByRole("menuitem", { hidden: true });
    expect(all.filter((item) => item.tabIndex === 0)).toEqual([first]);
    expect(
      screen.getByRole("menuitem", { name: "Disabled action" }).tabIndex,
    ).toBe(-1);
    expect(
      screen.getByRole("menuitem", {
        name: "Closed details action",
        hidden: true,
      }).tabIndex,
    ).toBe(-1);

    fireEvent.keyDown(document, { key: "ArrowUp" });
    expect(document.activeElement).toBe(last);
    expect(all.filter((item) => item.tabIndex === 0)).toEqual([last]);

    fireEvent.keyDown(document, { key: "ArrowDown" });
    expect(document.activeElement).toBe(first);
    fireEvent.keyDown(document, { key: "End" });
    expect(document.activeElement).toBe(last);
    fireEvent.keyDown(document, { key: "Home" });
    expect(document.activeElement).toBe(first);

    last.focus();
    fireEvent.focus(last);
    expect(all.filter((item) => item.tabIndex === 0)).toEqual([last]);
  });

  it("hands Tab and Shift+Tab to the invoking row's adjacent controls", async () => {
    render(<FocusContractFixture />);
    const invoker = screen.getByRole("button", { name: "Invoking row" });
    const first = await screen.findByRole("menuitem", {
      name: "First action",
    });
    await waitFor(() => expect(document.activeElement).toBe(first));

    fireEvent.keyDown(document, { key: "Tab" });
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "After menu" }),
      ),
    );
    expect(screen.queryByRole("menu")).toBeNull();

    fireEvent.click(invoker);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("menuitem", { name: "First action" }),
      ),
    );
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Before menu" }),
      ),
    );
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("restores Escape and plain selections without stealing modal focus", async () => {
    const action = vi.fn();
    const { rerender } = render(<FocusContractFixture action={action} />);
    const invoker = screen.getByRole("button", { name: "Invoking row" });
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("menuitem", { name: "First action" }),
      ),
    );

    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(invoker));

    fireEvent.click(invoker);
    const first = await screen.findByRole("menuitem", {
      name: "First action",
    });
    fireEvent.click(first);
    expect(action).toHaveBeenCalledOnce();
    await waitFor(() => expect(document.activeElement).toBe(invoker));

    function ModalFixture() {
      const [open, setOpen] = useState(true);
      const [dialog, setDialog] = useState(false);
      const [returnFocus, setReturnFocus] =
        useState<HTMLButtonElement | null>(null);
      return (
        <>
          <button ref={setReturnFocus}>Modal invoking row</button>
          {open && returnFocus && (
            <ContextMenu
              x={24}
              y={24}
              ariaLabel="Modal actions"
              returnFocus={returnFocus}
              onClose={() => setOpen(false)}
            >
              <MenuItem onClick={() => setDialog(true)}>Rename…</MenuItem>
            </ContextMenu>
          )}
          {dialog && (
            <FocusScope
              role="dialog"
              aria-label="Rename session"
              initialFocus="input"
              restoreFocus
              onEscape={() => setDialog(false)}
            >
              <input aria-label="Rename session name" />
            </FocusScope>
          )}
        </>
      );
    }

    rerender(<ModalFixture />);
    const rename = await screen.findByRole("menuitem", { name: "Rename…" });
    fireEvent.click(rename);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("textbox", { name: "Rename session name" }),
      ),
    );
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Modal invoking row" }),
      ),
    );
  });

  it("recovers when the active item disappears and when an empty menu loads", async () => {
    function DynamicFixture() {
      const [phase, setPhase] = useState(0);
      const [returnFocus, setReturnFocus] =
        useState<HTMLButtonElement | null>(null);
      return (
        <>
          <button ref={setReturnFocus}>Dynamic invoking row</button>
          <button tabIndex={-1} onClick={() => setPhase((value) => value + 1)}>
            Advance dynamic content
          </button>
          {returnFocus && (
            <ContextMenu
              x={24}
              y={24}
              ariaLabel="Dynamic actions"
              returnFocus={returnFocus}
              onClose={() => {}}
            >
              {phase === 0 && (
                <MenuItem onClick={() => {}}>First dynamic</MenuItem>
              )}
              {phase <= 1 && (
                <MenuItem onClick={() => {}}>Replacement dynamic</MenuItem>
              )}
              {phase >= 3 && (
                <MenuItem onClick={() => {}}>Loaded dynamic</MenuItem>
              )}
            </ContextMenu>
          )}
        </>
      );
    }

    render(<DynamicFixture />);
    const first = await screen.findByRole("menuitem", {
      name: "First dynamic",
    });
    await waitFor(() => expect(document.activeElement).toBe(first));

    fireEvent.click(
      screen.getByRole("button", { name: "Advance dynamic content" }),
    );
    const replacement = await screen.findByRole("menuitem", {
      name: "Replacement dynamic",
    });
    await waitFor(() => expect(document.activeElement).toBe(replacement));
    expect(replacement.tabIndex).toBe(0);

    fireEvent.click(
      screen.getByRole("button", { name: "Advance dynamic content" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("menuitem")).toBeNull(),
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Advance dynamic content" }),
    );
    const loaded = await screen.findByRole("menuitem", {
      name: "Loaded dynamic",
    });
    await waitFor(() => expect(document.activeElement).toBe(loaded));
    expect(loaded.tabIndex).toBe(0);
  });

  it("keeps internal scrolling open and restores on external scroll dismissal", async () => {
    render(<FocusContractFixture />);
    const menu = await screen.findByRole("menu", { name: "Session actions" });
    const first = screen.getByRole("menuitem", { name: "First action" });
    const invoker = screen.getByRole("button", { name: "Invoking row" });
    await waitFor(() => expect(document.activeElement).toBe(first));

    fireEvent.scroll(menu);
    expect(screen.getByRole("menu", { name: "Session actions" })).toBe(menu);

    fireEvent.scroll(window);
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(invoker));
  });

  it("closes an empty dynamic menu on Tab instead of stranding the surface", async () => {
    function EmptyFixture() {
      const [open, setOpen] = useState(true);
      const [showItem, setShowItem] = useState(true);
      const [returnFocus, setReturnFocus] =
        useState<HTMLButtonElement | null>(null);
      return (
        <>
          <button ref={setReturnFocus}>Empty invoking row</button>
          <button tabIndex={-1} onClick={() => setShowItem(false)}>
            Empty the menu
          </button>
          {open && returnFocus && (
            <ContextMenu
              x={24}
              y={24}
              ariaLabel="Empty actions"
              returnFocus={returnFocus}
              onClose={() => setOpen(false)}
            >
              {showItem && (
                <MenuItem onClick={() => {}}>Temporary action</MenuItem>
              )}
            </ContextMenu>
          )}
          <button>After empty menu</button>
        </>
      );
    }

    render(<EmptyFixture />);
    const item = await screen.findByRole("menuitem", {
      name: "Temporary action",
    });
    await waitFor(() => expect(document.activeElement).toBe(item));
    fireEvent.click(screen.getByRole("button", { name: "Empty the menu" }));
    await waitFor(() => expect(screen.queryByRole("menuitem")).toBeNull());

    fireEvent.keyDown(document, { key: "Tab" });
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "After empty menu" }),
    );
  });

  it("preserves positive tabindex and checked-radio order during Tab handoff", async () => {
    function OrderedFixture() {
      const [open, setOpen] = useState(true);
      const [returnFocus, setReturnFocus] =
        useState<HTMLButtonElement | null>(null);
      return (
        <>
          <button tabIndex={1}>Positive first</button>
          <button tabIndex={2}>Positive previous</button>
          <button
            ref={setReturnFocus}
            tabIndex={3}
            onClick={() => setOpen(true)}
          >
            Ordered invoking row
          </button>
          {open && returnFocus && (
            <ContextMenu
              x={24}
              y={24}
              ariaLabel="Ordered actions"
              returnFocus={returnFocus}
              onClose={() => setOpen(false)}
            >
              <MenuItem onClick={() => {}}>Ordered action</MenuItem>
            </ContextMenu>
          )}
          <label>
            <input type="radio" name="destination" /> Unchecked destination
          </label>
          <label>
            <input type="radio" name="destination" defaultChecked /> Checked destination
          </label>
          <button>Default after</button>
        </>
      );
    }

    render(<OrderedFixture />);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("menuitem", { name: "Ordered action" }),
      ),
    );
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Positive previous" }),
    );

    cleanup();

    function RadioFixture() {
      const [returnFocus, setReturnFocus] =
        useState<HTMLButtonElement | null>(null);
      return (
        <>
          <button ref={setReturnFocus}>Radio invoking row</button>
          {returnFocus && (
            <ContextMenu
              x={24}
              y={24}
              ariaLabel="Radio actions"
              returnFocus={returnFocus}
              onClose={() => {}}
            >
              <MenuItem onClick={() => {}}>Radio action</MenuItem>
            </ContextMenu>
          )}
          <label>
            <input type="radio" name="destination" /> Unchecked destination
          </label>
          <label>
            <input type="radio" name="destination" defaultChecked /> Checked destination
          </label>
          <button>After radios</button>
        </>
      );
    }

    render(<RadioFixture />);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("menuitem", { name: "Radio action" }),
      ),
    );
    fireEvent.keyDown(document, { key: "Tab" });
    expect(document.activeElement).toBe(
      screen.getByRole("radio", { name: "Checked destination" }),
    );
  });
});
