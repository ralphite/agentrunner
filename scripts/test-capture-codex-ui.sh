#!/usr/bin/env bash
# Platform-independent contract checks for the macOS Codex screenshot driver.
set -euo pipefail
cd "$(dirname "$0")/.."

driver=qa/capture-codex-ui.sh
bash -n "$driver"

help=$($driver --help)
for surface in new-chat pull-requests sites scheduled plugins; do
  [[ "$help" == *"$surface"* ]] || {
    echo "capture driver help is missing surface: $surface" >&2
    exit 1
  }
done

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

if $driver --surface destructive-target >"$tmpdir/unknown.out" 2>"$tmpdir/unknown.err"; then
  echo "capture driver accepted an unknown surface" >&2
  exit 1
fi
grep -Fq 'unsupported surface: destructive-target' "$tmpdir/unknown.err"

if $driver --settle 31 >"$tmpdir/settle.out" 2>"$tmpdir/settle.err"; then
  echo "capture driver accepted an invalid settle value" >&2
  exit 1
fi
grep -Fq 'whole seconds from 0 to 30' "$tmpdir/settle.err"

if grep -Fq 'System Events' "$driver"; then
  echo "capture driver must not regress to the blocking System Events path" >&2
  exit 1
fi
grep -Fq 'trap close_palette EXIT' "$driver"
# Literal source contract; variable expansion would defeat this assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ -n "$restore_query" && -n "$surface" ]]' "$driver"

echo "test-capture-codex-ui: ok"
