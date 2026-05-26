#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_file_contains() {
  local file="$1" needle="$2" message="$3"
  if ! grep -Fq -- "$needle" "$file"; then
    echo "--- $file ---" >&2
    cat "$file" >&2 || true
    fail "$message: missing '$needle'"
  fi
}

assert_line_equals() {
  local file="$1" line_number="$2" want="$3" message="$4" line
  line="$(sed -n "${line_number}p" "$file")"
  if [ "$line" != "$want" ]; then
    echo "--- $file ---" >&2
    cat "$file" >&2 || true
    fail "$message: expected line $line_number '$want', got '$line'"
  fi
}

new_fixture() {
  local root fakebin
  root="$(mktemp -d)"
  fakebin="$root/fakebin"
  mkdir -p "$fakebin"
  cat >"$fakebin/pkill" <<'PKILL'
#!/usr/bin/env bash
printf 'pkill %s\n' "$*" >>"$TEST_LOG"
PKILL
  cat >"$fakebin/tmux" <<'TMUX'
#!/usr/bin/env bash
printf 'tmux %s\n' "$*" >>"$TEST_LOG"
TMUX
  chmod +x "$fakebin/pkill" "$fakebin/tmux"
  printf '%s\n' "$root"
}

run_restart() {
  local root="$1"
  TEST_LOG="$root/log" PATH="$root/fakebin:$PATH" "$(dirname "$0")/restart-runtime.sh"
}

test_restarts_tmux_sidebar_runtime_processes_and_hidden_session() {
  local root
  root="$(new_fixture)"
  run_restart "$root"
  assert_line_equals "$root/log" 1 "pkill -f tmux-session-sidebar daemon serve-ui" "restart should stop existing sidebar UI first"
  assert_line_equals "$root/log" 2 "pkill -f tmux-session-sidebar daemon serve" "restart should stop existing daemon next"
  assert_line_equals "$root/log" 3 "tmux kill-session -t __tmux-session-sidebar" "restart should remove hidden sidebar session"
  assert_line_equals "$root/log" 4 "tmux source-file $HOME/.tmux.conf" "restart should reload tmux config last"
}

test_restarts_tmux_sidebar_runtime_processes_and_hidden_session

echo "restart-runtime tests passed"
