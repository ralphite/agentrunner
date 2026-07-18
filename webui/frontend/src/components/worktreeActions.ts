import { AR } from "../api";
import { useStore } from "../store";

// INC-41 RD-C · the worktree's lifecycle actions (INC-49), in one place.
//
// They were written inside DiffView, so they existed only *there*: the review
// rail's `…` menu could apply a worktree back onto its project or remove it,
// and the Environment panel — the rail that literally has a row named
// `Worktree` — could do nothing with it but copy its path. Two rails, one
// mutually exclusive with the other (they share the same slot), and the actions
// lived in exactly one of them.
//
// So they move here, verbatim: same endpoints, same confirmation modals, same
// toasts, same refresh-on-success. DiffView and SupervisionPanel now call the
// same code, which is the whole point — a destructive action must not have two
// implementations that can drift apart on the question of whether it asks first.
//
// `onDone` is the caller's re-read of its own git state (DiffView reloads the
// diff; the Environment section reloads its rows), and `setBusy` is the caller's
// own in-flight flag, so its rows/buttons go inert exactly as they did before.
export interface WorktreeActionsOpts {
  sid: string;
  // Re-read git after a successful mutation.
  onDone?: () => void;
  // Raised for the duration of a mutation, so the caller can disable its rows.
  setBusy?: (busy: boolean) => void;
}

export interface WorktreeActions {
  // Apply the worktree's changes back onto its main checkout — confirms first.
  applyBack: (mainRepo: string) => void;
  // Delete the worktree checkout and prune it — confirms first, and confirms a
  // SECOND time (force) when the backend refuses because work is unapplied.
  removeWorktree: () => void;
}

export function useWorktreeActions({ sid, onDone, setBusy }: WorktreeActionsOpts): WorktreeActions {
  const toast = useStore((s) => s.toast);
  const openModal = useStore((s) => s.openModal);

  const busy = (on: boolean) => setBusy?.(on);
  const done = () => onDone?.();

  // Apply the worktree's changes back onto its main checkout (INC-49) — Codex's
  // "Apply changes". Lands unstaged in the project so the user reviews there; a
  // conflict is reported and the project is left untouched.
  const applyBack = (mainRepo: string) => {
    openModal({
      kind: "confirm",
      title: "Apply changes to project?",
      body: `Applies this worktree's changes onto ${mainRepo} (left unstaged for you to review and commit there). If they don't apply cleanly, nothing is changed and the conflict is reported.`,
      confirmLabel: "Apply changes",
      onConfirm: async () => {
        busy(true);
        try {
          const r = await AR.applyWorktree(sid);
          toast(r.applied ? "applied to project — review the changes there" : "no changes to apply", "info");
          done();
        } catch (e: any) {
          toast(e.message, "error", e.details);
        } finally {
          busy(false);
        }
      },
    });
  };

  // Remove the worktree checkout + prune (INC-49). A dirty worktree is refused
  // first; the backend's structured refusal turns into a force confirmation so
  // unapplied work is never silently discarded.
  const forceRemove = async () => {
    busy(true);
    try {
      await AR.removeWorktree(sid, true);
      toast("worktree removed", "info");
      done();
    } catch (e: any) {
      toast(e.message, "error", e.details);
    } finally {
      busy(false);
    }
  };
  const removeWorktree = () => {
    openModal({
      kind: "confirm",
      title: "Remove worktree?",
      body: "Deletes this isolated checkout and prunes it from git. Your project and any applied changes are unaffected.",
      confirmLabel: "Remove worktree",
      danger: true,
      onConfirm: async () => {
        busy(true);
        try {
          await AR.removeWorktree(sid, false);
          toast("worktree removed", "info");
          done();
        } catch (e: any) {
          if (/unapplied changes/.test(e.message)) {
            // The confirm modal auto-closes itself right after this handler
            // resolves, which would clobber a modal opened synchronously here —
            // so defer the force prompt to the next tick.
            setTimeout(
              () =>
                openModal({
                  kind: "confirm",
                  title: "Discard unapplied changes?",
                  body: "This worktree has changes that haven't been applied to the project. Removing it deletes them permanently. Apply the changes first if you want to keep them.",
                  confirmLabel: "Delete anyway",
                  danger: true,
                  onConfirm: forceRemove,
                }),
              0,
            );
          } else {
            toast(e.message, "error", e.details);
          }
        } finally {
          busy(false);
        }
      },
    });
  };

  return { applyBack, removeWorktree };
}
