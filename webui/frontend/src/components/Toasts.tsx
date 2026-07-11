import { Info, Warning, X } from "@phosphor-icons/react";
import { useStore } from "../store";

export function Toasts() {
  const { toasts, dismissToast } = useStore();
  return (
    <div className="fixed right-4 bottom-4 flex flex-col gap-2 z-[60] max-w-[440px]">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={
            "px-[14px] py-[10px] rounded-app text-[13px] whitespace-pre-wrap shadow-[0_6px_24px_rgba(0,0,0,0.18)] cursor-pointer " +
            (t.kind === "error" ? "bg-[#7f1d1d] text-white" : "bg-accent text-accent-ink")
          }
          role="status"
          onClick={() => dismissToast(t.id)}
        >
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
