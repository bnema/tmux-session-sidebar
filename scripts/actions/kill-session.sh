#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(cd -- "${BASH_SOURCE[0]%/*}" && pwd -P)/_bootstrap.sh"

client_name=""
session_name=""
confirm=""

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
  "$TMUX_BIN" display-message "tmux-session-sidebar: session '$session_name' does not exist"
  exit 1
fi

mapfile -t sessions < <("$TMUX_BIN" list-sessions 2>/dev/null)
session_count="${#sessions[@]}"
if [ "$session_count" -le 1 ]; then
  "$TMUX_BIN" display-message 'tmux-session-sidebar: refusing to kill the last remaining session'
  exit 1
fi

if [ -z "$confirm" ]; then
  if [ ! -t 0 ]; then
    echo 'tmux-session-sidebar: interactive input required for kill confirmation; use --confirm' >&2
    exit 1
  fi

  printf 'Kill session %s? [y/N]: ' "$session_name" >&2
  if ! read -r confirm; then
    echo 'tmux-session-sidebar: failed to read kill confirmation; use --confirm' >&2
    exit 1
  fi
fi

case "$confirm" in
  y|Y|yes|YES)
    ;;
  *)
    "$TMUX_BIN" display-message "tmux-session-sidebar: kill cancelled"
    exit 1
    ;;
esac

session_target="$(sidebar_session_target "$session_name")" || exit 1

"$TMUX_BIN" kill-session -t "$session_target"
"$TMUX_BIN" display-message "tmux-session-sidebar: killed session $session_name"
