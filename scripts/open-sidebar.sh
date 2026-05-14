#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/lib/tmux.sh"

client_name="${1:-}"
source_window_id="${2:-}"
source_path="${4:-}"

source_window_id="$(sidebar_current_window_id "$source_window_id")" || exit 1
sidebar_current_pane_id "${3:-}" >/dev/null || exit 1
source_path="$(sidebar_current_path "$source_path")" || exit 1

existing_pane="$(sidebar_existing_sidebar_pane "$source_window_id")"
if [ -n "$existing_pane" ]; then
  tmux kill-pane -t "$existing_pane"
  exit 0
fi

width="$(sidebar_get_option @session-sidebar-width "30%")"

quoted_script=""
printf -v quoted_script '%q' "$SCRIPT_DIR/sidebar.sh"

sidebar_cmd=""
printf -v sidebar_cmd '%s --client %q --source-path %q' \
  "$quoted_script" \
  "$client_name" \
  "$source_path"

new_pane_id="$(tmux split-window -P -F '#{pane_id}' -t "$source_window_id" -hbf -l "$width" -c "$source_path" "$sidebar_cmd")" || exit 1

tmux set-option -p -t "$new_pane_id" @session-sidebar-pane 1
