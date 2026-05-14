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
session_index=""

while [ $# -gt 0 ]; do
  case "$1" in
    --client)
      require_arg "$1" "${2:-}"
      client_name="$2"
      shift 2
      ;;
    --index)
      require_arg "$1" "${2:-}"
      session_index="$2"
      shift 2
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

client_name="$(sidebar_current_client "$client_name")" || exit 1
require_arg --index "$session_index"

session_name="$(sidebar_visible_session_name_at_index "$client_name" "$session_index")" || {
  "$TMUX_BIN" display-message "tmux-session-sidebar: no visible session in quick-switch slot $session_index"
  exit 1
}

exec "$SCRIPT_DIR/switch-session.sh" --client "$client_name" --session "$session_name"
