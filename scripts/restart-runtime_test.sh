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

line_number_for() {
  local file="$1" needle="$2"
  { grep -nF -- "$needle" "$file" || true; } | head -n 1 | cut -d: -f1
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
if [ "$1" = display-message ] && [ "${*: -1}" = '#{config_files}' ]; then
  printf '%s\n' "${TEST_TMUX_CONFIG_FILES:-}"
fi
TMUX
  cat >"$fakebin/ps" <<'PS'
#!/usr/bin/env bash
printf 'ps %s\n' "$*" >>"$TEST_LOG"
case "$*" in
  "-o command= -p 456")
    count_file="$TEST_ROOT/ps-456-count"
    count="$(cat "$count_file" 2>/dev/null || printf 0)"
    printf '%s' "$((count + 1))" >"$count_file"
    [ "$count" = 0 ] && printf '/tmp/old-build/tmux-session-sidebar daemon serve-ui\n'
    exit 0
    ;;
  "-o command= -p 457")
    count_file="$TEST_ROOT/ps-457-count"
    count="$(cat "$count_file" 2>/dev/null || printf 0)"
    printf '%s' "$((count + 1))" >"$count_file"
    [ "$count" = 0 ] && printf '/tmp/old-build/tmux-session-sidebar daemon bootstrap\n'
    exit 0
    ;;
  "-o command= -p 458")
    count_file="$TEST_ROOT/ps-458-count"
    count="$(cat "$count_file" 2>/dev/null || printf 0)"
    printf '%s' "$((count + 1))" >"$count_file"
    [ "$count" = 0 ] && printf '/tmp/old-build/tmux-session-sidebar daemon serve\n'
    exit 0
    ;;
  "-eo pid=,command=")
    if [ "${TEST_PS_SCAN_OLD_RUNTIME:-}" = 1 ]; then
      printf '456 /tmp/old-build/tmux-session-sidebar daemon serve-ui\n'
      printf '457 /tmp/old-build/tmux-session-sidebar daemon bootstrap\n'
      printf '458 /tmp/old-build/tmux-session-sidebar daemon serve\n'
    fi
    exit 0
    ;;
esac
count_file="$TEST_ROOT/ps-count"
count="$(cat "$count_file" 2>/dev/null || printf 0)"
printf '%s' "$((count + 1))" >"$count_file"
if [ "${TEST_PS_MATCH_ONCE:-}" = 1 ] && [ "$count" = 0 ]; then
  printf '%s daemon serve\n' "$TEST_RUNTIME"
fi
if [ "${TEST_PS_OLD_RUNTIME_MATCH_ONCE:-}" = 1 ] && [ "$count" = 0 ]; then
  printf '/tmp/old-build/tmux-session-sidebar daemon serve\n'
fi
PS
  chmod +x "$fakebin/pkill" "$fakebin/tmux" "$fakebin/ps"
  printf '%s\n' "$root"
}

run_restart() {
  local root="$1" state_home
  state_home="${XDG_STATE_HOME:-$root/statehome}"
  mkdir -p "$state_home"
  TEST_ROOT="$root" TEST_LOG="$root/log" TEST_RUNTIME="$(cd "$(dirname "$0")/.." && pwd)/.bin/tmux-session-sidebar" XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$root/config}" XDG_STATE_HOME="$state_home" PATH="$root/fakebin:$PATH" "$(dirname "$0")/restart-runtime.sh"
}

test_restarts_tmux_sidebar_runtime_processes_and_hidden_session() {
  local root
  root="$(new_fixture)"
  run_restart "$root"
  assert_line_equals "$root/log" 1 "ps -eo pid=,command=" "restart should scan sidebar UI processes first"
  assert_line_equals "$root/log" 2 "ps -eo pid=,command=" "restart should scan plugin bootstrap daemon processes next"
  assert_line_equals "$root/log" 3 "ps -eo pid=,command=" "restart should scan daemon serve processes next"
  assert_line_equals "$root/log" 4 "tmux kill-session -t __tmux-session-sidebar" "restart should remove hidden sidebar session"
  assert_line_equals "$root/log" 5 "tmux display-message -p #{config_files}" "restart should inspect active tmux config files before fallback"
  assert_line_equals "$root/log" 6 "tmux source-file $HOME/.tmux.conf" "restart should reload fallback tmux config last"
}

