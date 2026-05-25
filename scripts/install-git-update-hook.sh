#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
GIT_BIN="$(command -v git 2>/dev/null || true)"
MKDIR_BIN="$(command -v mkdir 2>/dev/null || true)"
CHMOD_BIN="$(command -v chmod 2>/dev/null || true)"
SED_BIN="$(command -v sed 2>/dev/null || true)"
MKTEMP_BIN="$(command -v mktemp 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || exit 0
[ -n "$PWD_BIN" ] || exit 0
[ -n "$GIT_BIN" ] || exit 0
[ -n "$MKDIR_BIN" ] || exit 0
[ -n "$CHMOD_BIN" ] || exit 0
[ -n "$SED_BIN" ] || exit 0

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

shell_quote() {
  printf "'%s'" "$(printf '%s' "$1" | "$SED_BIN" "s/'/'\\\\''/g")"
}

install_hook() {
  local hook hook_dir quoted_plugin_dir quoted_ensure_runtime tmp
  hook="$(hook_path)" || return 0
  hook_dir="$("$DIRNAME_BIN" "$hook")"

  if [ -e "$hook" ] && ! grep -Fq "$MARKER" "$hook" 2>/dev/null; then
    return 0
  fi

  "$MKDIR_BIN" -p "$hook_dir" || return 0
  quoted_plugin_dir="$(shell_quote "$PLUGIN_DIR")"
  quoted_ensure_runtime="$(shell_quote "$PLUGIN_DIR/scripts/ensure-runtime.sh")"
  if [ -n "$MKTEMP_BIN" ]; then
    tmp="$("$MKTEMP_BIN" "$hook.tmp.XXXXXX" 2>/dev/null || true)"
  else
    tmp=""
  fi
  [ -n "$tmp" ] || tmp="$hook.tmp.$$"
  cat >"$tmp" <<HOOK
#!/usr/bin/env bash
# $MARKER
set -u
PLUGIN_DIR=$quoted_plugin_dir
ENSURE_RUNTIME=$quoted_ensure_runtime
LOG_DIR="\$PLUGIN_DIR/.bin"
LOG_FILE="\$LOG_DIR/update-hook.log"
run_ensure_runtime() {
  cd "\$PLUGIN_DIR" && \
    [ -x "\$ENSURE_RUNTIME" ] && \
    TMUX_SESSION_SIDEBAR_REFRESH_RELEASE=1 "\$ENSURE_RUNTIME"
}
if mkdir -p "\$LOG_DIR" 2>/dev/null; then
  run_ensure_runtime >>"\$LOG_FILE" 2>&1 || true
else
  printf 'tmux-session-sidebar: cannot create log directory: %s\n' "\$LOG_DIR" >&2
  run_ensure_runtime || true
fi
exit 0
HOOK
  mv "$tmp" "$hook" || { rm -f "$tmp"; return 0; }
  "$CHMOD_BIN" +x "$hook" || return 0
}

main() {
  is_git_checkout || exit 0
  install_hook
}

main "$@"
