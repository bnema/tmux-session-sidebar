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

new_fixture() {
  local root plugin
  root="${1:-$(mktemp -d)}"
  plugin="$root/plugin"
  mkdir -p "$plugin/scripts"
  cp "$(dirname "$0")/remove-git-update-hook.sh" "$plugin/scripts/remove-git-update-hook.sh"
  chmod +x "$plugin/scripts/remove-git-update-hook.sh"
  (cd "$plugin" && git init -q && git config user.email test@example.com && git config user.name Test && touch README && git add . && git commit -qm initial)
  printf '%s\n' "$root"
}

run_remover() {
  local root="$1"
  "$root/plugin/scripts/remove-git-update-hook.sh"
}

test_removes_managed_post_merge_hook() {
  local root hook
  root="$(new_fixture)"
  hook="$root/plugin/.git/hooks/post-merge"
  cat >"$hook" <<'HOOK'
#!/usr/bin/env bash
# tmux-session-sidebar managed update hook
echo update
HOOK
  chmod +x "$hook"

  run_remover "$root"

  [ ! -e "$hook" ] || fail "managed post-merge hook should be removed"
}

test_removes_hooks_with_managed_marker_even_when_wrapped() {
  local root hook
  root="$(new_fixture)"
  hook="$root/plugin/.git/hooks/post-merge"
  cat >"$hook" <<'HOOK'
#!/usr/bin/env bash
echo before
# tmux-session-sidebar managed update hook
echo after
HOOK
  chmod +x "$hook"

  run_remover "$root"

  [ ! -e "$hook" ] || fail "post-merge hook with managed marker should be treated as managed and removed"
}

test_preserves_unrelated_custom_hooks() {
  local root hook before after
  root="$(new_fixture)"
  hook="$root/plugin/.git/hooks/post-merge"
  cat >"$hook" <<'HOOK'
#!/usr/bin/env bash
echo custom hook
HOOK
  chmod +x "$hook"
  before="$(cksum "$hook")"

  run_remover "$root"

  after="$(cksum "$hook")"
  [ "$before" = "$after" ] || fail "custom post-merge hook should be preserved"
  assert_file_contains "$hook" "custom hook" "custom hook content should remain"
}

test_missing_hook_is_ok() {
  local root
  root="$(new_fixture)"

  run_remover "$root"
}

test_removes_managed_post_merge_hook
test_removes_hooks_with_managed_marker_even_when_wrapped
test_preserves_unrelated_custom_hooks
test_missing_hook_is_ok

echo "remove git update hook tests passed"
