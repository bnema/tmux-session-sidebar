#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
REAL_TMUX_BIN="$(command -v tmux 2>/dev/null || true)"
REAL_SCRIPT_BIN="$(command -v script 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
[ -n "$REAL_TMUX_BIN" ] || { echo 'tmux not found' >&2; exit 1; }
[ -n "$REAL_SCRIPT_BIN" ] || { echo 'script not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
sock="tss_test_sidebar_heat_disabled_$$"
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

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-heat-colors off
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-heat-score 0
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-heat-updated-at 1000
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -t alpha @session-sidebar-heat-attached-count 1

output="$(
  env -u TMUX \
    PATH="$fake_bin:$PATH" \
    SESSION_SIDEBAR_NOW=1600 \
    SESSION_SIDEBAR_COLOR_CAPABILITY=256 \
    "$REPO_DIR/scripts/sidebar.sh" \
      --client "$client_name" \
      --source-path "$work_dir" \
      --render-entries
)"

printf '%s\n' "$output" | grep -Fqx $'alpha\t* [1] alpha' || {
  echo 'expected heat-disabled render to produce plain current-session label' >&2
  printf 'output:\n%s\n' "$output" >&2
  exit 1
}

updated_at="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -t alpha -vq @session-sidebar-heat-updated-at)"
[ "$updated_at" = '1000' ] || {
  echo "expected heat-disabled render not to sync heat state, got updated-at: ${updated_at:-<empty>}" >&2
  exit 1
}

echo 'ok: heat-disabled sidebar render skips heat sync and color work'
