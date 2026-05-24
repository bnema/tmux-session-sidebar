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
  local table="$1" key="$2" binding
  [ -z "$key" ] && return 1
  binding="$("$TMUX_BIN" list-keys -T "$table" "$key" 2>/dev/null || true)"
  [ -n "$binding" ] && printf '%s\n' "$binding" | "$GREP_BIN" -Fq "tmux-session-sidebar sidebar toggle"
}

unbind_plugin_binding() {
  local key="$1"
  [ -z "$key" ] && return 0
  if binding_belongs_to_plugin root "$key"; then
    "$TMUX_BIN" unbind-key -T root "$key" 2>/dev/null || true
  fi
  if binding_belongs_to_plugin prefix "$key"; then
    "$TMUX_BIN" unbind-key -T prefix "$key" 2>/dev/null || true
  fi
}

install_runtime_hooks() {
  local quoted_runtime="$1"
  "$TMUX_BIN" set-hook -g client-attached[9701] \
    "run-shell \"$quoted_runtime hook client-attached --client #{q:client_name}\""
  "$TMUX_BIN" set-hook -g client-detached[9702] \
    "run-shell \"$quoted_runtime hook client-detached --client #{q:client_name}\""
  "$TMUX_BIN" set-hook -g client-session-changed[9703] \
    "run-shell -b \"$quoted_runtime hook client-session-changed --client #{q:client_name}\""
  "$TMUX_BIN" set-hook -g client-resized[9704] \
    "run-shell -b \"$quoted_runtime hook client-resized --client #{q:client_name}\""
  "$TMUX_BIN" set-hook -g window-resized[9705] \
    "run-shell -b \"$quoted_runtime hook window-resized --window #{q:hook_window}\""
}

main() {
  set_default @session-sidebar-key                  M-b
  set_default @session-sidebar-width                20
  set_default @session-sidebar-project-roots        "$HOME/projects"
  set_default @session-sidebar-use-fzf              on
  set_default @session-sidebar-close-after-switch   off
  set_default @session-sidebar-heat-colors          on
  set_default @session-sidebar-heat-half-life-hours 8
  set_default @session-sidebar-heat-stale-hours     24
  set_default @session-sidebar-heat-refresh-seconds 5
  set_default @session-sidebar-activity-debug-log off
  set_default @session-sidebar-agent-attention on

  local sidebar_key previous_key quoted_daemon_control quoted_runtime quoted_state_dir runtime_bin slot state_dir
  sidebar_key="$("$TMUX_BIN" show-options -gvq @session-sidebar-key)"
  previous_key="$("$TMUX_BIN" show-options -gvq @session-sidebar-bound-key 2>/dev/null || true)"
  if [ "$sidebar_key" = b ] && { [ -z "$previous_key" ] || [ "$previous_key" = b ]; }; then
    sidebar_key=M-b
    "$TMUX_BIN" set-option -gq @session-sidebar-key "$sidebar_key"
  fi
  runtime_bin="$("$SCRIPTS_DIR/ensure-runtime.sh")"
  printf -v quoted_daemon_control '%q' "$SCRIPTS_DIR/daemon-control.sh"
  printf -v quoted_runtime '%q' "$runtime_bin"
  state_dir="${XDG_STATE_HOME:-$HOME/.local/state}/tmux-session-sidebar"
  printf -v quoted_state_dir '%q' "$state_dir"

  if [ -n "$previous_key" ] && [ "$previous_key" != "$sidebar_key" ]; then
    unbind_plugin_binding "$previous_key"
  fi

  "$TMUX_BIN" run-shell -b "$quoted_daemon_control $quoted_runtime $quoted_state_dir"
  "$TMUX_BIN" bind-key -n "$sidebar_key" \
    run-shell "$quoted_runtime sidebar toggle --client #{q:client_name}"
  "$TMUX_BIN" set-option -gq @session-sidebar-bound-key "$sidebar_key"

  for slot in 1 2 3 4 5 6 7 8 9; do
    "$TMUX_BIN" bind-key -n "C-$slot" \
      run-shell "$quoted_runtime action quick-switch --client #{q:client_name} --slot $slot"
  done
  "$TMUX_BIN" bind-key -n C-0 \
    run-shell "$quoted_runtime action quick-switch --client #{q:client_name} --slot 10"

  install_runtime_hooks "$quoted_runtime"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
