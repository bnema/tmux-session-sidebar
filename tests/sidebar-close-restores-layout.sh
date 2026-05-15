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
sock="tss_test_sidebar_close_restores_layout_$$"
client_log="$work_dir/client.log"
client_pid=""
fake_bin="$work_dir/bin"
mkdir -p "$fake_bin"
cat >"$fake_bin/tmux" <<EOF
#!/usr/bin/env bash
exec "$REAL_TMUX_BIN" -L "$sock" "\$@"
EOF
chmod +x "$fake_bin/tmux"

cleanup() {
  if [ -n "$client_pid" ]; then
    kill "$client_pid" 2>/dev/null || true
  fi
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" kill-server 2>/dev/null || true
  rm -rf "$work_dir"
}
trap cleanup EXIT

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

env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s alpha 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-use-fzf off
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

window_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-windows -t '=alpha' -F '#{window_id}' | head -n1)"
base_pane_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$window_id" -F '#{pane_id}' | head -n1)"
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -P -F '#{pane_id}' -t "$base_pane_id" -h 'sleep 9999' >/dev/null

expected_layout="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" display-message -p -t "$window_id" '#{window_layout}')"

env -u TMUX PATH="$fake_bin:$PATH" "$REPO_DIR/scripts/open-sidebar.sh" "$client_name" "$window_id" "$base_pane_id" "$work_dir"
sidebar_pane_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$window_id" -F '#{pane_id}	#{@session-sidebar-pane}' | awk -F $'\t' '$2 == 1 { print $1; exit }')"
[ -n "$sidebar_pane_id" ] || {
  echo 'expected open-sidebar.sh to create a marked sidebar pane' >&2
  exit 1
}

wait_for_sidebar_ready "$sidebar_pane_id" || {
  echo 'expected sidebar pane to finish starting before closing it' >&2
  exit 1
}
sleep 0.5
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" q Enter

for _ in 1 2 3 4 5 6 7 8 9 10; do
  if ! env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$window_id" -F '#{pane_id}' | grep -Fxq "$sidebar_pane_id"; then
    break
  fi
  sleep 0.2
done

if env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$window_id" -F '#{pane_id}' | grep -Fxq "$sidebar_pane_id"; then
  echo 'expected sidebar pane to close after pressing q in fallback mode' >&2
  exit 1
fi

actual_layout=''
for _ in 1 2 3 4 5 6 7 8 9 10 \
         11 12 13 14 15 16 17 18 19 20; do
  actual_layout="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" display-message -p -t "$window_id" '#{window_layout}')"
  if [ "$actual_layout" = "$expected_layout" ]; then
    break
  fi
  sleep 0.1
done

[ "$actual_layout" = "$expected_layout" ] || {
  echo 'expected closing the sidebar to restore the original pane layout' >&2
  printf 'expected: %s\n' "$expected_layout" >&2
  printf 'actual:   %s\n' "$actual_layout" >&2
  exit 1
}

echo 'ok: closing the sidebar restores the original pane layout'
