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
sock="tss_test_close_after_switch_$$"
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
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30; do
  client_name="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_name}' | head -n1)"
  if [ -n "$client_name" ]; then
    break
  fi
  sleep 0.2
done
[ -n "$client_name" ] || { echo 'no tmux client found' >&2; exit 1; }

alpha_window_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-windows -t '=alpha' -F '#{window_id}' | head -n1)"
beta_window_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-windows -t '=beta' -F '#{window_id}' | head -n1)"
alpha_sidebar_pane="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -P -F '#{pane_id}' -t "$alpha_window_id" -hbf -l 20 'sleep 9999')"

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" run-shell "$(printf '%q ' "$REPO_DIR/scripts/actions/switch-session.sh" --client "$client_name" --session beta --sidebar-pane "$alpha_sidebar_pane")"
sleep 1

client_session="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_session}' | head -n1)"
[ "$client_session" = 'beta' ] || {
  echo "expected client to switch to beta, got: ${client_session:-<empty>}" >&2
  exit 1
}

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$alpha_window_id" -F '#{pane_id}' | grep -Fxq "$alpha_sidebar_pane" || {
  echo 'expected sidebar pane to stay open by default after session switch' >&2
  exit 1
}

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-close-after-switch on
beta_sidebar_pane="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -P -F '#{pane_id}' -t "$beta_window_id" -hbf -l 20 'sleep 9999')"

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" run-shell "$(printf '%q ' "$REPO_DIR/scripts/actions/switch-session.sh" --client "$client_name" --session alpha --sidebar-pane "$beta_sidebar_pane")"
sleep 1

client_session="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_session}' | head -n1)"
[ "$client_session" = 'alpha' ] || {
  echo "expected client to switch back to alpha, got: ${client_session:-<empty>}" >&2
  exit 1
}

if env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$beta_window_id" -F '#{pane_id}' | grep -Fxq "$beta_sidebar_pane"; then
  echo 'expected sidebar pane to close when @session-sidebar-close-after-switch is on' >&2
  exit 1
fi

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -gu @session-sidebar-close-after-switch
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" run-shell "$REPO_DIR/tmux-session-sidebar.tmux"
sleep 1

default_close_after_switch="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -gvq @session-sidebar-close-after-switch)"
[ "$default_close_after_switch" = 'off' ] || {
  echo "expected plugin default @session-sidebar-close-after-switch=off, got: ${default_close_after_switch:-<empty>}" >&2
  exit 1
}

echo 'ok: close-after-switch stays open by default, closes when enabled, and defaults to off'
