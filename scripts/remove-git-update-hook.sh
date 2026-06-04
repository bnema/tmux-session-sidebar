#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
GIT_BIN="$(command -v git 2>/dev/null || true)"
GREP_BIN="$(command -v grep 2>/dev/null || true)"
RM_BIN="$(command -v rm 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || exit 0
[ -n "$PWD_BIN" ] || exit 0
[ -n "$GIT_BIN" ] || exit 0
[ -n "$GREP_BIN" ] || exit 0
[ -n "$RM_BIN" ] || exit 0

PLUGIN_DIR="$(cd "$("$DIRNAME_BIN" "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 0
MARKER="tmux-session-sidebar managed update hook"

is_git_checkout() {
  "$GIT_BIN" -C "$PLUGIN_DIR" rev-parse --git-dir >/dev/null 2>&1
}

hook_path() {
  local path
  path="$("$GIT_BIN" -C "$PLUGIN_DIR" rev-parse --git-path hooks/post-merge 2>/dev/null || true)"
  [ -n "$path" ] || return 1
  case "$path" in
    /*) printf '%s\n' "$path" ;;
    *) printf '%s/%s\n' "$PLUGIN_DIR" "$path" ;;
  esac
}

remove_managed_hook() {
  local hook
  hook="$(hook_path)" || return 0
  [ -e "$hook" ] || return 0
  "$GREP_BIN" -Fq "$MARKER" "$hook" 2>/dev/null || return 0
  "$RM_BIN" -f -- "$hook" 2>/dev/null || true
}

main() {
  is_git_checkout || exit 0
  remove_managed_hook
}

main "$@"
