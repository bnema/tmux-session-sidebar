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
sock="tss_test_sidebar_shortcut_indicators_$$"
client_log="$work_dir/client.log"
sidebar_input_first="$work_dir/sidebar-input-first.txt"
sidebar_input_second="$work_dir/sidebar-input-second.txt"
fzf_calls="$work_dir/fzf-calls.txt"
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

cat >"$fake_bin/fzf" <<'EOF'
#!/usr/bin/env bash
call_count=0
if [ -f "$TEST_FZF_CALLS" ]; then
  call_count="$(cat "$TEST_FZF_CALLS")"
fi
call_count=$((call_count + 1))
printf '%s' "$call_count" > "$TEST_FZF_CALLS"
if [ "$call_count" -eq 1 ]; then
  cat > "$TEST_SIDEBAR_INPUT_FIRST"
  printf 'alt-h\n'
else
  cat > "$TEST_SIDEBAR_INPUT_SECOND"
  printf 'esc\n'
fi
EOF
chmod +x "$fake_bin/fzf"

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s alpha 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s beta 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s gamma 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s delta 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s epsilon 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s zeta 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s eta 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s theta 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s iota 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s kappa 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s 123 'sleep 9999'
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

sidebar_cmd="$(printf '%q ' env PATH="$fake_bin:$PATH" TEST_FZF_CALLS="$fzf_calls" TEST_SIDEBAR_INPUT_FIRST="$sidebar_input_first" TEST_SIDEBAR_INPUT_SECOND="$sidebar_input_second" "$REPO_DIR/scripts/sidebar.sh" --client "$client_name" --source-path "$work_dir")"
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -d -t "$alpha_window_id" -hbf -l 40 "$sidebar_cmd" >/dev/null

for _ in 1 2 3 4 5 6 7 8 9 10; do
  if [ -s "$sidebar_input_first" ] && [ -s "$sidebar_input_second" ]; then
    break
  fi
  sleep 0.2
done

[ -s "$sidebar_input_first" ] || {
  echo 'expected sidebar to render initial entries into fzf input' >&2
  exit 1
}

[ -s "$sidebar_input_second" ] || {
  echo 'expected sidebar to rerender entries after Alt+h' >&2
  exit 1
}

grep -Fqx $'alpha\t* [1] alpha' "$sidebar_input_first" || {
  echo 'expected alpha to show quick-switch badge [1]' >&2
  printf 'sidebar input:\n' >&2
  cat "$sidebar_input_first" >&2
  exit 1
}

grep -Fqx $'beta\t  [2] beta' "$sidebar_input_first" || {
  echo 'expected beta to show quick-switch badge [2]' >&2
  printf 'sidebar input:\n' >&2
  cat "$sidebar_input_first" >&2
  exit 1
}

grep -Fqx $'delta\t  [3] delta' "$sidebar_input_first" || {
  echo 'expected delta to show quick-switch badge [3]' >&2
  printf 'sidebar input:\n' >&2
  cat "$sidebar_input_first" >&2
  exit 1
}

grep -Fqx $'zeta\t  [0] zeta' "$sidebar_input_first" || {
  echo 'expected zeta to show quick-switch badge [0] for Ctrl+0' >&2
  printf 'sidebar input:\n' >&2
  cat "$sidebar_input_first" >&2
  exit 1
}

if grep -Fq $'123\t' "$sidebar_input_first"; then
  echo 'did not expect purely numeric session names to appear by default' >&2
  printf 'sidebar input:\n' >&2
  cat "$sidebar_input_first" >&2
  exit 1
fi

grep -Fqx $'123\t      123' "$sidebar_input_second" || {
  echo 'expected numeric session to appear without a quick-switch badge when numbers are shown' >&2
  printf 'sidebar input after Alt+h:\n' >&2
  cat "$sidebar_input_second" >&2
  exit 1
}

grep -Fqx $'alpha\t* [1] alpha' "$sidebar_input_second" || {
  echo 'expected alpha to keep quick-switch badge [1] when numeric sessions are shown' >&2
  printf 'sidebar input after Alt+h:\n' >&2
  cat "$sidebar_input_second" >&2
  exit 1
}

grep -Fqx $'beta\t  [2] beta' "$sidebar_input_second" || {
  echo 'expected beta to keep quick-switch badge [2] when numeric sessions are shown' >&2
  printf 'sidebar input after Alt+h:\n' >&2
  cat "$sidebar_input_second" >&2
  exit 1
}

grep -Fqx $'delta\t  [3] delta' "$sidebar_input_second" || {
  echo 'expected delta to keep quick-switch badge [3] when numeric sessions are shown' >&2
  printf 'sidebar input after Alt+h:\n' >&2
  cat "$sidebar_input_second" >&2
  exit 1
}

grep -Fqx $'zeta\t  [0] zeta' "$sidebar_input_second" || {
  echo 'expected zeta to keep quick-switch badge [0] when numeric sessions are shown' >&2
  printf 'sidebar input after Alt+h:\n' >&2
  cat "$sidebar_input_second" >&2
  exit 1
}

echo 'ok: sidebar shows quick-switch badges without extra status metadata'
