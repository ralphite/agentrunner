import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { Check } from "@phosphor-icons/react";
import {
  getAvailableMenuItems,
  getMenuItems,
  getTabbableElements,
  setRovingMenuItem,
} from "./menuFocus";
import { useEscapeLayer, useFocusScope } from "../ui/FocusScope";

const PopoverMenuContext = createContext(true);
const SUBMENU_HOVER_OPEN_DELAY_MS = 120;

// Popover is the drop-up menu primitive the composer controls hang off of. It
// anchors a panel to a trigger button, opens *upward* (the composer sits at the
// bottom of the screen), and closes on outside-click / Escape. Kept dependency-
// free and controlled-optional so each control can drive its own open state.
//
// INC-41 ENV-CLIP — the panel is positioned against the *viewport*
// (`position: fixed` + measured coordinates), not against `.pop-wrap`.
//
// Why: an `position: absolute` panel lives inside every ancestor's overflow box,
// so any ancestor that scrolls cuts the menu in half — and a clipped menu is not
// merely invisible, it is *unclickable* (`elementFromPoint` lands on whatever is
// behind it). Round 36 turned the Environment rail into a floating card with
// `overflow: auto` (tw.css) and instantly ate 125px — 56% — of the
// `Commit or push` menu it hosts: two of the three git actions could not be
// reached. `.diffwrap` / `.timeline` are the same trap waiting to spring.
// `position: fixed` takes the viewport as its containing block, so no ancestor
// `overflow` can clip it, whatever the panel is nested in.
//
// Why fixed *in place* rather than a `createPortal` to <body>: the panel keeps
// its DOM home, so the cascade it was authored against keeps applying —
// ancestor-scoped rules (`.home.home-welcome .cx-project-popover` &c. in
// tw.css sizes the New-session project picker) and inherited type/colour would
// silently drop off a portaled node, and every popover would have to re-earn
// them. Fixed-in-place changes exactly one thing (the containing block); the
// stacking context, the CSS context and the focus/click plumbing are untouched.
// The invariant it rests on: no ancestor of a popover may create a containing
// block for fixed descendants (transform / filter / backdrop-filter /
// perspective / will-change / contain). The dev-only guard below shouts if one
// ever appears — that is the day to reach for a portal.
export function Popover({
  trigger,
  children,
  align = "left",
  panelClass = "",
  wrapClass = "",
  panelRole = "menu",
  ariaLabel,
  onOpen,
}: {
  trigger: (open: boolean, toggle: () => void) => React.ReactNode;
  children: (close: () => void) => React.ReactNode;
  align?: "left" | "right";
  panelClass?: string;
  wrapClass?: string;
  panelRole?: "menu" | "dialog";
  ariaLabel?: string;
  onOpen?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [place, setPlace] = useState<Place | null>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const autoFocusedRef = useRef(false);
  const rovingItemRef = useRef<HTMLElement | null>(null);
  const placed = place !== null;
  const triggerElement = () =>
    wrapRef.current?.querySelector<HTMLElement>(
      ":scope > button, :scope > * > button",
    ) ?? null;
  const allMenuItems = () => getMenuItems(panelRef.current);
  const enabledMenuItems = () => getAvailableMenuItems(panelRef.current);
  // A selection is the safest initial keyboard position: opening a menu and
  // pressing Enter must never drift to the first (potentially higher-risk)
  // option merely because it is first in DOM order. PopItem supplies the
  // data attribute so this stays a shared menu-primitive rule, not a
  // composer-specific exception.
  const selectedMenuItem = () =>
    enabledMenuItems().find(
      (item) =>
        item.dataset.popoverActive === "true" ||
        item.getAttribute("aria-current") === "true",
    ) ?? null;
  const focusMenuItem = (target: HTMLElement) => {
    setRovingMenuItem(panelRef.current, target);
    rovingItemRef.current = target;
    target.focus();
  };
  // An item selection is a completed visit to this temporary surface. Return
  // keyboard focus to the trigger after React removes the chosen row; outside
  // clicks and anchor-loss keep their own target.
  const close = () => {
    setPlace(null);
    setOpen(false);
    const focusTrigger = () => {
      const active = document.activeElement;
      // A selected action may deliberately hand focus to a newly mounted
      // surface (for example More → Show Environment → its Close button).
      // Restore only while focus is still inside this temporary popover, on
      // its trigger, or stranded on <body>; never steal an explicit handoff.
      if (
        active instanceof HTMLElement &&
        active !== document.body &&
        !wrapRef.current?.contains(active) &&
        !panelRef.current?.contains(active)
      ) {
        return;
      }
      triggerElement()?.focus();
    };
    // Selection handlers run after pointer focus has already settled. Restore
    // synchronously so keyboard continuation is never stranded on <body>.
    focusTrigger();
    // A background browser tab may throttle requestAnimationFrame indefinitely,
    // leaving keyboard focus on <body> after a menu selection. A zero-delay
    // callback still runs after React commits the closed panel and works in both
    // visible product tabs and headless Storybook workers.
    window.setTimeout(focusTrigger, 0);
  };

  // Popovers can live inside persistent focus scopes (most visibly the mobile
  // sidebar). Register the open panel as the top dismissible layer so Escape
  // closes only this temporary surface instead of bubbling to and collapsing
  // its parent. This is deliberately separate from Tab containment: menus hand
  // Tab back to the page, while dialog popovers keep their existing controls.
  useEscapeLayer(
    () => {
      setPlace(null);
      setOpen(false);
      triggerElement()?.focus();
    },
    open,
  );

  // Dialog popovers are temporary choice surfaces, not ordinary page content.
  // They must keep Tab inside their own controls until the user chooses, closes,
  // or escapes them. Menus deliberately retain their existing Tab handoff
  // below, so this only changes panels that already declare dialog semantics.
  useFocusScope(panelRef, {
    initialFocus:
      "[data-popover-autofocus], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), button:not([disabled])",
    restoreFocus: false,
    trapTab: true,
    enabled: open && placed && panelRole === "dialog",
  });

  // Measure the anchor, then pin the panel to those viewport coordinates.
  //
  // Flip: the composer sits near the top on the Home hero (menus would overflow
  // above the viewport) but at the bottom in a session. Drop toward the larger
  // side, and cap the panel to the space that side actually has (W13: a fixed
  // max-height taller than the room above still overflowed past the top of the
  // viewport). Horizontally the panel starts at the anchor's aligned edge and is
  // then clamped into the viewport — the same correction the old `marginLeft` /
  // `marginRight` nudge made, now expressible directly because the panel owns
  // absolute coordinates instead of an offset from its wrapper.
  const position = useCallback(() => {
    const el = wrapRef.current;
    const panel = panelRef.current;
    if (!el || !panel) return;
    const rect = el.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const above = rect.top;
    const below = vh - rect.bottom;
    const drop: Drop = below > above ? "down" : "up";
    const width = panel.offsetWidth;
    const left = clamp(align === "left" ? rect.left : rect.right - width, PAD, Math.max(PAD, vw - PAD - width));
    setPlace({
      drop,
      left,
      top: drop === "down" ? rect.bottom + GAP : undefined,
      bottom: drop === "up" ? vh - rect.top + GAP : undefined,
      // A hard 160px floor made compact menus escape short Storybook canvases
      // and small split panes. Respect the actual larger side of the viewport;
      // the panel already scrolls when its content genuinely needs more room.
      maxH: Math.max(
        MIN_PANEL_HEIGHT,
        (drop === "down" ? below : above) - GAP - PAD,
      ),
    });
  }, [align]);

  useLayoutEffect(() => {
    if (!open) {
      autoFocusedRef.current = false;
      rovingItemRef.current = null;
      return;
    }
    warnIfClipped(wrapRef.current);
    position();
  }, [open, position]);

  useLayoutEffect(() => {
    // Before placement the panel is visibility:hidden, and browsers refuse to
    // focus descendants of a hidden box. `placed` flips in the positioning
    // layout effect; on that commit, focus synchronously before paint. Never
    // steal focus again during scroll/resize re-measures.
    if (!open || !placed || autoFocusedRef.current) return;
    const target =
      panelRole === "menu"
        ? selectedMenuItem() ?? enabledMenuItems()[0]
        : panelRef.current?.querySelector<HTMLElement>(
            "[data-popover-autofocus]",
          );
    if (!target) return;
    if (panelRole === "menu") focusMenuItem(target);
    else target.focus();
    if (document.activeElement === target) autoFocusedRef.current = true;
    // Testing-library completes its synthetic pointer sequence after React
    // layout effects and can restore focus to the trigger. Reassert once after
    // the event callback; the connected check makes this harmless if the popover
    // closed in the meantime.
    window.setTimeout(() => {
      if (
        !target.isConnected ||
        document.activeElement === target ||
        panelRef.current?.contains(document.activeElement)
      ) {
        return;
      }
      if (panelRole === "menu") focusMenuItem(target);
      else target.focus();
      if (document.activeElement === target) autoFocusedRef.current = true;
    }, 50);
  }, [open, panelRole, placed]);

  // Menu contents can change while the temporary surface stays open (loading,
  // filtering, permissions). Keep exactly one available item in the roving tab
  // order and recover focus if the current item disappears or becomes
  // unavailable. Attribute observation deliberately excludes tabindex so our
  // own normalization cannot create an observer loop.
  useEffect(() => {
    const panel = panelRef.current;
    if (!open || !placed || panelRole !== "menu" || !panel) return;

    const syncRovingItem = () => {
      const items = enabledMenuItems();
      const active =
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null;
      const previous = rovingItemRef.current;
      const target =
        (active && items.includes(active) ? active : null) ||
        (previous && items.includes(previous) ? previous : null) ||
        items.find((item) => item.tabIndex === 0) ||
        items[0] ||
        null;

      allMenuItems().forEach((item) => {
        item.tabIndex = item === target ? 0 : -1;
      });
      rovingItemRef.current = target;
      if (!target) {
        autoFocusedRef.current = false;
        return;
      }

      const previousBecameUnavailable =
        !!previous &&
        (!previous.isConnected || !items.includes(previous));
      if (!autoFocusedRef.current || previousBecameUnavailable) {
        target.focus();
        if (document.activeElement === target) {
          autoFocusedRef.current = true;
        }
      }
    };

    syncRovingItem();
    const observer = new MutationObserver(syncRovingItem);
    observer.observe(panel, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: [
        "aria-disabled",
        "aria-hidden",
        "class",
        "disabled",
        "hidden",
        "inert",
        "style",
      ],
    });
    return () => observer.disconnect();
  }, [open, panelRole, placed]);

  // A viewport-pinned panel does not ride its scroller, so re-measure whenever
  // anything moves (capture phase: the scroll may be an inner pane, not the
  // window). If the anchor itself scrolls out of sight, the menu has nothing
  // left to hang off — close it rather than leave it floating over the page.
  useEffect(() => {
    if (!open) return;
    let raf = 0;
    const follow = () => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        const rect = wrapRef.current?.getBoundingClientRect();
        if (!rect) return;
        if (rect.bottom < 0 || rect.top > window.innerHeight) {
          setPlace(null);
          setOpen(false);
        }
        else position();
      });
    };
    window.addEventListener("scroll", follow, true);
    window.addEventListener("resize", follow);
    return () => {
      if (raf) cancelAnimationFrame(raf);
      window.removeEventListener("scroll", follow, true);
      window.removeEventListener("resize", follow);
    };
  }, [open, position]);

  const toggle = () => {
    if (open) {
      setPlace(null);
      setOpen(false);
      return;
    }
    onOpen?.();
    setPlace(null);
    setOpen(true);
  };

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) {
        setPlace(null);
        setOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (panelRole !== "menu") return;
      if (e.key === "Tab") {
        const trigger = triggerElement();
        if (!trigger) return;
        const panel = panelRef.current;
        const candidates = getTabbableElements().filter(
          (element) => !panel?.contains(element),
        );
        const triggerIndex = candidates.indexOf(trigger);
        const adjacent =
          triggerIndex < 0
            ? null
            : candidates[triggerIndex + (e.shiftKey ? -1 : 1)] ?? null;
        const target = adjacent ?? trigger;
        e.preventDefault();
        setPlace(null);
        setOpen(false);
        // Move from the temporary menu as though focus had remained on its
        // trigger in the page's tab order. The target is outside the panel and
        // survives its unmount.
        target?.focus();
        window.setTimeout(() => {
          if (target?.isConnected) target.focus();
        }, 0);
        return;
      }
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
      const items = enabledMenuItems();
      if (!items.length) return;
      const active = document.activeElement as HTMLElement | null;
      const index = items.indexOf(active as HTMLElement);
      let next = 0;
      if (e.key === "End") next = items.length - 1;
      else if (e.key === "Home") next = 0;
      else if (e.key === "ArrowUp") next = index <= 0 ? items.length - 1 : index - 1;
      else next = index < 0 || index === items.length - 1 ? 0 : index + 1;
      e.preventDefault();
      focusMenuItem(items[next]);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, panelRole]);

  const onKeyDownCapture = (event: React.KeyboardEvent) => {
    // ArrowDown is the established keyboard opener for both anchored menus and
    // picker dialogs. Once open, only menu panels adopt arrow-key roving.
    if (open || event.key !== "ArrowDown") return;
    const target = event.target as HTMLElement;
    if (!target.closest("button")) return;
    event.preventDefault();
    onOpen?.();
    setPlace(null);
    setOpen(true);
  };

  return (
    <div className={`pop-wrap${wrapClass ? ` ${wrapClass}` : ""}`} ref={wrapRef} onKeyDownCapture={onKeyDownCapture}>
      {trigger(open, toggle)}
      {open && (
        <PopoverMenuContext.Provider value={panelRole === "menu"}>
          <div
            ref={panelRef}
            // `pop-right` names the small right-side accessory inside a menu
            // row. Alignment must use its own namespace or that 18px accessory
            // rule collapses every right-aligned popover.
            className={`pop-panel pop-align-${align} pop-${place?.drop ?? "up"} ${panelClass}`}
            role={panelRole}
            aria-label={ariaLabel}
            onFocusCapture={(event) => {
              if (panelRole !== "menu") return;
              const item = (event.target as HTMLElement).closest<HTMLElement>(
                '[role="menuitem"]',
              );
              if (item && enabledMenuItems().includes(item)) {
                allMenuItems().forEach((candidate) => {
                  candidate.tabIndex = candidate === item ? 0 : -1;
                });
                rovingItemRef.current = item;
              }
            }}
            style={{
              // Every offset is stated, none inherited: the stylesheet's
              // `.pop-up { bottom: calc(100% + 8px) }` / `.pop-align-right { right: 0 }`
              // are written for an absolute panel and would mean *the viewport's*
              // edge once the panel is fixed. The classes stay (they still carry
              // the animation and are what the CSS hooks read); the geometry is
              // ours. The first render has nothing to measure yet — it is hidden,
              // laid out at its static position, measured, and placed inside the
              // same layout pass, so it never paints in the wrong spot.
              position: "fixed",
              left: place?.left,
              right: "auto",
              top: place?.drop === "down" ? place.top : "auto",
              bottom: place?.drop === "up" ? place.bottom : "auto",
              maxHeight: place?.maxH,
              maxWidth: `calc(100vw - ${PAD * 2}px)`,
              visibility: place ? undefined : "hidden",
            }}
          >
            {children(close)}
          </div>
        </PopoverMenuContext.Provider>
      )}
    </div>
  );
}

