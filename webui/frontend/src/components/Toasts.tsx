import { useStore } from "../store";
import { ToastItem } from "./ToastItem";

export function Toasts() {
  const { toasts, dismissToast } = useStore();
  return (
    <div className="toast-stack">
      {toasts.map((t) => (
        <ToastItem
          key={t.id}
          toast={t}
          onDismiss={dismissToast}
        />
      ))}
    </div>
  );
}
