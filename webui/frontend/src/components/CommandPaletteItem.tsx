import type { Ref } from "react";
import { modLabel } from "../shortcuts";

export interface CommandPaletteItemModel {
  id: string;
  label: string;
  hint?: string;
  group: string;
  quickNum?: number;
  session?: boolean;
  dot?: string;
  dotTitle?: string;
  actionCount?: number;
  run: () => void;
}

interface CommandPaletteItemProps {
  item: CommandPaletteItemModel;
  selected: boolean;
  buttonRef?: Ref<HTMLButtonElement>;
  onSelect: () => void;
  onHover: () => void;
}

export function CommandPaletteItem({
  item,
  selected,
  buttonRef,
  onSelect,
  onHover,
}: CommandPaletteItemProps) {
  return (
    <button
      type="button"
      id={item.id}
      ref={buttonRef}
      className={`cmdk-item${selected ? " sel" : ""}`}
      role="option"
      aria-selected={selected}
      onMouseEnter={onHover}
      onClick={onSelect}
    >
      {item.session &&
        (item.actionCount ? (
          <span
            className="status-count"
            title={item.dotTitle}
            aria-hidden="true"
          >
            {item.actionCount}
          </span>
        ) : (
          <span
            className={`status-dot${item.dot ? ` ${item.dot}` : ""}`}
            style={item.dot ? undefined : { visibility: "hidden" }}
            title={item.dotTitle}
            aria-hidden="true"
          />
        ))}
      <span className="cmdk-label">{item.label}</span>
      {item.hint && <span className="cmdk-hint">{item.hint}</span>}
      {item.quickNum && item.quickNum <= 9 && (
        <span className="cmdk-kbd" aria-hidden="true">
          {modLabel}
          {item.quickNum}
        </span>
      )}
    </button>
  );
}