type Drop = "up" | "down";
type Place = { drop: Drop; left: number; top?: number; bottom?: number; maxH: number };

const PAD = 8; // breathing room between the panel and the viewport edge
const GAP = 8; // between the anchor and the panel
const MIN_PANEL_HEIGHT = 48;

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi);

// The one thing that can still clip a fixed panel: an ancestor that makes itself
// the containing block for fixed descendants. Nothing in the app does today
// (checked live, round 39); this is the tripwire for the day someone adds a
// `transform` to a scroller and re-opens ENV-CLIP without knowing it. Dev only —
// it costs a walk up the tree per open and says nothing when all is well.
// (cast: the project ships no `vite/client` types, and this is the only
// import.meta.env reader in src — not worth a d.ts of its own.)
const DEV = (import.meta as unknown as { env?: { DEV?: boolean } }).env?.DEV === true;

function warnIfClipped(el: HTMLElement | null) {
  if (!DEV || !el) return;
  for (let p = el.parentElement; p && p !== document.body; p = p.parentElement) {
    const s = getComputedStyle(p);
    const culprit = [
      ["transform", s.transform],
      ["perspective", s.perspective],
      ["filter", s.filter],
      ["backdrop-filter", s.backdropFilter],
      ["will-change", s.willChange],
      ["contain", s.contain],
    ].find(([, v]) => v && v !== "none" && v !== "auto" && v !== "normal");
    if (culprit) {
      console.warn(
        `Popover: ancestor <${p.tagName.toLowerCase()}.${p.className}> sets ${culprit[0]}: ${culprit[1]}, ` +
          `which makes it the containing block for the fixed panel — the panel can be clipped and become unclickable (INC-41 ENV-CLIP). ` +
          `Move that property, or portal the panel out.`,
      );
      return;
    }
  }
}

