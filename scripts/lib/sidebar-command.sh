#!/usr/bin/env bash
# Command construction helpers shared by the sidebar and refresh actions.

sidebar_quote_args() {
  local quoted="" arg
  for arg in "$@"; do
    printf -v quoted '%s%q ' "$quoted" "$arg"
  done
  printf '%s' "$quoted"
}

sidebar_render_entries_command() {
  local sidebar_script="$1"
  local client_name="$2"
  local show_numbered_sessions="$3"
  local sidebar_pane_id="${4:-}"
  local active_filter="${5:-}"
  local source_path="${6:-}"
  local -a args

  [ -n "$sidebar_script" ] || return 1
  [ -n "$client_name" ] || return 1

  args=("$sidebar_script" --client "$client_name" --show-numbered-sessions "$show_numbered_sessions")
  if [ -n "$active_filter" ]; then
    args+=(--active-filter "$active_filter")
  fi
  if [ -n "$source_path" ]; then
    args+=(--source-path "$source_path")
  fi
  if [ -n "$sidebar_pane_id" ]; then
    args+=(--sidebar-pane "$sidebar_pane_id")
  fi
  args+=(--render-entries)
  sidebar_quote_args "${args[@]}"
}
