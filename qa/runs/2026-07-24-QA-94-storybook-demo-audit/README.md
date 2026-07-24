# QA-94 Storybook human playback / QA demos

## Environment

- Current-source Storybook: `http://127.0.0.1:6011/`
- Shared production: `http://127.0.0.1:8809/`
- Store: `~/.local/share/agentrunner/` (674 retained sessions)
- Retained deep link:
  `#20260723-225218-reply-with-exactly-inc96-brows-2aa84db75337d904`
- No daemon restart, session close/delete, workspace cleanup, or store cleanup.

## Browser result

- Before implementation, production retained Session, Environment, Changes,
  Scheduled and Settings were exercised in the in-app browser with no captured
  warning/error.
- Human pacing sample (`ApprovalCard/DenyReasonOpen`, explicit `human`):
  200ms no reason field; 1.8s empty; 2.9s partial `Command ch`; 5.1s complete.
- Combined-keyboard sample (`SessionView/KeyboardNavigation`, explicit
  `human`): opener focused at 252ms, Ctrl+F reached the search field at 557ms;
  typing started at 2694ms, was still partial at 3000ms, completed at 3100ms,
  and Escape returned focus at 4938ms.
- All six QA playlists loaded `Ready`, rendered a non-empty canonical Story,
  and had no Storybook render error. A preview-side terminal-result handshake
  distinguishes `Ready` from a retryable `Failed to load`; the parent also
  matches the current frame revision so Reset cannot accept a stale result.
- Every playlist passed Play/Pause/Next/Reset/Replay/Autoplay and 2× speed in
  the in-app browser. `Next` aligned both transport and checkpoint text at
  step 2, with counts 8/6/6/6/7/6 (39 canonical checkpoints total).
- The six playlist Stories are part of the Storybook interaction project;
  their shared `play` check scopes checkpoint text to the checkpoint region.
  Scheduled step 2 keeps the selected row `Running`, detail status `Active`,
  and the available action `Pause`.
- 320×640 Running Queued kept every visible 44px control inside the composer.

## Accepted screenshots

- `04-production-retained-session.png`
- `05-production-environment.png`
- `06-production-changes.png`
- `07-production-scheduled.png`
- `08-production-settings.png`
- `09-human-paced-story.jpg`
- `10-qa-demo-session-delivery.jpg`
- `11-qa-demo-changes-artifacts.jpg`
- `12-qa-demo-scheduled-work.jpg`
- `13-composer-phone-no-overflow.jpg`
- `14-demo-session-delivery.jpg`
- `15-demo-attention-permissions.jpg`
- `16-demo-goals-agents-supervision.jpg`
- `17-demo-scheduled-work.jpg`
- `18-demo-changes-artifacts.jpg`
- `19-demo-navigation-recovery.jpg`
- `20-human-keyboard-mid-sequence.jpg`
- `browser-transport-results.json` (all six structured control matrices pass)
- `retained-session.events.jsonl` (shared retained session journal export)
- `retained-session.workspace-diff.json` (completed last-turn diff; empty)

The blank manager calibration capture and two core-demo captures from a
different worktree/port were rejected and moved out of this directory; they
are not evidence.
