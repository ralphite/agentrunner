import { useStore } from "../store";

export function Toasts() {
  const { toasts, dismissToast } = useStore();
  return (
    <div className="toasts">
      {toasts.map((t) => (
        <div key={t.id} className={"toast " + t.kind} onClick={() => dismissToast(t.id)}>
          {t.text}
        </div>
      ))}
    </div>
  );
}
