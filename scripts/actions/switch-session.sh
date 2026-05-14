#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
SCRIPT_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")" && "$PWD_BIN")" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/../lib/tmux.sh"

client_name=""
session_name=""
sidebar_pane_id=""

while [ $# -gt 0 ]; do
  case "$1" in
    --client)
      require_arg "$1" "${2:-}"
      client_name="$2"
      shift 2
      ;;
    --session)
      require_arg "$1" "${2:-}"
      session_name="$2"
      shift 2
      ;;
    --sidebar-pane)
      require_arg "$1" "${2:-}"
      sidebar_pane_id="$2"
      shift 2
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

client_name="$(sidebar_current_client "$client_name")" || exit 1
[ -n "$session_name" ] || {
  echo 'tmux-session-sidebar: missing session name' >&2
  exit 1
}

if ! sidebar_validate_session_name "$session_name"; then
  exit 1
fi

if ! sidebar_session_exists "$session_name"; then
  "$TMUX_BIN" display-message "tmux-session-sidebar: session '$session_name' does not exist"
  exit 1
fi

close_after_switch="$(sidebar_get_option @session-sidebar-close-after-switch on)"
session_target="$(sidebar_session_target "$session_name")" || exit 1

"$TMUX_BIN" switch-client -c "$client_name" -t "$session_target"

if [ "$close_after_switch" = "on" ] && [ -n "$sidebar_pane_id" ]; then
  "$TMUX_BIN" kill-pane -t "$sidebar_pane_id" 2>/dev/null || true
fi
