import { useState } from "react";
import { CheckCircle, Circle, Question } from "@phosphor-icons/react";

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
    <div className="ask-form" role="group" aria-label="Question from the agent">
      <div className="ask-form-head">
        <Question size={16} weight="bold" /> The agent is asking
      </div>
      {questions.map((q, qi) => (
        <div className="ask-q" key={qi}>
          <div className="ask-q-text">{q.question}</div>
          {q.options && q.options.length > 0 && (
            <div className="ask-opts">
              {q.options.map((o, oi) => {
                const on = oi + 1;
                const isSel = selected[qi]?.has(on) ?? false;
                return (
                  <button
                    type="button"
                    className={"ask-opt" + (isSel ? " sel" : "")}
                    key={oi}
                    onClick={() => toggle(qi, on, !!q.multi_select)}
                    title={o.description || o.label}
                  >
                    {isSel ? <CheckCircle size={15} weight="fill" /> : <Circle size={15} />}
                    <span className="ask-opt-label">{o.label}</span>
                    {o.description && <span className="ask-opt-desc">{o.description}</span>}
                  </button>
                );
              })}
            </div>
          )}
          {(q.allow_free_text || !q.options?.length) && (
            <input
              className="ask-free"
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
        <button className="primary" disabled={!allAnswered || busy} onClick={submit}>
          Submit
        </button>
        <button disabled={busy} onClick={skip}>
          Skip
        </button>
      </div>
    </div>
  );
}
