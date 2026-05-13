#!/usr/bin/env bash

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS_DIR="$PLUGIN_DIR/scripts"

set_default() {
  local option="$1" default_value="$2"
  if [ -z "$(tmux show-options -gvq "$option" 2>/dev/null)" ]; then
    tmux set-option -gq "$option" "$default_value"
  fi
}

main() {
  set_default @session-sidebar-key             B
  set_default @session-sidebar-width           30%
  set_default @session-sidebar-project-roots   /home/brice/projects
  set_default @session-sidebar-use-fzf         on
  set_default @session-sidebar-close-after-switch  on

  local sidebar_key
  sidebar_key="$(tmux show-options -gvq @session-sidebar-key)"

  tmux bind-key -T prefix "$sidebar_key" \
    run-shell "$SCRIPTS_DIR/open-sidebar.sh '#{client_name}' '#{window_id}' '#{pane_id}' '#{pane_current_path}'"
}

main "$@"
