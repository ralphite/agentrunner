import { ArrowLeft, MagnifyingGlass } from "@phosphor-icons/react";

// INC-41 L2 · A session id that the daemon doesn't know (typo'd deep link,
// deleted store, stale bookmark) used to render a fully functional-looking
// conversation — empty timeline, live composer, spinners that never settle.
// This is the honest terminal state for that fetch: say it's gone, and offer
// the one route out. Shaped like `.tl-empty` (icon · title · sub · action).
export function SessionNotFound({ sid, onBack }: { sid: string; onBack: () => void }) {
  return (
    <div className="tl-empty tl-notfound" role="alert">
      <MagnifyingGlass size={26} weight="light" />
      <b>Task not found</b>
      <span>
        No task matches <code className="tl-notfound-id">{sid}</code>. The link may be out of date, or the
        task was removed from this machine's store.
      </span>
      <button type="button" className="tl-empty-cta" onClick={onBack}>
        <ArrowLeft size={14} /> Back to all tasks
      </button>
    </div>
  );
}
