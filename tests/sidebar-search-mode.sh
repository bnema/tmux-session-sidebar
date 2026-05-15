#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
REAL_TMUX_BIN="$(command -v tmux 2>/dev/null || true)"
FZF_BIN="$(command -v fzf 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
[ -n "$REAL_TMUX_BIN" ] || { echo 'tmux not found' >&2; exit 1; }
[ -n "$FZF_BIN" ] || { echo 'fzf not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
sock="tss_test_sidebar_search_mode_$$"
client_log="$work_dir/client.log"
client_pid=""

cleanup() {
  if [ -n "$client_pid" ]; then
    kill "$client_pid" 2>/dev/null || true
  fi
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" kill-server 2>/dev/null || true
  rm -rf "$work_dir"
}
trap cleanup EXIT

pane_exists() {
  local pane_id="$1"
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -a -F '#{pane_id}' | grep -Fx "$pane_id" >/dev/null
}

client_session() {
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_session}' | head -n1
}

active_filter() {
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -p -t "$1" -vq @session-sidebar-active-filter 2>/dev/null || true
}

wait_for_sidebar_ready() {
  local pane_id="$1"
  local current_command

  for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25; do
    current_command="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" display-message -p -t "$pane_id" '#{pane_current_command}' 2>/dev/null || true)"
    case "$current_command" in
      ''|tmux) ;;
      *) return 0 ;;
    esac
    sleep 0.2
  done
  return 1
}

wait_for_active_filter() {
  local pane_id="$1"
  local expected_filter="$2"
  local actual_filter

  for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25; do
    actual_filter="$(active_filter "$pane_id")"
    if [ "$actual_filter" = "$expected_filter" ]; then
      return 0
    fi
    sleep 0.2
  done
  return 1
}

assert_sidebar_state() {
  local pane_id="$1"
  local expected_session="$2"
  local expected_filter="$3"
  local label="$4"
  local actual_session actual_filter

  pane_exists "$pane_id" || {
    echo "expected sidebar pane to stay open after $label" >&2
    exit 1
  }

  actual_session="$(client_session)"
  [ "$actual_session" = "$expected_session" ] || {
    echo "expected client session $expected_session after $label, got: ${actual_session:-<empty>}" >&2
    exit 1
  }

  actual_filter="$(active_filter "$pane_id")"
  [ "$actual_filter" = "$expected_filter" ] || {
    echo "expected active filter '${expected_filter:-<unset>}' after $label, got: ${actual_filter:-<unset>}" >&2
    exit 1
  }
}

env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s alpha 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s beta 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-heat-colors off
script -q -c "env -u TMUX TERM=xterm-256color tmux -L $sock attach-session -t alpha" "$client_log" >/dev/null 2>&1 &
client_pid=$!

client_name=""
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  client_name="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_name}' | head -n1)"
  if [ -n "$client_name" ]; then
    break
  fi
  sleep 0.2
done
[ -n "$client_name" ] || { echo 'no tmux client found' >&2; exit 1; }

alpha_window_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-windows -t '=alpha' -F '#{window_id}' | head -n1)"
sidebar_cmd="$(printf '%q ' "$REPO_DIR/scripts/sidebar.sh" --client "$client_name" --source-path "$work_dir")"
sidebar_pane_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -P -F '#{pane_id}' -t "$alpha_window_id" -hbf -l 40 "$sidebar_cmd")"

wait_for_sidebar_ready "$sidebar_pane_id" || {
  echo 'expected sidebar pane to finish starting' >&2
  exit 1
}

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" /
sleep 0.4
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" b e t a
sleep 0.4
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" Enter
wait_for_active_filter "$sidebar_pane_id" beta || {
  echo 'expected Enter on a non-empty query to persist the active filter' >&2
  exit 1
}
assert_sidebar_state "$sidebar_pane_id" alpha beta 'applying a non-empty search filter'

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" /
sleep 0.4
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" C-u
sleep 0.2
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" Enter
wait_for_active_filter "$sidebar_pane_id" '' || {
  echo 'expected empty-query Enter to clear the active filter' >&2
  exit 1
}
assert_sidebar_state "$sidebar_pane_id" alpha '' 'pressing Enter on an empty search query'

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" /
sleep 0.4
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" b e t a
sleep 0.4
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" Enter
wait_for_active_filter "$sidebar_pane_id" beta || {
  echo 'expected filter to be restored before testing Escape' >&2
  exit 1
}

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" /
sleep 0.4
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" Escape
wait_for_active_filter "$sidebar_pane_id" '' || {
  echo 'expected Escape to clear the active filter' >&2
  exit 1
}
assert_sidebar_state "$sidebar_pane_id" alpha '' 'pressing Esc in search mode'

echo 'ok: search mode Enter/Esc returns to browse mode without closing the sidebar'
