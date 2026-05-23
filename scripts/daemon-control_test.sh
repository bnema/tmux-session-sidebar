#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local expected="$1" actual="$2" message="$3"
  if [ "$expected" != "$actual" ]; then
    fail "$message: expected '$expected', got '$actual'"
  fi
}

assert_file_contains() {
  local file="$1" needle="$2" message="$3"
  if ! grep -Fq -- "$needle" "$file"; then
    echo "--- $file ---" >&2
    cat "$file" >&2 || true
    fail "$message: missing '$needle'"
  fi
}

new_fixture() {
  local root runtime state_dir
  root="$(mktemp -d)"
  runtime="$root/tmux-session-sidebar"
  state_dir="$root/state"
  mkdir -p "$state_dir"
  cat >"$runtime" <<'RUNTIME'
#!/usr/bin/env bash
set -euo pipefail
state_dir="${STATE_DIR:?}"
log_file="$state_dir/runtime.log"
printf 'start %s %s\n' "$$" "$*" >>"$log_file"
case "${3-}" in
  hold)
    trap 'printf "stop %s\n" "$$" >>"$log_file"; exit 0' TERM INT
    while :; do sleep 0.1; done
    ;;
  ignore-term)
    trap '' TERM INT
    while :; do sleep 0.1; done
    ;;
esac
RUNTIME
  chmod +x "$runtime"
  printf '%s\n' "$root"
}

run_control() {
  local root="$1"
  STATE_DIR="$root/state" "$(dirname "$0")/daemon-control.sh" "$root/tmux-session-sidebar" "$root/state"
}

test_starts_runtime_when_no_existing_pid() {
  local root
  root="$(new_fixture)"
  run_control "$root"
  assert_file_contains "$root/state/runtime.log" "daemon serve" "daemon control should start the runtime"
}

test_stops_existing_matching_daemon_before_restart() {
  local root old_pid starts
  root="$(new_fixture)"
  STATE_DIR="$root/state" "$root/tmux-session-sidebar" daemon serve hold &
  old_pid=$!
  printf '%s\n' "$old_pid" >"$root/state/daemon.pid"
  sleep 0.2

  run_control "$root"

  if kill -0 "$old_pid" 2>/dev/null; then
    fail "old daemon pid $old_pid should have been stopped before restart"
  fi
  assert_file_contains "$root/state/runtime.log" "stop $old_pid" "old daemon should receive a stop signal"
  starts="$(grep -c '^start ' "$root/state/runtime.log" | tr -d ' ')"
  assert_eq "2" "$starts" "daemon control should record old and new daemon starts"
}

test_logs_and_aborts_when_existing_daemon_ignores_term() {
  local root stuck_pid
  root="$(new_fixture)"
  STATE_DIR="$root/state" "$root/tmux-session-sidebar" daemon serve ignore-term &
  stuck_pid=$!
  printf '%s\n' "$stuck_pid" >"$root/state/daemon.pid"
  sleep 0.2

  if run_control "$root"; then
    kill -KILL "$stuck_pid" 2>/dev/null || true
    fail "daemon control should fail when the existing daemon ignores TERM"
  fi

  assert_file_contains "$root/state/errors.log" "did not exit after stop request" "daemon control should log stuck-daemon failures"
  kill -KILL "$stuck_pid" 2>/dev/null || true
}

test_starts_runtime_when_no_existing_pid
test_stops_existing_matching_daemon_before_restart
test_logs_and_aborts_when_existing_daemon_ignores_term

echo "daemon-control tests passed"
