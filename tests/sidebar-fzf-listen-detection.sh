#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
REAL_TMUX_BIN="$(command -v tmux 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
[ -n "$REAL_TMUX_BIN" ] || { echo 'tmux not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

run_case() {
  local case_name="$1"
  local help_mode="$2"
  local expect_listen="$3"
  local work_dir sock fake_bin client_log args_log helper_log client_pid client_name alpha_window_id sidebar_cmd

  work_dir="$(mktemp -d)"
  sock="tss_test_sidebar_fzf_listen_${case_name}_$$"
  fake_bin="$work_dir/bin"
  client_log="$work_dir/client.log"
  args_log="$work_dir/fzf-args.txt"
  helper_log="$work_dir/helper-runs.txt"
  client_pid=""
  mkdir -p "$fake_bin"

  cleanup_case() {
    if [ -n "$client_pid" ]; then
      kill "$client_pid" 2>/dev/null || true
    fi
    env -u TMUX "$REAL_TMUX_BIN" -L "$sock" kill-server 2>/dev/null || true
    rm -rf "$work_dir"
  }
  trap cleanup_case RETURN

  cat >"$fake_bin/fzf" <<EOF
#!/usr/bin/env bash
if [ "\${1:-}" = '--help' ]; then
  if [ "$help_mode" = 'socket' ]; then
    printf '%s\n' '--listen=SOCKET_PATH'
  else
    printf '%s\n' 'fzf old help without listen socket support'
  fi
  exit 0
fi
printf '%s\n' "\$*" > "$args_log"
printf 'esc\n'
EOF
  chmod +x "$fake_bin/fzf"

  cat >"$fake_bin/curl" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "$helper_log"
exit 1
EOF
  chmod +x "$fake_bin/curl"

  env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s alpha 'sleep 9999'
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" new-session -d -s beta 'sleep 9999'
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

  sidebar_cmd="$(printf '%q ' env PATH="$fake_bin:$PATH" "$REPO_DIR/scripts/sidebar.sh" --client "$client_name" --source-path "$work_dir")"
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -d -t "$alpha_window_id" -hbf -l 40 "$sidebar_cmd" >/dev/null

  for _ in 1 2 3 4 5 6 7 8 9 10; do
    [ -s "$args_log" ] && break
    sleep 0.2
  done

  [ -s "$args_log" ] || {
    echo "expected fake fzf to capture invocation args for case $case_name" >&2
    exit 1
  }

  if [ "$expect_listen" = 'yes' ]; then
    grep -Fq -- '--listen ' "$args_log" || {
      echo "expected --listen args for case $case_name" >&2
      cat "$args_log" >&2
      exit 1
    }
  else
    if grep -Fq -- '--listen ' "$args_log"; then
      echo "did not expect --listen args for case $case_name" >&2
      cat "$args_log" >&2
      exit 1
    fi
  fi

  if grep -Fq 'execute-silent' "$args_log"; then
    echo "did not expect blocking execute-silent refresh startup for case $case_name" >&2
    cat "$args_log" >&2
    exit 1
  fi

  trap - RETURN
  cleanup_case
}

run_case unsupported none no
run_case socket socket yes

echo 'ok: fzf listen support is detected safely and refresh startup is non-blocking'