// PopSection / PopItem / PopHint are the building blocks used inside a Popover
// panel — a labelled group, a selectable row (with optional check + description),
// and a small footer hint.
export function PopSection({ label, children }: { label?: string; children: React.ReactNode }) {
  return (
    <div className="pop-section">
      {label && <div className="pop-section-label">{label}</div>}
      {children}
    </div>
  );
}

export function PopItem({
  onClick,
  onHoverOpen,
  onArrowRight,
  onArrowLeft,
  submenu,
  active,
  icon,
  title,
  desc,
  right,
  danger,
  disabled,
  ariaLabel,
  className = "",
}: {
  onClick?: () => void;
  onHoverOpen?: () => void;
  onArrowRight?: () => void;
  onArrowLeft?: () => void;
  submenu?: boolean;
  active?: boolean;
  icon?: React.ReactNode;
  title: React.ReactNode;
  desc?: React.ReactNode;
  right?: React.ReactNode;
  danger?: boolean;
  disabled?: boolean;
  ariaLabel?: string;
  className?: string;
}) {
  const inMenu = useContext(PopoverMenuContext);
  const hoverOpenTimer = useRef<number | null>(null);
  const clearHoverOpenTimer = () => {
    if (hoverOpenTimer.current === null) return;
    window.clearTimeout(hoverOpenTimer.current);
    hoverOpenTimer.current = null;
  };
  useEffect(() => () => clearHoverOpenTimer(), []);
  return (
    <button
      type="button"
      className={
        "pop-item" +
        (active ? " active" : "") +
        (danger ? " danger" : "") +
        (disabled ? " disabled" : "") +
        (className ? ` ${className}` : "")
      }
      onClick={() => {
        clearHoverOpenTimer();
        onClick?.();
      }}
      onMouseMove={(event) => {
        if (disabled) return;
        if (!onHoverOpen || hoverOpenTimer.current !== null || event.buttons !== 0) return;
        hoverOpenTimer.current = window.setTimeout(() => {
          hoverOpenTimer.current = null;
          onHoverOpen();
        }, SUBMENU_HOVER_OPEN_DELAY_MS);
      }}
      onMouseLeave={clearHoverOpenTimer}
      onKeyDown={(event) => {
        clearHoverOpenTimer();
        if (disabled) return;
        if (event.key === "ArrowRight" && onArrowRight) {
          event.preventDefault();
          onArrowRight();
          return;
        }
        if (event.key === "ArrowLeft" && onArrowLeft) {
          event.preventDefault();
          onArrowLeft();
        }
      }}
      disabled={disabled}
      role={inMenu ? "menuitem" : undefined}
      tabIndex={inMenu ? -1 : undefined}
      aria-label={ariaLabel}
      aria-current={active ? "true" : undefined}
      data-popover-active={active ? "true" : undefined}
      aria-haspopup={submenu ? "menu" : undefined}
      aria-expanded={submenu ? false : undefined}
    >
      {icon !== undefined && <span className="pop-ico">{icon}</span>}
      <span className="pop-body">
        <span className="pop-title">{title}</span>
        {desc && <span className="pop-desc">{desc}</span>}
      </span>
      {right !== undefined ? <span className="pop-right">{right}</span> : active ? <span className="pop-check"><Check size={14} /></span> : null}
    </button>
  );
}
