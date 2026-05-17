#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(cd -- "${BASH_SOURCE[0]%/*}" && pwd -P)/_bootstrap.sh"

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

switch_args=(--client "$client_name" --session "$session_name")
client_window_id="$($TMUX_BIN display-message -p -t "$client_name" '#{window_id}' 2>/dev/null || true)"
if [ -n "$client_window_id" ]; then
  sidebar_pane_id="$(sidebar_existing_sidebar_pane "$client_window_id" 2>/dev/null || true)"
  if [ -n "$sidebar_pane_id" ]; then
    switch_args+=(--sidebar-pane "$sidebar_pane_id")
  fi
fi

exec "$SCRIPT_DIR/switch-session.sh" "${switch_args[@]}"
