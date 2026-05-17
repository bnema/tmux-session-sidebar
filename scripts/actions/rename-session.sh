#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(cd -- "${BASH_SOURCE[0]%/*}" && pwd -P)/_bootstrap.sh"

client_name=""
session_name=""
new_name=""
rename_input_error='tmux-session-sidebar: interactive input required for rename; use --new-name'

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
    --new-name)
      require_arg "$1" "${2:-}"
      new_name="$2"
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

if [ -z "$new_name" ]; then
  if [ ! -t 0 ]; then
    echo "$rename_input_error" >&2
    exit 1
  fi

  printf 'Rename session %s to: ' "$session_name" >&2
  if ! read -r new_name; then
    echo 'tmux-session-sidebar: failed to read new session name; use --new-name' >&2
    exit 1
  fi
  if [ -z "$new_name" ]; then
    echo "$rename_input_error" >&2
    exit 1
  fi
fi

if ! validation_msg="$(sidebar_validate_session_name "$new_name" 2>&1 >/dev/null)"; then
  "$TMUX_BIN" display-message "tmux-session-sidebar: invalid session name: $new_name — ${validation_msg:-unknown error}"
  exit 1
fi

if [ "$new_name" = "$session_name" ]; then
  "$TMUX_BIN" display-message "tmux-session-sidebar: session already named $session_name"
  exit 0
fi

if sidebar_session_exists "$new_name"; then
  "$TMUX_BIN" display-message "tmux-session-sidebar: session $new_name already exists"
  exit 1
fi

session_target="$(sidebar_session_target "$session_name")" || exit 1

"$TMUX_BIN" rename-session -t "$session_target" "$new_name"
"$TMUX_BIN" display-message "tmux-session-sidebar: renamed $session_name to $new_name"
