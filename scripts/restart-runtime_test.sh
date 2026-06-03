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
  cat >"$fakebin/ps" <<'PS'
#!/usr/bin/env bash
printf 'ps %s\n' "$*" >>"$TEST_LOG"
count_file="$TEST_ROOT/ps-count"
count="$(cat "$count_file" 2>/dev/null || printf 0)"
printf '%s' "$((count + 1))" >"$count_file"
if [ "${TEST_PS_MATCH_ONCE:-}" = 1 ] && [ "$count" = 0 ]; then
  printf '%s daemon serve\n' "$TEST_RUNTIME"
fi
PS
  chmod +x "$fakebin/pkill" "$fakebin/tmux" "$fakebin/ps"
  printf '%s\n' "$root"
}

run_restart() {
  local root="$1" state_home
  state_home="${XDG_STATE_HOME:-$root/statehome}"
  mkdir -p "$state_home"
  TEST_ROOT="$root" TEST_LOG="$root/log" TEST_RUNTIME="$(cd "$(dirname "$0")/.." && pwd)/.bin/tmux-session-sidebar" XDG_STATE_HOME="$state_home" PATH="$root/fakebin:$PATH" "$(dirname "$0")/restart-runtime.sh"
}

test_restarts_tmux_sidebar_runtime_processes_and_hidden_session() {
  local root runtime_pattern runtime_path
  root="$(new_fixture)"
  run_restart "$root"
  runtime_path="$(cd "$(dirname "$0")/.." && pwd)/.bin/tmux-session-sidebar"
  runtime_pattern="${runtime_path//./\\.}"
  assert_line_equals "$root/log" 1 "pkill -f $runtime_pattern daemon serve-ui" "restart should stop existing sidebar UI first with escaped plugin-local runtime scope"
  assert_line_equals "$root/log" 2 "pkill -f $runtime_pattern daemon serve" "restart should stop existing daemon next with escaped plugin-local runtime scope"
  assert_line_equals "$root/log" 3 "tmux kill-session -t __tmux-session-sidebar" "restart should remove hidden sidebar session"
  assert_line_equals "$root/log" 4 "tmux source-file $HOME/.tmux.conf" "restart should reload tmux config last"
}

test_stops_pidfile_daemon_before_fallback_and_reload() {
  local root runtime_pattern runtime_path state_dir
  root="$(new_fixture)"
  runtime_path="$(cd "$(dirname "$0")/.." && pwd)/.bin/tmux-session-sidebar"
  runtime_pattern="${runtime_path//./\\.}"
  state_dir="$root/statehome/tmux-session-sidebar"
  mkdir -p "$state_dir"
  printf '123\n' >"$state_dir/daemon.pid"

  TEST_PS_MATCH_ONCE=1 XDG_STATE_HOME="$root/statehome" run_restart "$root"

  assert_line_equals "$root/log" 1 "ps -o command= -p 123" "restart should inspect daemon pidfile before fallback kills"
  assert_file_contains "$root/log" "pkill -f $runtime_pattern daemon serve" "restart should still run scoped daemon fallback after pidfile handling"
  assert_file_contains "$root/log" "tmux source-file $HOME/.tmux.conf" "restart should reload tmux after pidfile handling"
}

test_restarts_tmux_sidebar_runtime_processes_and_hidden_session
test_stops_pidfile_daemon_before_fallback_and_reload

echo "restart-runtime tests passed"
