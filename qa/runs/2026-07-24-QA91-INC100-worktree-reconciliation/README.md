# QA-91 · INC-100.2 worktree reconciliation

- Date: 2026-07-24 (America/Los_Angeles)
- URL: `http://127.0.0.1:8809/`
- Runtime: production shared-store Web UI
- Store: `~/.local/share/agentrunner/`
- Tested build: `b60e88d2-dirty-231821`
- Health: `ok=true`, `daemonUp=true`, `versionMatch=true`

## Browser evidence

- Viewport: 390×844.
- Dialog geometry: `x=12`, `y=12`, `width=366`, `height=810.59`.
- Explicit close action: 44×44, accessible name `Close keyboard shortcuts`.
- Search geometry: `x=25`, `y=77`, `width=340`, `height=41.59`.
- Search `settings` filtered the list to `Open settings`.
- Full list: `clientHeight=690`, `scrollHeight=2089`, `overflow-y=auto`;
  manual scroll reached `scrollTop=540`.
- Body horizontal overflow: false.
- Closing removed the dialog and restored focus to `Show sidebar`.
- Hard reload restored meaningful Home content.
- Browser warning/error log: `[]`.
- Screenshot: `shortcuts-mobile.png`.

## Automated gates

- Focused Shortcuts suites: 2 files / 19 tests passed.
- Frontend full unit: 86 files / 832 tests passed.
- Frontend production build passed.
- Storybook: 64 passed, 2 skipped, 559 tests passed.
- Full repository gate: `./scripts/check.sh` passed (`lint`, `wiring`, Go tests,
  install; all green).

## Data discipline

No shared session, journal, workspace, or store fixture was closed, deleted, or
cleaned. The retained QA workspace diff identified during reconciliation remains
in its owning session worktree and is not product source.
