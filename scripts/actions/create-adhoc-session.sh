#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(cd -- "${BASH_SOURCE[0]%/*}" && pwd -P)/_bootstrap.sh"
# shellcheck source=/dev/null
source "$SIDEBAR_SCRIPT_DIR/lib/projects.sh"

client_name=""
sidebar_pane_id=""
session_name=""
source_path=""
adhoc_input_error='tmux-session-sidebar: interactive input required for ad-hoc session creation; use --name'

while [ $# -gt 0 ]; do
  case "$1" in
    --client)
      require_arg "$1" "${2:-}"
      client_name="$2"
      shift 2
      ;;
    --sidebar-pane)
      require_arg "$1" "${2:-}"
      sidebar_pane_id="$2"
      shift 2
      ;;
    --name)
      require_arg "$1" "${2:-}"
      session_name="$2"
      shift 2
      ;;
    --source-path)
      require_arg "$1" "${2:-}"
      source_path="$2"
      shift 2
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

client_name="$(sidebar_current_client "$client_name")" || exit 1
source_path="$(sidebar_current_path "$source_path")" || exit 1

if [ -z "$session_name" ]; then
  if [ ! -t 0 ]; then
    echo "$adhoc_input_error" >&2
    exit 1
  fi

  printf 'New ad-hoc session name: ' >&2
  if ! read -r session_name; then
    echo 'tmux-session-sidebar: failed to read ad-hoc session name; use --name' >&2
    exit 1
  fi
  if [ -z "$session_name" ]; then
    echo "$adhoc_input_error" >&2
    exit 1
  fi
fi

if ! validation_msg="$(sidebar_validate_session_name "$session_name" 2>&1 >/dev/null)"; then
  "$TMUX_BIN" display-message "tmux-session-sidebar: ${validation_msg:-invalid session name: $session_name}"
  exit 1
fi

switch_args=(
  --client "$client_name"
  --session "$session_name"
)
if [ -n "$sidebar_pane_id" ]; then
  switch_args+=(--sidebar-pane "$sidebar_pane_id")
fi

if sidebar_session_exists "$session_name"; then
  "$TMUX_BIN" display-message "tmux-session-sidebar: switched to existing session $session_name"
  exec "$SCRIPT_DIR/switch-session.sh" "${switch_args[@]}"
fi

"$TMUX_BIN" new-session -d -s "$session_name" -c "$source_path"
session_target="$(sidebar_session_target "$session_name")" || exit 1
"$TMUX_BIN" set-option -t "$session_target" @session-sidebar-kind adhoc
"$TMUX_BIN" display-message "tmux-session-sidebar: created ad-hoc session $session_name"

exec "$SCRIPT_DIR/switch-session.sh" "${switch_args[@]}"
