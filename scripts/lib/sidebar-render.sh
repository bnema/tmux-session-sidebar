#!/usr/bin/env bash
# Rendering helpers for sidebar session rows. Source after tmux.sh and after
# sidebar.sh has initialized its render-related globals.

sidebar_pane_width() {
  local width
  if [ -z "$sidebar_pane_id" ]; then
    printf '30'
    return 0
  fi
  width="$("$TMUX_BIN" display-message -p -t "$sidebar_pane_id" '#{pane_width}' 2>/dev/null || true)"
  case "$width" in
    ''|*[!0-9]*) width=30 ;;
  esac
  printf '%s' "$width"
}

truncate_label() {
  local text="$1"
  local max_width="$2"

  if [ "${#text}" -le "$max_width" ]; then
    printf '%s' "$text"
    return 0
  fi

  if [ "$max_width" -le 3 ]; then
    printf '%s' "${text:0:max_width}"
    return 0
  fi

  printf '%s...' "${text:0:$((max_width - 3))}"
}

quick_switch_badge() {
  local index="$1"

  case "$index" in
    1|2|3|4|5|6|7|8|9)
      printf '[%s] ' "$index"
      ;;
    10)
      printf '[0] '
      ;;
    *)
      printf '    '
      ;;
  esac
}

render_session_label() {
  local session_name="$1"
  local is_current="$2"
  local quick_switch_label="$3"
  local pane_width="$4"
  local prefix name_width rendered_name

  prefix="  "
  if [ "$is_current" = "current" ]; then
    prefix="* "
  fi

  name_width=$((pane_width - ${#prefix} - ${#quick_switch_label}))
  if [ "$name_width" -lt 4 ]; then
    name_width=4
  fi

  rendered_name="$(truncate_label "$session_name" "$name_width")"
  printf '%s%s%s' "$prefix" "$quick_switch_label" "$rendered_name"
}

heat_colors_enabled() {
  [ "$(sidebar_get_option @session-sidebar-heat-colors on)" != "off" ]
}

terminal_color_capability() {
  local override term_features colors

  override="${SESSION_SIDEBAR_COLOR_CAPABILITY:-}"
  case "$override" in
    none|basic|256|rgb)
      printf '%s' "$override"
      return 0
      ;;
  esac

  term_features="$("$TMUX_BIN" display-message -p -t "$client_name" '#{client_termfeatures}' 2>/dev/null || true)"
  case "$term_features" in
    *RGB*)
      printf 'rgb'
      return 0
      ;;
  esac

  colors="$(tput colors 2>/dev/null || true)"
  case "$colors" in
    ''|*[!0-9]*) printf 'none' ;;
    256|[3-9][0-9][0-9]*) printf '256' ;;
    [1-9]|[1-9][0-9]) printf 'basic' ;;
    *) printf 'none' ;;
  esac
}

session_heat_bucket() {
  local heat_score="$1"
  local last_seen_at="$2"
  local is_current="$3"
  local now stale_seconds half_life_seconds

  if [ "$is_current" = "current" ]; then
    printf 'current'
    return 0
  fi

  now="${render_now:-$(sidebar_now_epoch)}"
  stale_seconds="${render_stale_seconds:-$(sidebar_heat_stale_seconds)}"
  case "$last_seen_at" in
    ''|*[!0-9]*) ;;
    *)
      if [ $((now - last_seen_at)) -ge "$stale_seconds" ]; then
        printf 'stale'
        return 0
      fi
      ;;
  esac

  half_life_seconds="${render_half_life_seconds:-$(sidebar_heat_half_life_seconds)}"
  "$AWK_BIN" -v score="$heat_score" -v half_life_seconds="$half_life_seconds" '
    BEGIN {
      if (score !~ /^-?[0-9]+([.][0-9]+)?$/) score = 0;
      if (half_life_seconds !~ /^[0-9]+$/ || half_life_seconds <= 0) half_life_seconds = 28800;
      if (score >= half_life_seconds / 4) {
        print "hot";
      } else if (score >= half_life_seconds / 12) {
        print "warm";
      } else if (score >= half_life_seconds / 48) {
        print "cool";
      } else {
        print "none";
      }
    }
  '
}

