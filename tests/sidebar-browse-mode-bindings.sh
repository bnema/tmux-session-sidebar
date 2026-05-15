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
sock="tss_test_sidebar_browse_mode_bindings_$$"
client_log="$work_dir/client.log"
args_log="$work_dir/fzf-args.txt"
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

cat >"$fake_bin/fzf" <<EOF
#!/usr/bin/env bash
if [ "\${1:-}" = '--help' ]; then
  printf '%s\n' 'fzf help without listen'
  exit 0
fi
printf '%s\n' "\$*" > "$args_log"
printf 'esc\n'
EOF
chmod +x "$fake_bin/fzf"

env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s alpha 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s beta 'sleep 9999'
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
sidebar_cmd="$(printf '%q ' env PATH="$fake_bin:$PATH" SESSION_SIDEBAR_FZF_LISTEN=off "$REPO_DIR/scripts/sidebar.sh" --client "$client_name" --source-path "$work_dir")"
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -d -t "$alpha_window_id" -hbf -l 40 "$sidebar_cmd" >/dev/null

for _ in 1 2 3 4 5 6 7 8 9 10; do
  [ -s "$args_log" ] && break
  sleep 0.2
done

[ -s "$args_log" ] || {
  echo 'expected fake fzf to capture invocation args' >&2
  exit 1
}

grep -Fq -- '--disabled' "$args_log" || {
  echo 'expected sidebar fzf to start with search disabled' >&2
  cat "$args_log" >&2
  exit 1
}

grep -Fq -- '--no-input' "$args_log" || {
  echo 'expected sidebar fzf to hide the input until search is activated' >&2
  cat "$args_log" >&2
  exit 1
}

grep -Fq -- '--bind j:down' "$args_log" || {
  echo 'expected j to move the selection down in browse mode' >&2
  cat "$args_log" >&2
  exit 1
}

grep -Fq -- '--bind k:up' "$args_log" || {
  echo 'expected k to move the selection up in browse mode' >&2
  cat "$args_log" >&2
  exit 1
}

grep -Fq -- '--expect=alt-n,alt-g,alt-a,alt-r,alt-x,alt-h,/' "$args_log" || {
  echo 'expected / to exit browse mode and enter search mode' >&2
  cat "$args_log" >&2
  exit 1
}

echo 'ok: sidebar fzf starts in browse mode with slash-to-search bindings'
