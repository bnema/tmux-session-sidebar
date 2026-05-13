#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/../lib/tmux.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/../lib/projects.sh"

client_name=""
sidebar_pane_id=""
project_path=""
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
    --project-path)
      require_arg "$1" "${2:-}"
      project_path="$2"
      shift 2
      ;;
    --session-name)
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

if [ -z "$project_path" ]; then
  project_path="$(sidebar_pick_project)" || exit 1
fi

if [ ! -d "$project_path" ]; then
  tmux display-message "tmux-session-sidebar: project path not found: $project_path"
  exit 1
fi

if [ -z "$session_name" ]; then
  session_name="$(sidebar_derive_session_name "$project_path")"
fi

if ! sidebar_validate_session_name "$session_name" >/dev/null 2>&1; then
  tmux display-message "tmux-session-sidebar: invalid session name: $session_name"
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

tmux new-session -d -s "$session_name" -c "$project_path" || exit 1
session_target="$(sidebar_session_target "$session_name")" || exit 1
tmux set-option -t "$session_target" @session-sidebar-kind project
tmux set-option -t "$session_target" @session-sidebar-project-path "$project_path"
tmux display-message "tmux-session-sidebar: created project session $session_name"

exec "$SCRIPT_DIR/switch-session.sh" "${switch_args[@]}"
