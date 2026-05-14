#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/../lib/tmux.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/../lib/projects.sh"

client_name=""
sidebar_pane_id=""
session_name=""
source_path=""

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
    echo 'tmux-session-sidebar: interactive input required for ad-hoc session creation; use --name' >&2
    exit 1
  fi

  printf 'New ad-hoc session name: ' >&2
  if ! read -r session_name; then
    echo 'tmux-session-sidebar: failed to read ad-hoc session name; use --name' >&2
    exit 1
  fi
fi

if ! validation_msg="$(sidebar_validate_session_name "$session_name" 2>&1 >/dev/null)"; then
  tmux display-message "tmux-session-sidebar: ${validation_msg:-invalid session name: $session_name}"
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
  tmux display-message "tmux-session-sidebar: switched to existing session $session_name"
  exec "$SCRIPT_DIR/switch-session.sh" "${switch_args[@]}"
fi

tmux new-session -d -s "$session_name" -c "$source_path" || exit 1
session_target="$(sidebar_session_target "$session_name")" || exit 1
tmux set-option -t "$session_target" @session-sidebar-kind adhoc
tmux display-message "tmux-session-sidebar: created ad-hoc session $session_name"

exec "$SCRIPT_DIR/switch-session.sh" "${switch_args[@]}"
