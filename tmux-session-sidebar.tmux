#!/usr/bin/env bash

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" || exit 1
SCRIPTS_DIR="$PLUGIN_DIR/scripts"

set_default() {
  local option="$1" default_value="$2"
  if [ -z "$(tmux show-options -gvq "$option" 2>/dev/null)" ]; then
    tmux set-option -gq "$option" "$default_value"
  fi
}

binding_belongs_to_plugin() {
  local key="$1" binding
  [ -z "$key" ] && return 1
  binding="$(tmux list-keys -T prefix "$key" 2>/dev/null || true)"
  [ -n "$binding" ] && printf '%s\n' "$binding" | grep -Fq "$SCRIPTS_DIR/open-sidebar.sh"
}

unbind_plugin_binding() {
  local key="$1"
  [ -z "$key" ] && return 0
  if binding_belongs_to_plugin "$key"; then
    tmux unbind-key -T prefix "$key" 2>/dev/null || true
  fi
}

main() {
  set_default @session-sidebar-key             b
  set_default @session-sidebar-width           30%
  set_default @session-sidebar-project-roots   "$HOME/projects"
  set_default @session-sidebar-use-fzf         on
  set_default @session-sidebar-close-after-switch  on

  local sidebar_key previous_key quoted_script
  sidebar_key="$(tmux show-options -gvq @session-sidebar-key)"
  previous_key="$(tmux show-options -gvq @session-sidebar-bound-key 2>/dev/null || true)"
  printf -v quoted_script '%q' "$SCRIPTS_DIR/open-sidebar.sh"

  if [ -n "$previous_key" ] && [ "$previous_key" != "$sidebar_key" ]; then
    unbind_plugin_binding "$previous_key"
  fi

  tmux bind-key -T prefix "$sidebar_key" \
    run-shell "$quoted_script #{q:client_name} #{q:window_id} #{q:pane_id} #{q:pane_current_path}"
  tmux set-option -gq @session-sidebar-bound-key "$sidebar_key"
}

main "$@"
