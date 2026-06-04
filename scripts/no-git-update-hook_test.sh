#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

root="$(cd "$(dirname "$0")/.." && pwd)"

install_hook_script="install-git-update-hook"
if grep -R --exclude "$(basename "$0")" "$install_hook_script" \
  "$root/tmux-session-sidebar.tmux" "$root/Makefile" "$root/scripts" 2>/dev/null; then
  fail "automatic git update hook installer references should be removed"
fi

if [ -e "$root/scripts/${install_hook_script}.sh" ] || [ -e "$root/scripts/${install_hook_script}_test.sh" ]; then
  fail "automatic git update hook installer scripts should be removed"
fi

echo "no git update hook tests passed"
