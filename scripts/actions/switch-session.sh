#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/../lib/tmux.sh"

client_name=""
session_name=""
sidebar_pane_id=""

while [ $# -gt 0 ]; do
  case "$1" in
    --client)
      client_name="${2:-}"
      shift 2
      ;;
    --session)
      session_name="${2:-}"
      shift 2
      ;;
    --sidebar-pane)
      sidebar_pane_id="${2:-}"
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

if ! sidebar_session_exists "$session_name"; then
  tmux display-message "tmux-session-sidebar: session '$session_name' does not exist"
  exit 1
fi

target_session="$(sidebar_session_target "$session_name")" || exit 1
close_after_switch="$(sidebar_get_option @session-sidebar-close-after-switch on)"

if [ -n "${TMUX:-}" ]; then
  tmux switch-client -t "$target_session"
else
  tmux switch-client -c "$client_name" -t "$target_session"
fi

if [ "$close_after_switch" = "on" ] && [ -n "$sidebar_pane_id" ]; then
  tmux kill-pane -t "$sidebar_pane_id" 2>/dev/null || true
fi
