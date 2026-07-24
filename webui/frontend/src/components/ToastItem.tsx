import { Info, Warning, X } from "@phosphor-icons/react";
import type { AppState } from "../store";

export type ToastItemModel = AppState["toasts"][number];

interface ToastItemProps {
  toast: ToastItemModel;
  onDismiss: (id: ToastItemModel["id"]) => void;
}

export function ToastItem({ toast, onDismiss }: ToastItemProps) {
  return (
    <div
      className={`toast-card ${toast.kind === "error" ? "err" : "info"}`}
      role="status"
      onClick={() => onDismiss(toast.id)}
    >
      <span className="toast-ic" aria-hidden="true">
        {toast.kind === "error" ? (
          <Warning size={16} weight="fill" />
        ) : (
          <Info size={16} weight="fill" />
        )}
      </span>
      <span className="toast-text">
        {toast.text}
        {toast.details && (
          <details
            className="mt-1"
            onClick={(event) => event.stopPropagation()}
          >
            <summary className="cursor-pointer text-[12px] opacity-80 select-none">
              Details
            </summary>
            <pre className="mt-1 max-h-[180px] max-w-full overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-4 opacity-90">
              {toast.details}
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
          onDismiss(toast.id);
        }}
      >
        <X size={14} />
      </button>
    </div>
  );
}
