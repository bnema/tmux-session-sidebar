#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
REAL_TMUX_BIN="$(command -v tmux 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
[ -n "$REAL_TMUX_BIN" ] || { echo 'tmux not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
sock="tss_test_session_heat_tracking_$$"
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

env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s alpha 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s beta 'sleep 9999'
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

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-heat-score 0
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-heat-updated-at 1000
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-heat-attached-count 1
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t beta @session-sidebar-heat-score 0
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t beta @session-sidebar-heat-updated-at 1000
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t beta @session-sidebar-heat-attached-count 0

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" run-shell "$(printf '%q ' env SESSION_SIDEBAR_NOW=1600 "$REPO_DIR/scripts/actions/switch-session.sh" --client "$client_name" --session beta)"
sleep 1

client_session="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_session}' | head -n1)"
[ "$client_session" = 'beta' ] || {
  echo "expected client to switch to beta, got: ${client_session:-<empty>}" >&2
  exit 1
}

alpha_heat="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -t alpha -vq @session-sidebar-heat-score)"
awk -v value="$alpha_heat" 'BEGIN { exit !(value >= 599 && value <= 601) }' || {
  echo "expected alpha heat score near 600 seconds, got: ${alpha_heat:-<empty>}" >&2
  exit 1
}

alpha_updated="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -t alpha -vq @session-sidebar-heat-updated-at)"
[ "$alpha_updated" = '1600' ] || {
  echo "expected alpha heat update time 1600, got: ${alpha_updated:-<empty>}" >&2
  exit 1
}

alpha_attached="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -t alpha -vq @session-sidebar-heat-attached-count)"
[ "$alpha_attached" = '0' ] || {
  echo "expected alpha attached count to drop to 0, got: ${alpha_attached:-<empty>}" >&2
  exit 1
}

alpha_last_seen="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -t alpha -vq @session-sidebar-last-seen-at)"
[ "$alpha_last_seen" = '1600' ] || {
  echo "expected alpha last seen time 1600, got: ${alpha_last_seen:-<empty>}" >&2
  exit 1
}

beta_attached="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -t beta -vq @session-sidebar-heat-attached-count)"
[ "$beta_attached" = '1' ] || {
  echo "expected beta attached count to become 1, got: ${beta_attached:-<empty>}" >&2
  exit 1
}

beta_updated="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -t beta -vq @session-sidebar-heat-updated-at)"
[ "$beta_updated" = '1600' ] || {
  echo "expected beta heat update time 1600, got: ${beta_updated:-<empty>}" >&2
  exit 1
}

echo 'ok: switching sessions updates recent dwell heat state'
