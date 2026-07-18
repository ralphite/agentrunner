import { Info, Warning, X } from "@phosphor-icons/react";
import { useStore } from "../store";

export function Toasts() {
  const { toasts, dismissToast } = useStore();
  return (
    <div className="toast-stack">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`toast-card ${t.kind === "error" ? "err" : "info"}`}
          role="status"
          onClick={() => dismissToast(t.id)}
        >
          <span className="toast-ic" aria-hidden="true">
            {t.kind === "error" ? <Warning size={16} weight="fill" /> : <Info size={16} weight="fill" />}
          </span>
          <span className="toast-text">
            {t.text}
            {t.details && (
              /* G36 余项: the raw CLI/git stderr stays out of the sentence but
                 one tap away. stopPropagation — toggling the disclosure must
                 not dismiss the toast it belongs to. */
              <details className="mt-1" onClick={(e) => e.stopPropagation()}>
                <summary className="cursor-pointer text-[12px] opacity-80 select-none">Details</summary>
                <pre className="mt-1 max-h-[180px] max-w-full overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-4 opacity-90">
                  {t.details}
                </pre>
              </details>
            )}
          </span>
          <button
            type="button"
            className="toast-close"
            aria-label="Dismiss notification"
            onClick={(event) => {
              event.stopPropagation();
              dismissToast(t.id);
            }}
          >
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}
