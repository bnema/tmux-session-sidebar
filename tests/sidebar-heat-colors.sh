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
sock="tss_test_sidebar_heat_colors_$$"
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

env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s alpha 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s beta 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s gamma 'sleep 9999'
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

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-heat-half-life-hours 8
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-heat-stale-hours 24
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-heat-score 0
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-heat-updated-at 1000000000
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-heat-attached-count 1
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-last-seen-at 1000000000
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t beta @session-sidebar-heat-score 7200
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t beta @session-sidebar-heat-updated-at 1000000000
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t beta @session-sidebar-heat-attached-count 0
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t beta @session-sidebar-last-seen-at 1000000000
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t gamma @session-sidebar-heat-score 7200
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t gamma @session-sidebar-heat-updated-at 1000000000
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t gamma @session-sidebar-heat-attached-count 0
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t gamma @session-sidebar-last-seen-at 999900000

output="$(env -u TMUX PATH="$fake_bin:$PATH" SESSION_SIDEBAR_NOW=1000003600 SESSION_SIDEBAR_COLOR_CAPABILITY=256 "$REPO_DIR/scripts/sidebar.sh" --client "$client_name" --source-path "$work_dir" --render-entries)"

printf '%s\n' "$output" | grep -Fqx $'alpha\t\033[1;38;5;121m* [1] alpha\033[0m' || {
  echo 'expected current alpha session to render in the hottest current-session color' >&2
  printf 'output:\n%s\n' "$output" >&2
  exit 1
}

printf '%s\n' "$output" | grep -Fqx $'beta\t\033[38;5;108m  [2] beta\033[0m' || {
  echo 'expected recent beta session to render in a warm green color' >&2
  printf 'output:\n%s\n' "$output" >&2
  exit 1
}

printf '%s\n' "$output" | grep -Fqx $'gamma\t\033[2;38;5;244m  [3] gamma\033[0m' || {
  echo 'expected stale gamma session to render in faded gray' >&2
  printf 'output:\n%s\n' "$output" >&2
  exit 1
}

echo 'ok: sidebar renders heat-based recency colors'
