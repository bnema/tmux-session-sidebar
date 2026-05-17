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
if [ -z "${DATE_BIN:-}" ]; then
  DATE_BIN="$(sidebar_require_command date)" || _sidebar_return_or_exit 1
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

sidebar_get_session_option() {
  # Usage: sidebar_get_session_option SESSION OPTION [DEFAULT]
  local session="$1" option="$2" default_value="${3:-}"
  local value=""
  [ -n "$session" ] || {
    printf '%s' "$default_value"
    return 0
  }
  value="$("$TMUX_BIN" show-options -t "$session" -vq "$option" 2>/dev/null)" || true
  if [ -n "$value" ]; then
    printf '%s' "$value"
  else
    printf '%s' "$default_value"
  fi
}

sidebar_set_session_option() {
  # Usage: sidebar_set_session_option SESSION OPTION VALUE
  local session="$1" option="$2" value="$3"
  [ -n "$session" ] || return 1
  "$TMUX_BIN" set-option -q -t "$session" "$option" "$value"
}

sidebar_get_pane_option() {
  # Usage: sidebar_get_pane_option PANE OPTION [DEFAULT]
  local pane_id="$1" option="$2" default_value="${3:-}"
  local value=""
  [ -n "$pane_id" ] || {
    printf '%s' "$default_value"
    return 0
  }
  value="$("$TMUX_BIN" show-options -p -t "$pane_id" -vq "$option" 2>/dev/null)" || true
  if [ -n "$value" ]; then
    printf '%s' "$value"
  else
    printf '%s' "$default_value"
  fi
}

sidebar_set_pane_option() {
  # Usage: sidebar_set_pane_option PANE OPTION VALUE
  local pane_id="$1" option="$2" value="$3"
  [ -n "$pane_id" ] || return 1
  "$TMUX_BIN" set-option -p -q -t "$pane_id" "$option" "$value"
}

sidebar_unset_pane_option() {
  # Usage: sidebar_unset_pane_option PANE OPTION
  local pane_id="$1" option="$2"
  [ -n "$pane_id" ] || return 1
  "$TMUX_BIN" set-option -p -u -q -t "$pane_id" "$option" >/dev/null 2>&1 || true
}

sidebar_now_epoch() {
  # Usage: sidebar_now_epoch
  local now="${SESSION_SIDEBAR_NOW:-}"
  case "$now" in
    ''|*[!0-9]*) "$DATE_BIN" +%s ;;
    *) printf '%s' "$now" ;;
  esac
}

sidebar_option_seconds() {
  # Usage: sidebar_option_seconds OPTION DEFAULT_HOURS
  local option="$1" default_hours="$2" raw_value seconds
  raw_value="$(sidebar_get_option "$option" "$default_hours")"
  case "$raw_value" in
    ''|*[!0-9]*) raw_value="$default_hours" ;;
  esac
  seconds=$((raw_value * 3600))
  if [ "$seconds" -le 0 ]; then
    seconds=$((default_hours * 3600))
  fi
  printf '%s' "$seconds"
}

sidebar_heat_half_life_seconds() {
  sidebar_option_seconds @session-sidebar-heat-half-life-hours 8
}

sidebar_heat_stale_seconds() {
  sidebar_option_seconds @session-sidebar-heat-stale-hours 24
}

sidebar_heat_refresh_seconds() {
  local raw_value
  raw_value="$(sidebar_get_option @session-sidebar-heat-refresh-seconds 300)"
  case "$raw_value" in
    ''|*[!0-9]*) printf '300' ;;
    *) printf '%s' "$raw_value" ;;
  esac
}

sidebar_heat_max_score() {
  # Usage: sidebar_heat_max_score
  # Cap persisted heat so long-lived attached sessions do not store arbitrarily
  # large scores between refreshes or session switches.
  sidebar_heat_stale_seconds
}

sidebar_clamp_heat_score() {
  # Usage: sidebar_clamp_heat_score SCORE MAX_SCORE
  local score="$1" max_score="$2"
  "$AWK_BIN" -v score="$score" -v max_score="$max_score" '
    BEGIN {
      if (score !~ /^-?[0-9]+([.][0-9]+)?$/) score = 0;
      if (max_score !~ /^[0-9]+([.][0-9]+)?$/ || max_score <= 0) max_score = 86400;
      if (score < 0) score = 0;
      if (score > max_score) score = max_score;
      printf "%.6f", score;
    }
  '
}

