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
sock="tss_test_sidebar_refresh_width_$$"
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

env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s averyverylongsessionname 'sleep 9999'
script -q -c "env -u TMUX TERM=xterm-256color tmux -L $sock attach-session -t averyverylongsessionname" "$client_log" >/dev/null 2>&1 &
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

window_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-windows -t '=averyverylongsessionname' -F '#{window_id}' | head -n1)"
sidebar_pane_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -P -F '#{pane_id}' -t "$window_id" -hbf -l 20 'sleep 9999')"

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-heat-colors off
output="$(env -u TMUX PATH="$fake_bin:$PATH" "$REPO_DIR/scripts/sidebar.sh" --client "$client_name" --sidebar-pane "$sidebar_pane_id" --render-entries)"

grep -Fqx $'averyverylongsessionname	* [1] averyve...' <<<"$output" || {
  echo 'expected render-only refresh mode to preserve the real sidebar pane width' >&2
  printf 'output:\n%s\n' "$output" >&2
  exit 1
}

echo 'ok: render-only refresh preserves sidebar pane width'
