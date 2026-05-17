#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
REAL_TMUX_BIN="$(command -v tmux 2>/dev/null || true)"
REAL_SCRIPT_BIN="$(command -v script 2>/dev/null || true)"
FZF_BIN="$(command -v fzf 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
[ -n "$REAL_TMUX_BIN" ] || { echo 'tmux not found' >&2; exit 1; }
[ -n "$REAL_SCRIPT_BIN" ] || { echo 'script not found' >&2; exit 1; }
[ -n "$FZF_BIN" ] || { echo 'fzf not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
sock="tss_test_sidebar_current_marker_refresh_$$"
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

pane_contains() {
  local pane_id="$1"
  local needle="$2"

  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" capture-pane -p -t "$pane_id" | grep -Fq -- "$needle"
}

env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s alpha 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s beta 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-heat-colors off
"$REAL_SCRIPT_BIN" -q -c "env -u TMUX TERM=xterm-256color tmux -L $sock attach-session -t alpha" "$client_log" >/dev/null 2>&1 &
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
  echo 'expected sidebar pane to finish starting before switching sessions' >&2
  exit 1
}
sleep 0.5

pane_contains "$sidebar_pane_id" '* [1] alpha' || {
  echo 'expected the sidebar to initially mark alpha as current' >&2
  printf 'sidebar pane content:\n' >&2
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" capture-pane -p -t "$sidebar_pane_id" >&2
  exit 1
}

env -u TMUX PATH="$fake_bin:$PATH" "$REPO_DIR/scripts/actions/switch-session.sh" --client "$client_name" --session beta

client_session=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  client_session="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_session}' | head -n1)"
  if [ "$client_session" = 'beta' ]; then
    break
  fi
  sleep 0.2
done

[ "$client_session" = 'beta' ] || {
  echo "expected switch-session.sh to switch the client to beta, got: ${client_session:-<empty>}" >&2
  exit 1
}

for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  if pane_contains "$sidebar_pane_id" '* [2] beta'; then
    break
  fi
  sleep 0.2
done

pane_contains "$sidebar_pane_id" '* [2] beta' || {
  echo 'expected the sidebar current-session marker to refresh to beta after switching with heat colors off' >&2
  printf 'sidebar pane content:\n' >&2
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" capture-pane -p -t "$sidebar_pane_id" >&2
  exit 1
}

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" send-keys -t "$sidebar_pane_id" Enter
sleep 0.5
client_session="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_session}' | head -n1)"

[ "$client_session" = 'beta' ] || {
  echo "expected Enter after the refresh to keep targeting beta, got: ${client_session:-<empty>}" >&2
  printf 'sidebar pane content:\n' >&2
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" capture-pane -p -t "$sidebar_pane_id" >&2
  exit 1
}

echo 'ok: sidebar refreshes the current-session marker and keeps the current session selected after switching with heat colors off'
