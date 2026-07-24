import type { ReactNode } from "react";

export type HomeSuggestionTone = "blue" | "teal" | "violet" | "green" | "orange";

export interface HomeSuggestion {
  key: string;
  tone: HomeSuggestionTone;
  icon: ReactNode;
  label: string;
  seed: string;
  followups: string[];
}

interface HomeStarterCardProps {
  suggestion: HomeSuggestion;
  onSelect: (suggestion: HomeSuggestion) => void;
  disabled?: boolean;
}

export function HomeStarterCard({
  suggestion,
  onSelect,
  disabled = false,
}: HomeStarterCardProps) {
  return (
    <button
      type="button"
      className="home-empty-card max-[680px]:min-h-[76px] max-[680px]:gap-1 max-[680px]:px-2.5 max-[680px]:py-2"
      disabled={disabled}
      onClick={() => onSelect(suggestion)}
    >
      <span className={`home-empty-card-icon ${suggestion.tone}`} aria-hidden>
        {suggestion.icon}
      </span>
      <span className="home-empty-card-label">{suggestion.label}</span>
    </button>
  );
}

interface IntentSuggestionListProps {
  suggestion: HomeSuggestion;
  onSelect: (followup: string) => void;
}

export function IntentSuggestionList({
  suggestion,
  onSelect,
}: IntentSuggestionListProps) {
  return (
    <div
      className="home-intent-suggestions"
      aria-label={`${suggestion.seed} suggestions`}
    >
      {suggestion.followups.map((followup) => (
        <button
          key={followup}
          type="button"
          className="home-intent-suggestion"
          onClick={() => onSelect(followup)}
        >
          <span className={`home-intent-icon ${suggestion.tone}`} aria-hidden>
            {suggestion.icon}
          </span>
          <span>{followup}</span>
        </button>
      ))}
    </div>
  );
}
