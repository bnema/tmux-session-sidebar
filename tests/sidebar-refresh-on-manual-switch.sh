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
sock="tss_test_sidebar_refresh_on_manual_switch_$$"
client_log="$work_dir/client.log"
curl_calls="$work_dir/curl-calls.txt"
fake_bin="$work_dir/bin"
client_pid=""
mkdir -p "$fake_bin"

cleanup() {
  if [ -n "$client_pid" ]; then
    kill "$client_pid" 2>/dev/null || true
  fi
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" kill-server 2>/dev/null || true
  rm -rf "$work_dir"
}
trap cleanup EXIT

cat >"$fake_bin/curl" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "$curl_calls"
printf 'ok\n'
EOF
chmod +x "$fake_bin/curl"

env -u TMUX PATH="$fake_bin:$PATH" "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s alpha 'sleep 9999'
env -u TMUX PATH="$fake_bin:$PATH" "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s beta 'sleep 9999'
env -u TMUX PATH="$fake_bin:$PATH" "$REAL_TMUX_BIN" -L "$sock" run-shell "$REPO_DIR/tmux-session-sidebar.tmux"
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
sidebar_pane_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -P -F '#{pane_id}' -t "$alpha_window_id" -hbf -l 20 'sleep 9999')"

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-pane 1
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-refresh-socket /tmp/sidebar-refresh.sock
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-client "$client_name"
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-source-path "$work_dir"
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-show-numbered-sessions off

env -u TMUX PATH="$fake_bin:$PATH" "$REAL_TMUX_BIN" -L "$sock" switch-client -c "$client_name" -t beta
sleep 1

[ -s "$curl_calls" ] || {
  echo 'expected manual tmux session switch to trigger sidebar refresh via hook' >&2
  exit 1
}

grep -Fq -- '--unix-socket /tmp/sidebar-refresh.sock http -d reload-sync(' "$curl_calls" || {
  echo 'expected hook-driven refresh to target the registered sidebar socket' >&2
  cat "$curl_calls" >&2
  exit 1
}

echo 'ok: manual tmux session switches trigger sidebar refresh hooks'