sidebar_session_attached_count() {
  # Usage: sidebar_session_attached_count SESSION
  local session="$1"
  local value="0"
  [ -n "$session" ] || {
    printf '0'
    return 0
  }
  value="$("$TMUX_BIN" display-message -p -t "$session" '#{session_attached}' 2>/dev/null || true)"
  case "$value" in
    ''|*[!0-9]*) value=0 ;;
  esac
  printf '%s' "$value"
}

sidebar_heat_score_after_interval() {
  # Usage: sidebar_heat_score_after_interval SCORE ELAPSED_SECONDS ATTACHED_COUNT HALF_LIFE_SECONDS
  local score="$1" elapsed="$2" attached_count="$3" half_life_seconds="$4"
  "$AWK_BIN" -v score="$score" -v elapsed="$elapsed" -v attached_count="$attached_count" -v half_life_seconds="$half_life_seconds" '
    BEGIN {
      if (score !~ /^-?[0-9]+([.][0-9]+)?$/) score = 0;
      if (elapsed !~ /^[0-9]+$/) elapsed = 0;
      if (attached_count !~ /^[0-9]+$/) attached_count = 0;
      if (half_life_seconds !~ /^[0-9]+$/ || half_life_seconds <= 0) half_life_seconds = 28800;
      activity_weight = (attached_count > 0 ? 1 : 0);
      exponent = log(0.5) * elapsed / half_life_seconds;
      if (exponent < -700) {
        decay = 0;
      } else {
        decay = exp(exponent);
      }
      printf "%.6f", (score * decay) + (elapsed * activity_weight);
    }
  '
}

sidebar_sync_session_heat() {
  # Usage: sidebar_sync_session_heat SESSION
  # Updates persisted recent-dwell heat state using the current session_attached count.
  # Attached sessions grow linearly between sync points, then decay exponentially;
  # the persisted score is capped to avoid runaway values after very long runs.
  # Prints tab-separated SCORE\tLAST_SEEN_AT\tATTACHED_COUNT.
  local session="$1"
  local now actual_attached last_updated stored_score stored_attached elapsed half_life_seconds last_seen_at new_score max_score

  [ -n "$session" ] || return 0
  sidebar_session_is_internal "$session" && return 0
  sidebar_session_exists "$session" || return 0

  now="$(sidebar_now_epoch)"
  actual_attached="$(sidebar_session_attached_count "$session")"
  last_updated="$(sidebar_get_session_option "$session" @session-sidebar-heat-updated-at '')"

  case "$last_updated" in
    ''|*[!0-9]*)
      sidebar_set_session_option "$session" @session-sidebar-heat-score 0
      sidebar_set_session_option "$session" @session-sidebar-heat-updated-at "$now"
      sidebar_set_session_option "$session" @session-sidebar-heat-attached-count "$actual_attached"
      if [ "$actual_attached" -gt 0 ]; then
        sidebar_set_session_option "$session" @session-sidebar-last-seen-at "$now"
        last_seen_at="$now"
      else
        last_seen_at="$(sidebar_get_session_option "$session" @session-sidebar-last-seen-at '')"
      fi
      printf '0\t%s\t%s\n' "$last_seen_at" "$actual_attached"
      return 0
      ;;
  esac

  stored_score="$(sidebar_get_session_option "$session" @session-sidebar-heat-score 0)"
  stored_attached="$(sidebar_get_session_option "$session" @session-sidebar-heat-attached-count "$actual_attached")"
  case "$stored_attached" in
    ''|*[!0-9]*) stored_attached="$actual_attached" ;;
  esac

  elapsed=$((now - last_updated))
  if [ "$elapsed" -lt 0 ]; then
    elapsed=0
  fi

  half_life_seconds="$(sidebar_heat_half_life_seconds)"
  new_score="$(sidebar_heat_score_after_interval "$stored_score" "$elapsed" "$stored_attached" "$half_life_seconds")"
  max_score="$(sidebar_heat_max_score)"
  new_score="$(sidebar_clamp_heat_score "$new_score" "$max_score")"
  last_seen_at="$(sidebar_get_session_option "$session" @session-sidebar-last-seen-at '')"
  if [ "$stored_attached" -gt 0 ] || [ "$actual_attached" -gt 0 ]; then
    last_seen_at="$now"
    sidebar_set_session_option "$session" @session-sidebar-last-seen-at "$last_seen_at"
  fi

  sidebar_set_session_option "$session" @session-sidebar-heat-score "$new_score"
  sidebar_set_session_option "$session" @session-sidebar-heat-updated-at "$now"
  sidebar_set_session_option "$session" @session-sidebar-heat-attached-count "$actual_attached"

  printf '%s\t%s\t%s\n' "$new_score" "$last_seen_at" "$actual_attached"
}