test_sources_active_tmux_config_files() {
  local root first_conf second_conf
  root="$(new_fixture)"
  first_conf="$root/first.conf"
  second_conf="$root/second.conf"
  : >"$first_conf"
  : >"$second_conf"
  TEST_TMUX_CONFIG_FILES="$first_conf,$second_conf" run_restart "$root"
  assert_line_equals "$root/log" 1 "ps -eo pid=,command=" "restart should scan sidebar UI before sourcing configs"
  assert_line_equals "$root/log" 2 "ps -eo pid=,command=" "restart should scan bootstrap daemon before sourcing configs"
  assert_line_equals "$root/log" 3 "ps -eo pid=,command=" "restart should scan serve daemon before sourcing configs"
  assert_line_equals "$root/log" 4 "tmux kill-session -t __tmux-session-sidebar" "restart should remove hidden sidebar session before sourcing configs"
  assert_line_equals "$root/log" 5 "tmux display-message -p #{config_files}" "restart should query active tmux config files"
  assert_line_equals "$root/log" 6 "tmux source-file $first_conf" "restart should reload first active tmux config"
  assert_line_equals "$root/log" 7 "tmux source-file $second_conf" "restart should reload second active tmux config"
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
  local scan_line tmux_line
  scan_line="$(line_number_for "$root/log" "ps -eo pid=,command=")"
  tmux_line="$(line_number_for "$root/log" "tmux source-file $HOME/.tmux.conf")"
  [ -n "$scan_line" ] || fail "restart should still scan for runtime processes after pidfile handling"
  [ -n "$tmux_line" ] || fail "restart should reload tmux after pidfile handling"
  [ "$scan_line" -lt "$tmux_line" ] || fail "restart should scan runtime processes before tmux reload"
}

test_stops_stale_pidfile_daemon_from_previous_runtime_path() {
  local inspect_count root state_dir
  root="$(new_fixture)"
  state_dir="$root/statehome/tmux-session-sidebar"
  mkdir -p "$state_dir"
  printf '123\n' >"$state_dir/daemon.pid"

  TEST_PS_OLD_RUNTIME_MATCH_ONCE=1 XDG_STATE_HOME="$root/statehome" run_restart "$root"

  inspect_count="$(grep -cF 'ps -o command= -p 123' "$root/log" || true)"
  [ "$inspect_count" = 2 ] || fail "restart should treat previous-build tmux-session-sidebar daemon pid as stoppable and wait for exit; inspected $inspect_count times"
}

test_stops_scanned_previous_runtime_paths_when_enabled() {
  local root serve_count ui_count bootstrap_count
  root="$(new_fixture)"

  TMUX_SESSION_SIDEBAR_STOP_STALE_ANY_PATH=1 TEST_PS_SCAN_OLD_RUNTIME=1 run_restart "$root"

  ui_count="$(grep -cF 'ps -o command= -p 456' "$root/log" || true)"
  bootstrap_count="$(grep -cF 'ps -o command= -p 457' "$root/log" || true)"
  serve_count="$(grep -cF 'ps -o command= -p 458' "$root/log" || true)"
  [ "$ui_count" = 2 ] || fail "restart should stop and wait for scanned previous-build serve-ui pid; inspected $ui_count times"
  [ "$bootstrap_count" = 2 ] || fail "restart should stop and wait for scanned previous-build bootstrap pid; inspected $bootstrap_count times"
  [ "$serve_count" = 2 ] || fail "restart should stop and wait for scanned previous-build serve pid; inspected $serve_count times"
}

test_restarts_tmux_sidebar_runtime_processes_and_hidden_session
test_sources_active_tmux_config_files
test_stops_pidfile_daemon_before_fallback_and_reload
test_stops_stale_pidfile_daemon_from_previous_runtime_path
test_stops_scanned_previous_runtime_paths_when_enabled

echo "restart-runtime tests passed"
