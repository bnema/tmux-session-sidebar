#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
TMUX_BIN="$(command -v tmux 2>/dev/null || true)"
GREP_BIN="$(command -v grep 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
[ -n "$TMUX_BIN" ] || { echo 'tmux-session-sidebar: tmux not found' >&2; exit 1; }
[ -n "$GREP_BIN" ] || { echo 'tmux-session-sidebar: grep not found' >&2; exit 1; }
PLUGIN_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")" && "$PWD_BIN")" || exit 1
SCRIPTS_DIR="$PLUGIN_DIR/scripts"

set_default() {
  local option="$1" default_value="$2"
  if [ -z "$("$TMUX_BIN" show-options -gvq "$option" 2>/dev/null)" ]; then
    "$TMUX_BIN" set-option -gq "$option" "$default_value"
  fi
}

binding_belongs_to_plugin() {
  local key="$1" binding
  [ -z "$key" ] && return 1
  binding="$("$TMUX_BIN" list-keys -T prefix "$key" 2>/dev/null || true)"
  [ -n "$binding" ] && printf '%s\n' "$binding" | "$GREP_BIN" -Fq "$SCRIPTS_DIR/open-sidebar.sh"
}

unbind_plugin_binding() {
  local key="$1"
  [ -z "$key" ] && return 0
  if binding_belongs_to_plugin "$key"; then
    "$TMUX_BIN" unbind-key -T prefix "$key" 2>/dev/null || true
  fi
}

install_heat_hooks() {
  local quoted_sync_script
  printf -v quoted_sync_script '%q' "$SCRIPTS_DIR/actions/sync-session-heat.sh"

  "$TMUX_BIN" set-hook -g client-attached[9701] \
    "run-shell \"$quoted_sync_script --session #{q:session_name}\""
  "$TMUX_BIN" set-hook -g client-detached[9702] \
    "run-shell \"$quoted_sync_script --session #{q:session_name}\""
  "$TMUX_BIN" set-hook -g client-session-changed[9703] \
    "run-shell \"$quoted_sync_script --session #{q:client_last_session} --session #{q:session_name}\""
}

main() {
  set_default @session-sidebar-key                  b
  set_default @session-sidebar-width                20
  set_default @session-sidebar-project-roots        "$HOME/projects"
  set_default @session-sidebar-use-fzf              on
  set_default @session-sidebar-close-after-switch   off
  set_default @session-sidebar-heat-colors          on
  set_default @session-sidebar-heat-half-life-hours 8
  set_default @session-sidebar-heat-stale-hours     24
  set_default @session-sidebar-heat-refresh-seconds 300

  local sidebar_key previous_key quoted_script quoted_quick_script quoted_sync_script slot
  sidebar_key="$("$TMUX_BIN" show-options -gvq @session-sidebar-key)"
  previous_key="$("$TMUX_BIN" show-options -gvq @session-sidebar-bound-key 2>/dev/null || true)"
  printf -v quoted_script '%q' "$SCRIPTS_DIR/open-sidebar.sh"
  printf -v quoted_quick_script '%q' "$SCRIPTS_DIR/actions/quick-switch-session.sh"

  if [ -n "$previous_key" ] && [ "$previous_key" != "$sidebar_key" ]; then
    unbind_plugin_binding "$previous_key"
  fi

  "$TMUX_BIN" bind-key -T prefix "$sidebar_key" \
    run-shell "$quoted_script #{q:client_name} #{q:window_id} #{q:pane_id} #{q:pane_current_path}"
  "$TMUX_BIN" set-option -gq @session-sidebar-bound-key "$sidebar_key"

  for slot in 1 2 3 4 5 6 7 8 9; do
    "$TMUX_BIN" bind-key -n "C-$slot" \
      run-shell "$quoted_quick_script --client #{q:client_name} --index $slot"
  done
  "$TMUX_BIN" bind-key -n C-0 \
    run-shell "$quoted_quick_script --client #{q:client_name} --index 10"

  install_heat_hooks
  printf -v quoted_sync_script '%q' "$SCRIPTS_DIR/actions/sync-session-heat.sh"
  "$TMUX_BIN" run-shell -b "$quoted_sync_script --all"
}

main "$@"
