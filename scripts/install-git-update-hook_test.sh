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
  local root plugin
  root="${1:-$(mktemp -d)}"
  plugin="$root/plugin"
  mkdir -p "$plugin/scripts"
  cp "$(dirname "$0")/install-git-update-hook.sh" "$plugin/scripts/install-git-update-hook.sh"
  chmod +x "$plugin/scripts/install-git-update-hook.sh"
  cat >"$plugin/scripts/ensure-runtime.sh" <<'ENSURE'
#!/usr/bin/env bash
printf 'ensure-runtime %s\n' "$PWD" >>"$TEST_ROOT/ensure.log"
if [ -f "$TEST_ROOT/fail-ensure" ]; then
  echo "forced failure" >&2
  exit 42
fi
ENSURE
  chmod +x "$plugin/scripts/ensure-runtime.sh"
  cat >"$plugin/scripts/restart-runtime.sh" <<'RESTART'
#!/usr/bin/env bash
printf 'restart-runtime %s\n' "$PWD" >>"$TEST_ROOT/restart.log"
RESTART
  chmod +x "$plugin/scripts/restart-runtime.sh"
  (cd "$plugin" && git init -q && git config user.email test@example.com && git config user.name Test && touch README && git add . && git commit -qm initial)
  printf '%s\n' "$root"
}

run_installer() {
  local root="$1"
  TEST_ROOT="$root" "$root/plugin/scripts/install-git-update-hook.sh"
}

test_installs_post_merge_hook_that_rebuilds_restarts_and_does_not_break_git_pull() {
  local root hook
  root="$(new_fixture)"
  hook="$root/plugin/.git/hooks/post-merge"

  run_installer "$root"

  [ -x "$hook" ] || fail "post-merge hook should be executable"
  assert_file_contains "$hook" "tmux-session-sidebar managed update hook" "hook should identify itself"

  TEST_ROOT="$root" "$hook"
  assert_file_contains "$root/ensure.log" "ensure-runtime $root/plugin" "hook should run ensure-runtime from plugin root"
  assert_file_contains "$root/restart.log" "restart-runtime $root/plugin" "hook should restart runtime from plugin root after refresh"

  rm -f "$root/ensure.log" "$root/restart.log"
  touch "$root/fail-ensure"
  TEST_ROOT="$root" "$hook"
  assert_file_contains "$root/ensure.log" "ensure-runtime $root/plugin" "hook should still call ensure-runtime when it fails"
  [ ! -e "$root/restart.log" ] || fail "hook should not restart runtime when ensure-runtime fails"
}

test_installer_handles_literal_dollars_in_plugin_path() {
  local root hook
  root="$(mktemp -d "${TMPDIR:-/tmp}/tss-\$missing.XXXXXX")"
  new_fixture "$root" >/dev/null
  hook="$root/plugin/.git/hooks/post-merge"

  run_installer "$root"
  TEST_ROOT="$root" "$hook"

  assert_file_contains "$root/ensure.log" "ensure-runtime $root/plugin" "literal dollar path should not break generated hook"
}

test_hook_reports_bin_creation_failure_and_still_invokes_ensure_runtime() {
  local root hook output
  root="$(new_fixture)"
  hook="$root/plugin/.git/hooks/post-merge"
  run_installer "$root"

  rm -rf "$root/plugin/.bin"
  printf 'not a directory\n' >"$root/plugin/.bin"

  output="$(TEST_ROOT="$root" "$hook" 2>&1 >/dev/null)"
  assert_file_contains "$root/ensure.log" "ensure-runtime $root/plugin" "hook should still invoke ensure-runtime when .bin cannot be created"
  case "$output" in
    *"cannot create log directory"*) ;;
    *) fail "hook should report .bin log directory creation failure, got: $output" ;;
  esac
}

test_installer_is_idempotent_and_preserves_custom_hooks() {
  local root hook first second custom_root custom_hook before after
  root="$(new_fixture)"
  hook="$root/plugin/.git/hooks/post-merge"

  run_installer "$root"
  first="$(cksum "$hook")"
  run_installer "$root"
  second="$(cksum "$hook")"
  assert_eq "$first" "$second" "reinstalling managed hook should be idempotent"

  custom_root="$(new_fixture)"
  custom_hook="$custom_root/plugin/.git/hooks/post-merge"
  cat >"$custom_hook" <<'CUSTOM'
#!/usr/bin/env bash
echo custom hook
CUSTOM
  chmod +x "$custom_hook"
  before="$(cksum "$custom_hook")"
  run_installer "$custom_root"
  after="$(cksum "$custom_hook")"
  assert_eq "$before" "$after" "installer should preserve unrelated custom hooks"
}

test_installs_post_merge_hook_that_rebuilds_restarts_and_does_not_break_git_pull
test_installer_handles_literal_dollars_in_plugin_path
test_hook_reports_bin_creation_failure_and_still_invokes_ensure_runtime
test_installer_is_idempotent_and_preserves_custom_hooks

echo "install-git-update-hook tests passed"
