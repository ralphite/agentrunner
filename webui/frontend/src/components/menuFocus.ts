const MENU_ITEM_SELECTOR = '[role="menuitem"]';

const TABBABLE_SELECTOR = [
  'a[href]',
  'area[href]',
  'button',
  'input:not([type="hidden"])',
  'select',
  'textarea',
  'iframe',
  'object',
  'embed',
  'audio[controls]',
  'video[controls]',
  'summary',
  '[contenteditable]:not([contenteditable="false"])',
  '[tabindex]',
].join(",");

export function getMenuItems(root: ParentNode | null) {
  return [
    ...(root?.querySelectorAll<HTMLElement>(MENU_ITEM_SELECTOR) ?? []),
  ];
}

export function getAvailableMenuItems(root: ParentNode | null) {
  return getMenuItems(root).filter(isAvailableMenuItem);
}

export function setRovingMenuItem(
  root: ParentNode | null,
  target: HTMLElement | null,
) {
  getMenuItems(root).forEach((item) => {
    item.tabIndex = item === target ? 0 : -1;
  });
}

export function getTabbableElements() {
  const candidates = [
    ...document.querySelectorAll<HTMLElement>(TABBABLE_SELECTOR),
  ].filter(
    (element) =>
      element.tabIndex >= 0 &&
      !element.matches(":disabled") &&
      !hasUnavailableAncestor(element),
  );
  const radios = candidates.filter(
    (element): element is HTMLInputElement =>
      element instanceof HTMLInputElement &&
      element.type === "radio" &&
      element.name !== "",
  );
  const radioStops = new Set<HTMLElement>();
  for (const radio of radios) {
    const group = radios.filter(
      (candidate) =>
        candidate.name === radio.name && candidate.form === radio.form,
    );
    radioStops.add(group.find((candidate) => candidate.checked) ?? group[0]);
  }

  return candidates
    .filter(
      (element) =>
        !(element instanceof HTMLInputElement) ||
        element.type !== "radio" ||
        element.name === "" ||
        radioStops.has(element),
    )
    .map((element, domIndex) => ({ element, domIndex }))
    .sort((a, b) => {
      const aPositive = a.element.tabIndex > 0;
      const bPositive = b.element.tabIndex > 0;
      if (aPositive && bPositive) {
        return (
          a.element.tabIndex - b.element.tabIndex || a.domIndex - b.domIndex
        );
      }
      if (aPositive) return -1;
      if (bPositive) return 1;
      return a.domIndex - b.domIndex;
    })
    .map(({ element }) => element);
}

export function getAdjacentTabbableElement(
  opener: HTMLElement | null,
  panel: HTMLElement | null,
  backwards: boolean,
) {
  if (!opener) return null;
  const candidates = getTabbableElements().filter(
    (element) => !panel?.contains(element),
  );
  const openerIndex = candidates.indexOf(opener);
  if (openerIndex < 0) return opener.isConnected ? opener : null;
  return candidates[openerIndex + (backwards ? -1 : 1)] ?? opener;
}

export function isAvailableMenuItem(element: HTMLElement) {
  return (
    !element.matches(":disabled") &&
    element.getAttribute("aria-disabled") !== "true" &&
    !hasUnavailableAncestor(element, true)
  );
}

function hasUnavailableAncestor(
  element: HTMLElement,
  includeAriaDisabled = false,
) {
  for (
    let current: HTMLElement | null = element;
    current;
    current = current.parentElement
  ) {
    if (
      current instanceof HTMLDetailsElement &&
      !current.open &&
      element !== current
    ) {
      const summary = [...current.children].find(
        (child): child is HTMLElement =>
          child instanceof HTMLElement && child.tagName === "SUMMARY",
      );
      if (!summary?.contains(element)) return true;
    }
    if (
      current.hidden ||
      current.hasAttribute("inert") ||
      current.getAttribute("aria-hidden") === "true" ||
      (includeAriaDisabled &&
        current.getAttribute("aria-disabled") === "true")
    ) {
      return true;
    }
    const style = getComputedStyle(current);
    if (
      style.display === "none" ||
      style.visibility === "hidden" ||
      style.visibility === "collapse" ||
      style.contentVisibility === "hidden"
    ) {
      return true;
    }
  }
  return false;
}
