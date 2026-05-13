#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/../lib/tmux.sh"

client_name=""
session_name=""
confirm=""

require_arg() {
  local flag="$1"
  local value="${2:-}"
  if [ -z "$value" ]; then
    echo "tmux-session-sidebar: missing value for $flag" >&2
    exit 1
  fi
}

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
    --confirm)
      require_arg "$1" "${2:-}"
      confirm="$2"
      shift 2
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

client_name="$(sidebar_current_client "$client_name")" || exit 1
if [ -z "$session_name" ]; then
  session_name="$(sidebar_current_session "$client_name")" || exit 1
fi

if ! sidebar_session_exists "$session_name"; then
  tmux display-message "tmux-session-sidebar: session '$session_name' does not exist"
  exit 1
fi

session_count="$(tmux list-sessions 2>/dev/null | wc -l | tr -d ' ')"
if [ "$session_count" -le 1 ]; then
  tmux display-message 'tmux-session-sidebar: refusing to kill the last remaining session'
  exit 1
fi

if [ -z "$confirm" ]; then
  printf 'Kill session %s? [y/N]: ' "$session_name" >&2
  read -r confirm || exit 1
fi

case "$confirm" in
  y|Y|yes|YES)
    ;;
  *)
    tmux display-message "tmux-session-sidebar: kill cancelled"
    exit 1
    ;;
esac

tmux kill-session -t "$session_name" || exit 1
tmux display-message "tmux-session-sidebar: killed session $session_name"
