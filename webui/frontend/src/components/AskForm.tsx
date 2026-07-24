import { useId, useState } from "react";
import { CheckCircle, Circle, Question } from "@phosphor-icons/react";
import { Input } from "../ui/Field";
import { Button } from "../ui/Button";

// AskForm renders a structured ask_user park (INC-47.2): the questions the
// model posed, each as single/multi-select options and/or a free-text box.
// On submit it builds the 1-based `ar answer` specs the backend forwards.
// A plain single-question ask has no structure — the composer answers those
// with a normal message, so this only renders when questions[] is present.
export interface AskOption {
  label: string;
  description?: string;
}
export interface AskQuestion {
  question: string;
  options?: AskOption[];
  multi_select?: boolean;
  allow_free_text?: boolean;
}

export function AskForm({
  questions,
  onSubmit,
  onSkip,
}: {
  questions: AskQuestion[];
  onSubmit: (specs: string[]) => Promise<void> | void;
  onSkip: () => Promise<void> | void;
}) {
  // selected[q] = set of 1-based option numbers; text[q] = free text.
  const [selected, setSelected] = useState<Record<number, Set<number>>>({});
  const [text, setText] = useState<Record<number, string>>({});
  const [busy, setBusy] = useState(false);
  const formId = useId();

  const toggle = (q: number, opt: number, multi: boolean) => {
    setSelected((prev) => {
      const next = { ...prev };
      const cur = new Set(next[q] ?? []);
      if (multi) {
        cur.has(opt) ? cur.delete(opt) : cur.add(opt);
      } else {
        cur.clear();
        cur.add(opt);
      }
      next[q] = cur;
      return next;
    });
  };

  // A question is answered when it has a selection, or free text (where
  // allowed), or is a bare free-text question with text.
  const answered = (qi: number, q: AskQuestion): boolean => {
    const sel = selected[qi];
    if (sel && sel.size > 0) return true;
    if ((q.allow_free_text || !q.options?.length) && (text[qi] || "").trim()) return true;
    return false;
  };
  const allAnswered = questions.every((q, qi) => answered(qi, q));

  const buildSpecs = (): string[] => {
    const specs: string[] = [];
    questions.forEach((_q, qi) => {
      const sel = selected[qi];
      if (sel && sel.size > 0) {
        specs.push(`${qi + 1}:${[...sel].sort((a, b) => a - b).join(",")}`);
      } else if ((text[qi] || "").trim()) {
        specs.push(`${qi + 1}:text=${text[qi].trim()}`);
      }
    });
    return specs;
  };

  const submit = async () => {
    if (!allAnswered || busy) return;
    setBusy(true);
    try {
      await onSubmit(buildSpecs());
    } finally {
      setBusy(false);
    }
  };
  const skip = async () => {
    if (busy) return;
    setBusy(true);
    try {
      await onSkip();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      className="ask-form"
      role="group"
      aria-label="Question from the agent"
      aria-busy={busy}
    >
      <div className="ask-form-head">
        <Question size={16} weight="bold" /> The agent is asking
      </div>
      {questions.map((q, qi) => (
        <div className="ask-q mt-2 min-w-0" key={qi}>
          <div
            className="ask-q-text mb-1.5 text-[13px] leading-[1.4] text-ink"
            id={`${formId}-question-${qi}`}
          >
            {q.question}
          </div>
          {q.options && q.options.length > 0 && (
            <div className="ask-opts flex min-w-0 flex-col gap-1.5">
              {q.options.map((o, oi) => {
                const on = oi + 1;
                const isSel = selected[qi]?.has(on) ?? false;
                return (
                  <button
                    type="button"
                    className={`ask-opt flex w-full min-w-0 items-start gap-2 rounded-[8px] border px-2.5 py-2 text-left text-[13px] leading-[1.35] text-ink transition-colors ${
                      isSel
                        ? "sel border-blue bg-blue-soft hover:border-blue hover:bg-blue-soft"
                        : "border-line bg-transparent hover:border-dim hover:bg-panel-2"
                    }`}
                    key={oi}
                    onClick={() => toggle(qi, on, !!q.multi_select)}
                    title={o.description || o.label}
                    aria-pressed={isSel}
                  >
                    {isSel ? (
                      <CheckCircle className="mt-[1px] shrink-0 text-blue" size={16} weight="fill" />
                    ) : (
                      <Circle className="mt-[1px] shrink-0 text-dim" size={16} />
                    )}
                    <span className="ask-opt-copy min-w-0 flex-1 [overflow-wrap:anywhere]">
                      <span className="ask-opt-label block font-medium text-ink">{o.label}</span>
                      {o.description && (
                        <span className="ask-opt-desc mt-0.5 block text-[12px] leading-[1.35] text-dim">
                          {o.description}
                        </span>
                      )}
                    </span>
                  </button>
                );
              })}
            </div>
          )}
          {(q.allow_free_text || !q.options?.length) && (
            <Input
              className="ask-free"
              aria-labelledby={`${formId}-question-${qi}`}
              placeholder={q.options?.length ? "…or type an answer" : "Type your answer"}
              value={text[qi] || ""}
              onChange={(e) => setText((prev) => ({ ...prev, [qi]: e.target.value }))}
              onKeyDown={(e) => {
                if (e.key === "Enter" && allAnswered) submit();
              }}
            />
          )}
        </div>
      ))}
      <div className="ask-actions">
        <Button variant="solid" disabled={!allAnswered} loading={busy} onClick={submit}>
          Submit
        </Button>
        <Button variant="outline" disabled={busy} onClick={skip}>
          Skip
        </Button>
      </div>
    </div>
  );
}