sidebar_sync_all_session_heat() {
  # Usage: sidebar_sync_all_session_heat [CLIENT]
  local client="${1:-}"
  local session_name attached_state window_count is_current
  while IFS=$'\t' read -r session_name attached_state window_count is_current; do
    [ -n "$session_name" ] || continue
    sidebar_sync_session_heat "$session_name" >/dev/null || true
  done < <(sidebar_list_sessions "$client")
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

sidebar_session_is_internal() {
  # Usage: sidebar_session_is_internal NAME
  # Internal helper sessions are deliberately hidden from sidebar navigation.
  local name="$1"
  [[ "$name" == __* ]]
}

sidebar_list_visible_sessions() {
  # Usage: sidebar_list_visible_sessions [CLIENT] [SHOW_NUMBERED]
  # Prints the sidebar-visible session rows using the default filtering model.
  local client="${1:-}"
  local show_numbered="${2:-off}"
  local session_name attached_state window_count is_current

  while IFS=$'\t' read -r session_name attached_state window_count is_current; do
    [ -z "$session_name" ] && continue
    if sidebar_session_is_internal "$session_name"; then
      continue
    fi
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

sidebar_list_sidebar_panes() {
  # Usage: sidebar_list_sidebar_panes
  # Prints pane ids for panes marked as sidebar panes.
  local awk_program="\$2 == 1 { print \$1 }"
  "$TMUX_BIN" list-panes -a -F '#{pane_id}	#{@session-sidebar-pane}' 2>/dev/null |
    "$AWK_BIN" -F $'\t' "$awk_program"
}

sidebar_pane_window_id() {
  # Usage: sidebar_pane_window_id PANE_ID
  local pane_id="$1"
  [ -z "$pane_id" ] && return 1
  "$TMUX_BIN" display-message -p -t "$pane_id" '#{window_id}' 2>/dev/null
}

sidebar_window_saved_layout() {
  # Usage: sidebar_window_saved_layout WINDOW_ID
  local window_id="$1"
  [ -z "$window_id" ] && return 1
  "$TMUX_BIN" show-options -w -v -t "$window_id" @session-sidebar-window-layout 2>/dev/null || true
}

sidebar_window_save_layout() {
  # Usage: sidebar_window_save_layout WINDOW_ID
  local window_id="$1"
  local layout=""
  [ -z "$window_id" ] && return 1
  layout="$("$TMUX_BIN" display-message -p -t "$window_id" '#{window_layout}' 2>/dev/null || true)"
  [ -n "$layout" ] || return 1
  "$TMUX_BIN" set-option -wq -t "$window_id" @session-sidebar-window-layout "$layout"
}

sidebar_window_restore_layout() {
  # Usage: sidebar_window_restore_layout WINDOW_ID
  local window_id="$1"
  local layout=""
  [ -z "$window_id" ] && return 1
  layout="$(sidebar_window_saved_layout "$window_id")"
  [ -n "$layout" ] || return 1
  "$TMUX_BIN" select-layout -t "$window_id" "$layout" >/dev/null 2>&1 || true
  "$TMUX_BIN" set-option -wu -t "$window_id" @session-sidebar-window-layout >/dev/null 2>&1 || true
}
