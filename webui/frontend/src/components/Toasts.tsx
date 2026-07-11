import { Info, Warning, X } from "@phosphor-icons/react";
import { useStore } from "../store";

export function Toasts() {
  const { toasts, dismissToast } = useStore();
  return (
    <div className="toasts">
      {toasts.map((t) => (
        <div key={t.id} className={"toast " + t.kind} role="status" onClick={() => dismissToast(t.id)}>
          <span className="toast-ico" aria-hidden="true">
            {t.kind === "error" ? <Warning size={16} weight="fill" /> : <Info size={16} weight="fill" />}
          </span>
          <span className="toast-text">{t.text}</span>
          <button
            className="toast-close"
            aria-label="Dismiss notification"
            onClick={(event) => {
              event.stopPropagation();
              dismissToast(t.id);
            }}
          >
            <X size={13} />
          </button>
        </div>
      ))}
    </div>
  );
}
