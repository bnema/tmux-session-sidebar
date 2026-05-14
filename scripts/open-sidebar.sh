#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
SCRIPT_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")" && "$PWD_BIN")" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/lib/tmux.sh"

client_name="${1:-}"
source_window_id="${2:-}"
source_pane_id="${3:-}"
source_path="${4:-}"

source_window_id="$(sidebar_current_window_id "$source_window_id")" || exit 1
source_pane_id="$(sidebar_current_pane_id "$source_pane_id")" || exit 1
if [ -n "$source_path" ]; then
  source_path="$(sidebar_current_path "$source_path")" || exit 1
else
  source_path="$("$TMUX_BIN" display-message -p -t "$source_pane_id" '#{pane_current_path}')" || exit 1
fi

existing_pane="$(sidebar_existing_sidebar_pane "$source_window_id")"
if [ -n "$existing_pane" ]; then
  "$TMUX_BIN" kill-pane -t "$existing_pane"
  exit 0
fi

width="$(sidebar_get_option @session-sidebar-width 20)"

quoted_script=""
printf -v quoted_script '%q' "$SCRIPT_DIR/sidebar.sh"

sidebar_cmd=""
printf -v sidebar_cmd '%s --client %q --source-path %q' \
  "$quoted_script" \
  "$client_name" \
  "$source_path"

new_pane_id="$("$TMUX_BIN" split-window \
  -P -F '#{pane_id}' \
  -t "$source_window_id" \
  -hbf \
  -l "$width" \
  -c "$source_path" \
  "$sidebar_cmd")" || exit 1

"$TMUX_BIN" set-option -p -t "$new_pane_id" @session-sidebar-pane 1