session_heat_style() {
  local bucket="$1"
  local capability="$2"

  case "$capability:$bucket" in
    rgb:current) printf '\033[1;38;2;152;251;152m' ;;
    rgb:hot)     printf '\033[38;2;122;232;122m' ;;
    rgb:warm)    printf '\033[38;2;106;198;106m' ;;
    rgb:cool)    printf '\033[38;2;124;154;124m' ;;
    rgb:stale)   printf '\033[2;38;2;140;140;140m' ;;
    256:current) printf '\033[1;38;5;121m' ;;
    256:hot)     printf '\033[38;5;114m' ;;
    256:warm)    printf '\033[38;5;108m' ;;
    256:cool)    printf '\033[38;5;72m' ;;
    256:stale)   printf '\033[2;38;5;244m' ;;
    basic:current) printf '\033[1;32m' ;;
    basic:hot)     printf '\033[32m' ;;
    basic:warm)    printf '\033[2;32m' ;;
    basic:cool)    printf '\033[2;37m' ;;
    basic:stale)   printf '\033[2;37m' ;;
    *) printf '' ;;
  esac
}

render_heat_colors_enabled=""
render_color_capability=""
render_now=""
render_stale_seconds=""
render_half_life_seconds=""

prepare_render_context() {
  if heat_colors_enabled; then
    render_heat_colors_enabled="on"
    render_color_capability="$(terminal_color_capability)"
    render_now="$(sidebar_now_epoch)"
    render_stale_seconds="$(sidebar_heat_stale_seconds)"
    render_half_life_seconds="$(sidebar_heat_half_life_seconds)"
  else
    render_heat_colors_enabled="off"
    render_color_capability=""
    render_now=""
    render_stale_seconds=""
    render_half_life_seconds=""
  fi
}

colorize_session_label() {
  local label="$1"
  local heat_score="$2"
  local last_seen_at="$3"
  local is_current="$4"
  local bucket style

  if [ "$render_heat_colors_enabled" != "on" ]; then
    printf '%s' "$label"
    return 0
  fi

  bucket="$(session_heat_bucket "$heat_score" "$last_seen_at" "$is_current")"
  case "$bucket" in
    none)
      printf '%s' "$label"
      return 0
      ;;
  esac

  style="$(session_heat_style "$bucket" "$render_color_capability")"
  if [ -z "$style" ]; then
    printf '%s' "$label"
    return 0
  fi

  printf '%b%s%b' "$style" "$label" $'\033[0m'
}

toggle_numbered_sessions() {
  if [ "$show_numbered_sessions" = "on" ]; then
    show_numbered_sessions="off"
  else
    show_numbered_sessions="on"
  fi
  persist_sidebar_refresh_state
}

numbered_sessions_status_label() {
  if [ "$show_numbered_sessions" = "on" ]; then
    printf 'shown'
  else
    printf 'hidden'
  fi
}

render_session_entries() {
  local pane_width usable_width label colored_label quick_switch_label quick_switch_index heat_snapshot heat_score heat_last_seen
  pane_width="$(sidebar_pane_width)"
  usable_width=$((pane_width - 4))
  if [ "$usable_width" -lt 12 ]; then
    usable_width=12
  fi

  prepare_render_context
  quick_switch_index=0
  sidebar_list_visible_sessions "$client_name" "$show_numbered_sessions" | while IFS=$'\t' read -r session_name _ _ is_current; do
    [ -z "$session_name" ] && continue
    if sidebar_session_is_numeric "$session_name"; then
      quick_switch_label="$(quick_switch_badge '')"
    else
      quick_switch_index=$((quick_switch_index + 1))
      quick_switch_label="$(quick_switch_badge "$quick_switch_index")"
    fi
    heat_score=""
    heat_last_seen=""
    if [ "$render_heat_colors_enabled" = "on" ]; then
      heat_snapshot="$(sidebar_sync_session_heat "$session_name")"
      heat_score="${heat_snapshot%%$'\t'*}"
      heat_last_seen="${heat_snapshot#*$'\t'}"
      heat_last_seen="${heat_last_seen%%$'\t'*}"
    fi
    label="$(render_session_label "$session_name" "$is_current" "$quick_switch_label" "$usable_width")"
    colored_label="$(colorize_session_label "$label" "$heat_score" "$heat_last_seen" "$is_current")"
    printf '%s\t%s\n' "$session_name" "$colored_label"
  done
}

filtered_session_entries() {
  if [ -z "$active_filter" ]; then
    render_session_entries
    return 0
  fi

  local status

  set +e
  render_session_entries | "$FZF_BIN" \
    --ansi \
    --filter "$active_filter" \
    --delimiter=$'\t' \
    --with-nth=2
  status="$?"
  set -e

  if [ "$status" -eq 1 ]; then
    return 0
  fi
  return "$status"
}

