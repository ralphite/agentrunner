#!/usr/bin/env bash
# v2/qa/lib.sh — shared helpers for real-API QA gates. Source, don't run.
# Provides: qa_setup <profile>, qa_daemon, qa_new, qa_wait_turns, count_type.
# Requires GEMINI_API_KEY. Callers set AR (binary) and QA (name) first.

count_type() { # type file — errexit/pipefail-safe (grep -c → 1 on 0 matches)
  local n; n="$(grep -c "\"type\":\"$1\"" "$2" 2>/dev/null)" || n=0
  printf '%s' "${n:-0}"
}

qa_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

qa_setup() { # profile — sets $WORK, $WS, $XDG_DATA_HOME, loads .env
  [ -f "$qa_here/../../.env" ] && { set -a; . "$qa_here/../../.env"; set +a; }
  [ -n "${GEMINI_API_KEY:-}" ] || { echo "${QA:-QA}: GEMINI_API_KEY unset" >&2; exit 2; }
  WORK="$(mktemp -d)"
  export XDG_DATA_HOME="$WORK/xdg"
  WS="$WORK/ws"
  "$qa_here/ws.sh" prepare "$1" "$WS" >/dev/null
  trap 'kill ${DPID:-0} 2>/dev/null || true; sleep 0.3; rm -rf "$WORK" 2>/dev/null || true' EXIT
}

qa_spec() { # writes $WORK/base.yaml with the given tool list ($1)
  cat > "$WORK/base.yaml" <<YAML
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是一个严谨的编码助手。简洁作答,严格按用户指令行动,基于会话已有
  上下文继续。
tools: [${1:-read_file, bash}]
permissions:
  - { action: allow }
YAML
}

qa_daemon() { # starts the daemon, waits for its socket; sets $SOCK
  "$AR" daemon >"$WORK/daemon.log" 2>&1 &
  DPID=$!
  SOCK="$XDG_DATA_HOME/agentrunner/daemon.sock"
  local i; for i in $(seq 1 100); do [ -S "$SOCK" ] && return 0; sleep 0.1; done
  echo "${QA}: daemon socket never appeared" >&2; cat "$WORK/daemon.log" >&2; exit 1
}

qa_new() { # "opening message" — opens a conversational session; echoes sid
  local sid; sid="$("$AR" new --workspace "$WS" "$WORK/base.yaml" "$1" 2>>"$WORK/err.log")"
  [ -n "$sid" ] || { echo "${QA}: no session id" >&2; cat "$WORK/err.log" "$WORK/daemon.log" >&2; exit 1; }
  printf '%s' "$sid"
}

qa_sdir() { printf '%s' "$XDG_DATA_HOME/agentrunner/sessions/$1"; } # sid → journal dir

qa_wait_turns() { # sid_dir want [timeout_ticks]
  local dir="$1" want="$2" ticks="${3:-300}" i n
  for i in $(seq 1 "$ticks"); do
    n="$(count_type assistant_message "$dir/events.jsonl")"
    [ "$n" -ge "$want" ] && return 0
    sleep 0.2
  done
  echo "${QA}: timed out waiting for $want assistant messages (have ${n:-0})" >&2
  cat "$WORK/daemon.log" >&2; return 1
}

qa_wait_idle() { # sid_dir want_parks [timeout_ticks] — waits until the
  # session has parked at idle N times (one park per completed exchange);
  # message counts are unreliable, a turn may span several assistant steps.
  local dir="$1" want="$2" ticks="${3:-450}" i n
  for i in $(seq 1 "$ticks"); do
    n="$(count_type waiting_entered "$dir/events.jsonl")"
    [ "$n" -ge "$want" ] && return 0
    sleep 0.2
  done
  echo "${QA}: timed out waiting for $want idle parks (have ${n:-0})" >&2
  cat "$WORK/daemon.log" >&2; return 1
}
