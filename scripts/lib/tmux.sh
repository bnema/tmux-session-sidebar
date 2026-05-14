#!/usr/bin/env bash
# Strict mode is intentionally not enabled here because this file is sourced by
# other scripts and must not silently change the caller's shell options.
# tmux-session-sidebar — shared tmux helpers
# Sourceable with no side effects. All helpers accept explicit targets where
# possible so later scripts can run against an isolated tmux server.

_sidebar_return_or_exit() {
  local code="${1:-1}"
  if (return 0 2>/dev/null); then
    return "$code"
  fi
  exit "$code"
}

sidebar_require_command() {
  local name="$1"
  local resolved=""
  resolved="$(command -v "$name" 2>/dev/null || true)"
  if [ -z "$resolved" ]; then
    echo "tmux-session-sidebar: required command not found: $name" >&2
    return 1
  fi
  printf '%s' "$resolved"
}

if [ -z "${TMUX_BIN:-}" ]; then
  TMUX_BIN="$(sidebar_require_command tmux)" || _sidebar_return_or_exit 1
fi
if [ -z "${AWK_BIN:-}" ]; then
  AWK_BIN="$(sidebar_require_command awk)" || _sidebar_return_or_exit 1
fi

require_arg() {
  local flag="$1"
  local value="${2:-}"
  if [ -z "$value" ]; then
    echo "tmux-session-sidebar: missing value for $flag" >&2
    exit 1
  fi
}

sidebar_get_option() {
  # Usage: sidebar_get_option OPTION [DEFAULT]
  # Prints the option value or the default. Does not set the option.
  local option="$1" default_value="${2:-}"
  local value
  value="$("$TMUX_BIN" show-options -gvq "$option" 2>/dev/null)" || true
  if [ -n "$value" ]; then
    printf '%s' "$value"
  else
    printf '%s' "$default_value"
  fi
}

