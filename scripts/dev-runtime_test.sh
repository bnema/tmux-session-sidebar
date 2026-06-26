#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
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
  mkdir -p "$fakebin" "$root/scripts" "$root/.bin"
  cp "$(dirname "$0")/../Makefile" "$root/Makefile"

  cat >"$root/scripts/update-runtime.sh" <<'UPDATE'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  --ensure)
    mkdir -p .bin
    cat > .bin/tmux-session-sidebar <<'RUNTIME'
#!/usr/bin/env bash
printf 'runtime %s\n' "$*" >>"$TEST_LOG"
RUNTIME
    chmod +x .bin/tmux-session-sidebar
    printf '%s/.bin/tmux-session-sidebar\n' "$(pwd)"
    ;;
  --stop-only)
    printf 'update-runtime --stop-only stale-any-path=%s\n' "${TMUX_SESSION_SIDEBAR_STOP_STALE_ANY_PATH:-}" >>"$TEST_LOG"
    ;;
  *)
    printf 'unexpected update-runtime args: %s\n' "$*" >&2
    exit 2
    ;;
esac
UPDATE

  cat >"$fakebin/pkill" <<'PKILL'
#!/usr/bin/env bash
printf 'pkill %s\n' "$*" >>"$TEST_LOG"
PKILL

  cat >"$fakebin/tmux" <<'TMUX'
#!/usr/bin/env bash
printf 'tmux %s\n' "$*" >>"$TEST_LOG"
TMUX

  chmod +x "$root/scripts/update-runtime.sh" "$fakebin/pkill" "$fakebin/tmux"
  printf '%s\n' "$root"
}

run_dev_runtime() {
  local root="$1"
  TEST_LOG="$root/log" PATH="$root/fakebin:$PATH" make -C "$root" dev-runtime >/dev/null
}

assert_file_not_contains() {
  local file="$1" needle="$2" message="$3"
  if [ -e "$file" ] && grep -Fq -- "$needle" "$file"; then
    echo "--- $file ---" >&2
    cat "$file" >&2 || true
    fail "$message: unexpected '$needle'"
  fi
}

test_dev_runtime_closes_sidebar_before_force_stopping_processes() {
  local root runtime_path
  root="$(new_fixture)"
  run_dev_runtime "$root"
  runtime_path="$root/.bin/tmux-session-sidebar"

  assert_line_equals "$root/log" 1 "runtime sidebar close" "dev-runtime should close the sidebar cleanly before force-stopping runtime processes"
  assert_line_equals "$root/log" 2 "update-runtime --stop-only stale-any-path=1" "dev-runtime should delegate stale process cleanup to the central robust stopper with previous-build path cleanup enabled after clean close"
  assert_file_not_contains "$root/log" "pkill -f $runtime_path daemon" "dev-runtime should not use inline pkill cleanup"
  [ -f "$root/.bin/.dev-runtime" ] || fail "dev-runtime should create the dev runtime marker"
}

test_dev_runtime_closes_sidebar_before_force_stopping_processes

echo "dev-runtime tests passed"
