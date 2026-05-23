#!/usr/bin/env bash
set -euo pipefail

log() {
  local log_file="$1" message="$2"
  printf 'tmux-session-sidebar: %s\n' "$message" >>"$log_file"
}

is_sidebar_daemon_pid() {
  local pid="$1" runtime_bin="$2" command
  [ -n "$pid" ] || return 1
  case "$pid" in
    *[!0-9]*) return 1 ;;
  esac
  command="$(ps -o command= -p "$pid" 2>/dev/null || true)"
  case "$command" in
    "$runtime_bin daemon serve"|"$runtime_bin daemon serve "*) return 0 ;;
    *"/tmux-session-sidebar daemon serve"|*"/tmux-session-sidebar daemon serve "*) return 0 ;;
  esac
  return 1
}

wait_for_daemon_exit() {
  local pid="$1" runtime_bin="$2" attempt wait_retries
  wait_retries="${DAEMON_WAIT_RETRIES:-30}"
  case "$wait_retries" in
    *[!0-9]*|"") wait_retries=30 ;;
  esac
  for ((attempt = 1; attempt <= wait_retries; attempt++)); do
    if ! is_sidebar_daemon_pid "$pid" "$runtime_bin"; then
      return 0
    fi
    sleep 0.1
  done
  ! is_sidebar_daemon_pid "$pid" "$runtime_bin"
}

stop_existing_daemon() {
  local runtime_bin="$1" pid_file="$2" log_file="$3" old_pid
  [ -f "$pid_file" ] || return 0
  old_pid="$(tr -d '\n' <"$pid_file" 2>/dev/null || true)"
  if ! is_sidebar_daemon_pid "$old_pid" "$runtime_bin"; then
    return 0
  fi
  if ! kill "$old_pid" 2>/dev/null && is_sidebar_daemon_pid "$old_pid" "$runtime_bin"; then
    log "$log_file" "failed to stop existing daemon pid $old_pid"
    return 1
  fi
  if ! wait_for_daemon_exit "$old_pid" "$runtime_bin"; then
    log "$log_file" "existing daemon pid $old_pid did not exit after stop request"
    return 1
  fi
  return 0
}

main() {
  local runtime_bin state_dir pid_file log_file
  if [ "$#" -ne 2 ]; then
    echo "usage: $0 <runtime-bin> <state-dir>" >&2
    exit 2
  fi

  runtime_bin="$1"
  state_dir="$2"
  pid_file="$state_dir/daemon.pid"
  log_file="$state_dir/errors.log"

  mkdir -p "$state_dir"
  touch "$log_file"

  if [ ! -x "$runtime_bin" ]; then
    log "$log_file" "runtime is not executable: $runtime_bin"
    exit 1
  fi
  stop_existing_daemon "$runtime_bin" "$pid_file" "$log_file"
  "$runtime_bin" daemon ensure >/dev/null 2>>"$log_file"
  exec "$runtime_bin" daemon serve >/dev/null 2>>"$log_file"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
