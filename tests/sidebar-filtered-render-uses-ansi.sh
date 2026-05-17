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
sock="tss_test_sidebar_filtered_ansi_$$"
client_log="$work_dir/client.log"
client_pid=""
fake_bin="$work_dir/bin"
fzf_args_log="$work_dir/fzf.args"
mkdir -p "$fake_bin"
cat >"$fake_bin/tmux" <<EOF
#!/usr/bin/env bash
exec "$REAL_TMUX_BIN" -L "$sock" "\$@"
EOF
cat >"$fake_bin/fzf" <<'EOF'
#!/usr/bin/env bash
printf '__fzf_call__\n' >>"$SESSION_SIDEBAR_FZF_ARGS_LOG"
printf '%s\n' "$@" >>"$SESSION_SIDEBAR_FZF_ARGS_LOG"
cat
EOF
chmod +x "$fake_bin/tmux" "$fake_bin/fzf"

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

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-heat-colors on
sidebar_pane_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t '=alpha' -F '#{pane_id}' | head -n1)"

env -u TMUX \
  PATH="$fake_bin:$PATH" \
  SESSION_SIDEBAR_FZF_ARGS_LOG="$fzf_args_log" \
  SESSION_SIDEBAR_COLOR_CAPABILITY=256 \
  SESSION_SIDEBAR_FZF_LISTEN=off \
  "$REPO_DIR/scripts/sidebar.sh" \
    --client "$client_name" \
    --sidebar-pane "$sidebar_pane_id" \
    --source-path "$work_dir" \
    --active-filter alpha >/dev/null

awk '
  $0 == "__fzf_call__" {
    if (saw_filter && saw_ansi) found = 1
    saw_filter = 0
    saw_ansi = 0
    next
  }
  $0 == "--filter" { saw_filter = 1 }
  $0 == "--ansi" { saw_ansi = 1 }
  END {
    if (saw_filter && saw_ansi) found = 1
    exit found ? 0 : 1
  }
' "$fzf_args_log" || {
  echo 'expected filtered render fzf invocation to include --ansi' >&2
  printf 'fzf args:\n' >&2
  cat "$fzf_args_log" >&2
  exit 1
}

echo 'ok: filtered render invokes fzf with ANSI escape handling'