sidebar_current_client() {
  # Usage: sidebar_current_client [CLIENT]
  # Returns the client name to operate on.
  if [ $# -ge 1 ] && [ -n "$1" ]; then
    printf '%s' "$1"
  elif [ -n "${TMUX:-}" ]; then
    "$TMUX_BIN" display-message -p '#{client_name}'
  else
    echo "tmux-session-sidebar: no tmux client available" >&2
    return 1
  fi
}

sidebar_current_window_id() {
  # Usage: sidebar_current_window_id [WINDOW_ID]
  if [ $# -ge 1 ] && [ -n "$1" ]; then
    printf '%s' "$1"
  elif [ -n "${TMUX:-}" ]; then
    "$TMUX_BIN" display-message -p '#{window_id}'
  else
    echo "tmux-session-sidebar: no tmux client available" >&2
    return 1
  fi
}

sidebar_current_pane_id() {
  # Usage: sidebar_current_pane_id [PANE_ID]
  if [ $# -ge 1 ] && [ -n "$1" ]; then
    printf '%s' "$1"
  elif [ -n "${TMUX:-}" ]; then
    "$TMUX_BIN" display-message -p '#{pane_id}'
  else
    echo "tmux-session-sidebar: no tmux client available" >&2
    return 1
  fi
}

sidebar_current_path() {
  # Usage: sidebar_current_path [PATH]
  if [ $# -ge 1 ] && [ -n "$1" ]; then
    printf '%s' "$1"
  elif [ -n "${TMUX:-}" ]; then
    "$TMUX_BIN" display-message -p '#{pane_current_path}'
  else
    echo "tmux-session-sidebar: no tmux client available" >&2
    return 1
  fi
}

sidebar_current_session() {
  # Usage: sidebar_current_session [CLIENT]
  local client
  client="$(sidebar_current_client "$@")" || return 1
  "$TMUX_BIN" display-message -p -t "$client" '#{client_session}'
}

sidebar_list_sessions() {
  # Usage: sidebar_list_sessions [CLIENT]
  # Prints tab-separated lines: NAME\tATTACHED_STATE\tWINDOW_COUNT\tIS_CURRENT
  local client current_session name attached windows is_current
  client="${1:-}"
  current_session=""

  if [ -n "$client" ]; then
    current_session="$("$TMUX_BIN" display-message -p -t "$client" '#{client_session}' 2>/dev/null || true)"
  elif [ -n "${TMUX:-}" ]; then
    current_session="$("$TMUX_BIN" display-message -p '#{session_name}' 2>/dev/null || true)"
  fi

  while IFS=$'\t' read -r name attached windows; do
    [ -z "$name" ] && continue
    if [ "$name" = "$current_session" ]; then
      is_current="current"
    else
      is_current=""
    fi
    printf '%s\t%s\t%s\t%s\n' "$name" "$attached" "$windows" "$is_current"
  done < <("$TMUX_BIN" list-sessions -F '#{session_name}	#{?session_attached,attached,detached}	#{session_windows}' 2>/dev/null)
}

sidebar_session_is_numeric() {
  # Usage: sidebar_session_is_numeric NAME
  local name="$1"
  [[ "$name" =~ ^[0-9]+$ ]]
}

sidebar_list_visible_sessions() {
  # Usage: sidebar_list_visible_sessions [CLIENT] [SHOW_NUMBERED]
  # Prints the sidebar-visible session rows using the default filtering model.
  local client="${1:-}"
  local show_numbered="${2:-off}"
  local session_name attached_state window_count is_current

  while IFS=$'\t' read -r session_name attached_state window_count is_current; do
    [ -z "$session_name" ] && continue
    if [ "$show_numbered" != "on" ] && sidebar_session_is_numeric "$session_name"; then
      continue
    fi
    printf '%s\t%s\t%s\t%s\n' "$session_name" "$attached_state" "$window_count" "$is_current"
  done < <(sidebar_list_sessions "$client")
}

sidebar_visible_session_name_at_index() {
  # Usage: sidebar_visible_session_name_at_index CLIENT INDEX [SHOW_NUMBERED]
  local client="$1"
  local index="$2"
  local show_numbered="${3:-off}"
  local current=0
  local session_name attached_state window_count is_current

  if ! [[ "$index" =~ ^[0-9]+$ ]] || [ "$index" -le 0 ]; then
    return 1
  fi

  while IFS=$'\t' read -r session_name attached_state window_count is_current; do
    [ -z "$session_name" ] && continue
    current=$((current + 1))
    if [ "$current" -eq "$index" ]; then
      printf '%s' "$session_name"
      return 0
    fi
  done < <(sidebar_list_visible_sessions "$client" "$show_numbered")

  return 1
}

sidebar_session_exists() {
  # Usage: sidebar_session_exists NAME
  local name="$1"
  [ -n "$name" ] && "$TMUX_BIN" has-session -t "=$name" 2>/dev/null
}

sidebar_validate_session_name() {
  # Usage: sidebar_validate_session_name NAME
  # Returns 0 if the name is valid for a tmux session, nonzero otherwise.
  # Valid characters: A-Z a-z 0-9 _ -
  local name="$1"
  if [ -z "$name" ]; then
    echo "tmux-session-sidebar: session name must not be empty" >&2
    return 1
  fi
  if ! [[ "$name" =~ ^[A-Za-z0-9_-]+$ ]]; then
    echo "tmux-session-sidebar: session name contains invalid characters: $name" >&2
    return 1
  fi
  return 0
}

sidebar_session_target() {
  # Usage: sidebar_session_target NAME
  # Returns a validated session target string for tmux session-scoped commands.
  # Session names are already normalized and validated elsewhere, so a plain
  # quoted session name is the most compatible target form here.
  local name="$1"
  [ -z "$name" ] && return 1
  printf '%s' "$name"
}

sidebar_existing_sidebar_pane() {
  # Usage: sidebar_existing_sidebar_pane WINDOW_ID
  # Prints the pane id of the first pane in the window that has
  # @session-sidebar-pane == 1, or nothing if none is found.
  local window_id="$1"
  local awk_program="\$2 == 1 { print \$1; exit }"
  [ -z "$window_id" ] && return 0
  "$TMUX_BIN" list-panes -t "$window_id" -F '#{pane_id}	#{@session-sidebar-pane}' 2>/dev/null |
    "$AWK_BIN" -F $'\t' "$awk_program"
}
