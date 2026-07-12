import { Info, Warning, X } from "@phosphor-icons/react";
import { useStore } from "../store";

export function Toasts() {
  const { toasts, dismissToast } = useStore();
  return (
    // Sit above the composer/home-indicator on installed iOS (safe-area) and
    // above everything else (z-60). Self-contained flex layout — the old
    // `.toasts .toast` CSS never matched these class names, so icon/text/close
    // rendered unstyled (phone report).
    <div
      className="fixed right-4 flex flex-col gap-2 z-[60] w-[min(440px,calc(100vw-32px))]"
      style={{ bottom: "max(16px, env(safe-area-inset-bottom))" }}
    >
      {toasts.map((t) => (
        <div
          key={t.id}
          className={
            "flex items-start gap-2 px-[14px] py-[10px] rounded-app text-[13px] whitespace-pre-wrap break-words shadow-[0_6px_24px_rgba(0,0,0,0.28)] cursor-pointer " +
            (t.kind === "error" ? "bg-[#7f1d1d] text-white" : "bg-accent text-accent-ink")
          }
          role="status"
          onClick={() => dismissToast(t.id)}
        >
          <span className="shrink-0 mt-[1px]" aria-hidden="true">
            {t.kind === "error" ? <Warning size={16} weight="fill" /> : <Info size={16} weight="fill" />}
          </span>
          <span className="flex-1 min-w-0">{t.text}</span>
          <button
            className="shrink-0 -mr-1 grid place-items-center w-6 h-6 rounded opacity-80 hover:opacity-100"
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
