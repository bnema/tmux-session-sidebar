#!/usr/bin/env bash

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" || exit 1
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
  set_default @session-sidebar-project-roots   "$HOME/projects"
  set_default @session-sidebar-use-fzf         on
  set_default @session-sidebar-close-after-switch  on

  local sidebar_key quoted_script
  sidebar_key="$(tmux show-options -gvq @session-sidebar-key)"
  printf -v quoted_script '%q' "$SCRIPTS_DIR/open-sidebar.sh"

  tmux bind-key -T prefix "$sidebar_key" \
    run-shell "$quoted_script #{q:client_name} #{q:window_id} #{q:pane_id} #{q:pane_current_path}"
}

main "$@"
