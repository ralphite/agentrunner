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

const PopoverMenuContext = createContext(true);

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
  // An item selection is a completed visit to this temporary surface. Return
  // keyboard focus to the trigger after React removes the chosen row; outside
  // clicks and anchor-loss use setOpen directly so they keep their own target.
  const close = () => {
    setOpen(false);
    requestAnimationFrame(() => {
      wrapRef.current?.querySelector<HTMLElement>(":scope > button, :scope > * > button")?.focus();
    });
  };

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
    const drop: Drop = above < 360 && below > above ? "down" : "up";
    const width = panel.offsetWidth;
    const left = clamp(align === "left" ? rect.left : rect.right - width, PAD, Math.max(PAD, vw - PAD - width));
    setPlace({
      drop,
      left,
      top: drop === "down" ? rect.bottom + GAP : undefined,
      bottom: drop === "up" ? vh - rect.top + GAP : undefined,
      maxH: Math.max(160, (drop === "down" ? below : above) - 16),
    });
  }, [align]);

  useLayoutEffect(() => {
    if (!open) {
      setPlace((p) => (p ? null : p));
      return;
    }
    warnIfClipped(wrapRef.current);
    position();
    requestAnimationFrame(() => {
      wrapRef.current?.querySelector<HTMLElement>("[data-popover-autofocus]")?.focus();
    });
  }, [open, position]);

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
        if (rect.bottom < 0 || rect.top > window.innerHeight) setOpen(false);
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
    if (!open) onOpen?.();
    setOpen((value) => !value);
  };

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        wrapRef.current?.querySelector<HTMLElement>(":scope > button, :scope > * > button")?.focus();
        return;
      }
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) return;
      const items = [...(wrapRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])') || [])];
      if (!items.length) return;
      const active = document.activeElement as HTMLElement | null;
      const index = items.indexOf(active as HTMLElement);
      let next = 0;
      if (e.key === "End") next = items.length - 1;
      else if (e.key === "Home") next = 0;
      else if (e.key === "ArrowUp") next = index <= 0 ? items.length - 1 : index - 1;
      else next = index < 0 || index === items.length - 1 ? 0 : index + 1;
      e.preventDefault();
      items[next].focus();
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const onKeyDownCapture = (event: React.KeyboardEvent) => {
    if (open || event.key !== "ArrowDown") return;
    const target = event.target as HTMLElement;
    if (!target.closest("button")) return;
    event.preventDefault();
    onOpen?.();
    setOpen(true);
    requestAnimationFrame(() => {
      const auto = wrapRef.current?.querySelector<HTMLElement>("[data-popover-autofocus]");
      const first = wrapRef.current?.querySelector<HTMLElement>('[role="menuitem"]:not([disabled])');
      (auto || first)?.focus();
    });
  };

  return (
    <div className={`pop-wrap${wrapClass ? ` ${wrapClass}` : ""}`} ref={wrapRef} onKeyDownCapture={onKeyDownCapture}>
      {trigger(open, toggle)}
      {open && (
        <PopoverMenuContext.Provider value={panelRole === "menu"}>
          <div
            ref={panelRef}
            className={`pop-panel pop-${align} pop-${place?.drop ?? "up"} ${panelClass}`}
            role={panelRole}
            aria-label={ariaLabel}
            style={{
              // Every offset is stated, none inherited: the stylesheet's
              // `.pop-up { bottom: calc(100% + 8px) }` / `.pop-right { right: 0 }`
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
  active,
  icon,
  title,
  desc,
  right,
  danger,
  highlight,
  disabled,
}: {
  onClick?: () => void;
  active?: boolean;
  icon?: React.ReactNode;
  title: React.ReactNode;
  desc?: React.ReactNode;
  right?: React.ReactNode;
  danger?: boolean;
  highlight?: boolean;
  disabled?: boolean;
}) {
  const inMenu = useContext(PopoverMenuContext);
  return (
    <button
      type="button"
      className={"pop-item" + (danger ? " danger" : "") + (highlight ? " hl" : "") + (disabled ? " disabled" : "")}
      onClick={onClick}
      disabled={disabled}
      role={inMenu ? "menuitem" : undefined}
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
