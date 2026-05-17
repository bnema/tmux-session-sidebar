#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
SCRIPT_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")" && "$PWD_BIN")" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/lib/tmux.sh"
SED_BIN="$(sidebar_require_command sed)" || exit 1
FZF_BIN="$(command -v fzf 2>/dev/null || true)"
MKTEMP_BIN="$(command -v mktemp 2>/dev/null || true)"
CURL_BIN="$(command -v curl 2>/dev/null || true)"
FZF_SUPPORTS_LISTEN="off"
if [ -n "$FZF_BIN" ]; then
  case "${SESSION_SIDEBAR_FZF_LISTEN:-auto}" in
    on)
      FZF_SUPPORTS_LISTEN="on"
      ;;
    off)
      FZF_SUPPORTS_LISTEN="off"
      ;;
    *)
      case "$("$FZF_BIN" --help 2>/dev/null || true)" in
        *"--listen=SOCKET_PATH"*) FZF_SUPPORTS_LISTEN="on" ;;
      esac
      ;;
  esac
fi

client_name=""
source_path=""
show_numbered_sessions="off"
active_filter=""
render_entries_only="off"
refresh_loop_mode="off"
refresh_socket=""
refresh_interval=""
sidebar_pane_id=""

while [ $# -gt 0 ]; do
  case "$1" in
    --client)
      require_arg "$1" "${2:-}"
      client_name="$2"
      shift 2
      ;;
    --source-path)
      require_arg "$1" "${2:-}"
      source_path="$2"
      shift 2
      ;;
    --show-numbered-sessions)
      require_arg "$1" "${2:-}"
      show_numbered_sessions="$2"
      shift 2
      ;;
    --active-filter)
      require_arg "$1" "${2:-}"
      active_filter="$2"
      shift 2
      ;;
    --sidebar-pane)
      require_arg "$1" "${2:-}"
      sidebar_pane_id="$2"
      shift 2
      ;;
    --render-entries)
      render_entries_only="on"
      shift
      ;;
    --fzf-refresh-loop)
      refresh_loop_mode="on"
      shift
      ;;
    --socket)
      require_arg "$1" "${2:-}"
      refresh_socket="$2"
      shift 2
      ;;
    --interval)
      require_arg "$1" "${2:-}"
      refresh_interval="$2"
      shift 2
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

client_name="$(sidebar_current_client "$client_name")" || exit 1

if [ "$render_entries_only" != "on" ] && [ "$refresh_loop_mode" != "on" ]; then
  if [ -z "$sidebar_pane_id" ]; then
    sidebar_pane_id="$(sidebar_current_pane_id)" || exit 1
  fi
  "$TMUX_BIN" set-option -p -t "$sidebar_pane_id" @session-sidebar-pane 1
fi

persist_sidebar_refresh_state() {
  [ -n "$sidebar_pane_id" ] || return 0
  sidebar_set_pane_option "$sidebar_pane_id" @session-sidebar-client "$client_name"
  sidebar_set_pane_option "$sidebar_pane_id" @session-sidebar-show-numbered-sessions "$show_numbered_sessions"
  if [ -n "$active_filter" ]; then
    sidebar_set_pane_option "$sidebar_pane_id" @session-sidebar-active-filter "$active_filter"
  else
    sidebar_unset_pane_option "$sidebar_pane_id" @session-sidebar-active-filter
  fi
  if [ -n "$source_path" ]; then
    sidebar_set_pane_option "$sidebar_pane_id" @session-sidebar-source-path "$source_path"
  else
    sidebar_unset_pane_option "$sidebar_pane_id" @session-sidebar-source-path
  fi
}

persist_sidebar_refresh_socket() {
  local socket_path="${1:-}"
  [ -n "$sidebar_pane_id" ] || return 0
  if [ -n "$socket_path" ]; then
    sidebar_set_pane_option "$sidebar_pane_id" @session-sidebar-refresh-socket "$socket_path"
  else
    sidebar_unset_pane_option "$sidebar_pane_id" @session-sidebar-refresh-socket
  fi
}

schedule_sidebar_layout_restore_on_exit() {
  local sidebar_window_id quoted_restore_script restore_cmd

  [ -n "$sidebar_pane_id" ] || return 0
  sidebar_window_id="$(sidebar_pane_window_id "$sidebar_pane_id" 2>/dev/null || true)"
  [ -n "$sidebar_window_id" ] || return 0
  [ -n "$(sidebar_window_saved_layout "$sidebar_window_id")" ] || return 0

  printf -v quoted_restore_script '%q' "$SCRIPT_DIR/actions/restore-layout-after-pane-close.sh"
  printf -v restore_cmd '%s --window %q --pane %q' \
    "$quoted_restore_script" \
    "$sidebar_window_id" \
    "$sidebar_pane_id"
  "$TMUX_BIN" run-shell -b "$restore_cmd" >/dev/null 2>&1 || true
}

persist_sidebar_refresh_state
persist_sidebar_refresh_socket ""

if [ "$render_entries_only" != "on" ] && [ "$refresh_loop_mode" != "on" ]; then
  trap schedule_sidebar_layout_restore_on_exit EXIT
fi

quote_args() {
  local quoted="" arg
  for arg in "$@"; do
    printf -v quoted '%s%q ' "$quoted" "$arg"
  done
  printf '%s' "$quoted"
}

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

  now="$(sidebar_now_epoch)"
  stale_seconds="$(sidebar_heat_stale_seconds)"
  case "$last_seen_at" in
    ''|*[!0-9]*) ;;
    *)
      if [ $((now - last_seen_at)) -ge "$stale_seconds" ]; then
        printf 'stale'
        return 0
      fi
      ;;
  esac

  half_life_seconds="$(sidebar_heat_half_life_seconds)"
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

colorize_session_label() {
  local label="$1"
  local heat_score="$2"
  local last_seen_at="$3"
  local is_current="$4"
  local bucket capability style

  if ! heat_colors_enabled; then
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

  capability="$(terminal_color_capability)"
  style="$(session_heat_style "$bucket" "$capability")"
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

  quick_switch_index=0
  sidebar_list_visible_sessions "$client_name" "$show_numbered_sessions" | while IFS=$'\t' read -r session_name _ _ is_current; do
    [ -z "$session_name" ] && continue
    if sidebar_session_is_numeric "$session_name"; then
      quick_switch_label="$(quick_switch_badge '')"
    else
      quick_switch_index=$((quick_switch_index + 1))
      quick_switch_label="$(quick_switch_badge "$quick_switch_index")"
    fi
    heat_snapshot="$(sidebar_sync_session_heat "$session_name")"
    heat_score="${heat_snapshot%%$'\t'*}"
    heat_last_seen="${heat_snapshot#*$'\t'}"
    heat_last_seen="${heat_last_seen%%$'\t'*}"
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

  render_session_entries | "$FZF_BIN" \
    --filter "$active_filter" \
    --delimiter=$'\t' \
    --with-nth=2 || true
}

render_entries_command() {
  local args
  args=("$SCRIPT_DIR/sidebar.sh" --client "$client_name" --show-numbered-sessions "$show_numbered_sessions")
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
  quote_args "${args[@]}"
}

start_fzf_refresh_loop() {
  local interval_value="$1"
  local -a args

  [ -n "$refresh_socket" ] || return 1
  args=("$SCRIPT_DIR/sidebar.sh" --fzf-refresh-loop --socket "$refresh_socket" --interval "$interval_value" --client "$client_name" --show-numbered-sessions "$show_numbered_sessions")
  if [ -n "$source_path" ]; then
    args+=(--source-path "$source_path")
  fi
  if [ -n "$sidebar_pane_id" ]; then
    args+=(--sidebar-pane "$sidebar_pane_id")
  fi

  "${args[@]}" >/dev/null 2>&1 &
  printf '%s' "$!"
}

run_fzf_refresh_loop() {
  local reload_command payload interval_value

  [ -n "$refresh_socket" ] || return 0
  [ -n "$refresh_interval" ] || return 0
  [ -n "$CURL_BIN" ] || return 0
  case "$refresh_interval" in
    ''|*[!0-9]*) return 0 ;;
  esac
  interval_value="$refresh_interval"
  if [ "$interval_value" -le 0 ]; then
    return 0
  fi

  reload_command="$(render_entries_command)"
  payload="reload-sync($reload_command)"

  while sleep "$interval_value"; do
    "$CURL_BIN" --silent --show-error --unix-socket "$refresh_socket" http -d "$payload" >/dev/null 2>&1 || break
  done
}

session_from_selection() {
  local selection="$1"
  printf '%s' "${selection%%$'\t'*}"
}

display_from_selection() {
  local selection="$1"
  if [[ "$selection" == *$'\t'* ]]; then
    printf '%s' "${selection#*$'\t'}"
  else
    printf '%s' "$selection"
  fi
}

session_from_index() {
  local lines="$1"
  local choice="$2"
  local selected_line

  selected_line="$(printf '%s\n' "$lines" | "$SED_BIN" -n "${choice}p")"
  [ -n "$selected_line" ] || return 1
  session_from_selection "$selected_line"
}

current_session_name() {
  sidebar_current_session "$client_name"
}

should_close_after_switch() {
  local close_after_switch
  close_after_switch="$(sidebar_get_option @session-sidebar-close-after-switch off)"
  [ "$close_after_switch" = "on" ]
}

should_use_fzf_live_reload() {
  [ "$FZF_SUPPORTS_LISTEN" = "on" ] || return 1
  [ -n "$CURL_BIN" ] || return 1
  [ -n "$MKTEMP_BIN" ] || return 1
}

should_use_fzf_refresh_loop() {
  local interval_value

  should_use_fzf_live_reload || return 1
  heat_colors_enabled || return 1

  interval_value="$(sidebar_heat_refresh_seconds)"
  case "$interval_value" in
    ''|*[!0-9]*) return 1 ;;
  esac
  [ "$interval_value" -gt 0 ]
}

prompt_session_target() {
  local lines="$1"
  local prompt="$2"
  local default_session selected_session choice

  default_session="$(current_session_name)"
  printf '%s [Enter=current: %s]: ' "$prompt" "$default_session" >&2
  read -r choice || return 1

  if [ -z "$choice" ]; then
    printf '%s' "$default_session"
    return 0
  fi

  if ! [[ "$choice" =~ ^[0-9]+$ ]]; then
    echo 'tmux-session-sidebar: invalid selection' >&2
    return 1
  fi

  selected_session="$(session_from_index "$lines" "$choice")" || {
    echo 'tmux-session-sidebar: invalid selection' >&2
    return 1
  }
  printf '%s' "$selected_session"
}

handle_fzf_action() {
  local key="$1"
  local selection="$2"
  local session_name

  case "$key" in
    alt-n)
      if "$SCRIPT_DIR/actions/create-project-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        if should_close_after_switch; then
          return 1
        fi
        return 0
      fi
      return 0
      ;;
    alt-g)
      if "$SCRIPT_DIR/actions/create-current-git-project-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        if should_close_after_switch; then
          return 1
        fi
        return 0
      fi
      return 0
      ;;
    alt-a)
      if "$SCRIPT_DIR/actions/create-adhoc-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        if should_close_after_switch; then
          return 1
        fi
        return 0
      fi
      return 0
      ;;
    alt-h)
      toggle_numbered_sessions
      return 0
      ;;
  esac

  if [ -n "$selection" ]; then
    session_name="$(session_from_selection "$selection")"
  else
    session_name="$(current_session_name)"
  fi
  [ -n "$session_name" ] || return 1

  case "$key" in
    alt-r)
      "$SCRIPT_DIR/actions/rename-session.sh" \
        --client "$client_name" \
        --session "$session_name" || true
      return 0
      ;;
    alt-x)
      "$SCRIPT_DIR/actions/kill-session.sh" \
        --client "$client_name" \
        --session "$session_name" || true
      return 0
      ;;
    *)
      "$SCRIPT_DIR/actions/switch-session.sh" \
        --client "$client_name" \
        --session "$session_name" \
        --sidebar-pane "$sidebar_pane_id" || return 0
      if should_close_after_switch; then
        return 1
      fi
      return 0
      ;;
  esac
}

run_fzf_browser() {
  local output key query selection header_line refresh_dir refresh_seconds tab_delimiter refresh_loop_pid
  local browse_prompt search_prompt mode status original_exit_trap original_exit_handler
  local -a fzf_args

  cleanup_refresh_resources() {
    if [ -n "${refresh_loop_pid:-}" ]; then
      kill "$refresh_loop_pid" 2>/dev/null || true
      wait "$refresh_loop_pid" 2>/dev/null || true
      refresh_loop_pid=''
    fi
    persist_sidebar_refresh_socket ''
    if [ -n "$refresh_dir" ]; then
      rm -rf "$refresh_dir"
      refresh_dir=''
    fi
  }

  capture_existing_exit_trap() {
    local prefix suffix
    original_exit_trap="$(trap -p EXIT || true)"
    original_exit_handler=''
    [ -n "$original_exit_trap" ] || return 0
    prefix=$'trap -- \''
    suffix=$'\' EXIT'
    original_exit_handler="${original_exit_trap#$prefix}"
    original_exit_handler="${original_exit_handler%$suffix}"
  }

  run_fzf_browser_exit_trap() {
    cleanup_refresh_resources
    if [ -n "$original_exit_handler" ]; then
      eval "$original_exit_handler"
    fi
  }

  restore_fzf_browser_exit_trap() {
    trap - INT TERM
    if [ -n "$original_exit_trap" ]; then
      eval "$original_exit_trap"
    else
      trap - EXIT
    fi
  }

  browse_prompt='session> '
  search_prompt='filter> '
  header_line="j/k: move  /: filter  Enter: switch/apply  Esc: close filter/close sidebar  Alt+n: project  Alt+g: git repo  Alt+a: adhoc  Alt+r: rename  Alt+x: kill  Alt+h: numbers ($(numbered_sessions_status_label))"
  tab_delimiter=$'\t'
  mode='browse'

  while :; do
    persist_sidebar_refresh_state
    refresh_dir=''
    refresh_loop_pid=''
    output=''
    status=0
    capture_existing_exit_trap
    trap run_fzf_browser_exit_trap EXIT
    trap cleanup_refresh_resources INT TERM

    if [ "$mode" = 'browse' ]; then
      fzf_args=(
        "--ansi"
        "--disabled"
        "--no-input"
        "--delimiter=$tab_delimiter"
        "--with-nth=2"
        "--expect=alt-n,alt-g,alt-a,alt-r,alt-x,alt-h,/"
        "--header=$header_line"
        "--prompt=$browse_prompt"
        "--height=100%"
        "--bind" "j:down"
        "--bind" "k:up"
      )

      if should_use_fzf_live_reload; then
        refresh_dir="$("$MKTEMP_BIN" -d "${TMPDIR:-/tmp}/tmux-session-sidebar-fzf.XXXXXX")" || refresh_dir=''
        if [ -n "$refresh_dir" ]; then
          refresh_socket="$refresh_dir/fzf.sock"
          persist_sidebar_refresh_socket "$refresh_socket"
          fzf_args+=(
            "--track"
            "--id-nth=1"
            "--listen" "$refresh_socket"
          )
          if should_use_fzf_refresh_loop; then
            refresh_seconds="$(sidebar_heat_refresh_seconds)"
            refresh_loop_pid="$(start_fzf_refresh_loop "$refresh_seconds")"
          fi
        fi
      fi

      output="$({
        filtered_session_entries
      } | "$FZF_BIN" "${fzf_args[@]}")" || status=$?
    else
      persist_sidebar_refresh_socket ''
      fzf_args=(
        "--ansi"
        "--delimiter=$tab_delimiter"
        "--with-nth=2"
        "--print-query"
        "--expect=esc,alt-n,alt-g,alt-a,alt-r,alt-x,alt-h"
        "--header=$header_line"
        "--prompt=$search_prompt"
        "--query=$active_filter"
        "--height=100%"
        "--bind" "enter:accept-or-print-query"
      )

      output="$({
        render_session_entries
      } | "$FZF_BIN" "${fzf_args[@]}")" || status=$?
    fi

    cleanup_refresh_resources
    restore_fzf_browser_exit_trap

    if [ "$mode" = 'browse' ]; then
      key="$(printf '%s\n' "$output" | "$SED_BIN" -n '1p')"
      selection="$(printf '%s\n' "$output" | "$SED_BIN" -n '2p')"

      if [ "$status" -ne 0 ] && [ -z "$key" ] && [ -z "$selection" ]; then
        return 1
      fi

      case "$key" in
        /)
          mode='search'
          continue
          ;;
        alt-*)
          handle_fzf_action "$key" "$selection"
          return $?
          ;;
        '')
          [ -n "$selection" ] || return 1
          handle_fzf_action '' "$selection"
          return $?
          ;;
        *)
          return 1
          ;;
      esac
    fi

    key="$(printf '%s\n' "$output" | "$SED_BIN" -n '2p')"
    query="$(printf '%s\n' "$output" | "$SED_BIN" -n '1p')"
    selection="$(printf '%s\n' "$output" | "$SED_BIN" -n '3p')"

    if [ "$status" -ne 0 ] && [ -z "$key" ] && [ -z "$query" ] && [ -z "$selection" ]; then
      return 1
    fi

    case "$key" in
      '')
        [ "$status" -eq 0 ] || return 1
        active_filter="$query"
        mode='browse'
        continue
        ;;
      esc)
        active_filter=''
        mode='browse'
        continue
        ;;
      alt-*)
        handle_fzf_action "$key" "$selection"
        return $?
        ;;
      *)
        return 1
        ;;
    esac
  done
}

run_fallback_browser() {
  local lines line choice session_name index
  lines="$(render_session_entries)"

  if [ -n "$source_path" ]; then
    printf 'tmux session sidebar (%s)\n' "$source_path" >&2
  else
    printf 'tmux session sidebar\n' >&2
  fi
  printf 'numbered sessions: %s\n\n' "$(numbered_sessions_status_label)" >&2
  index=0
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    index=$((index + 1))
    printf '%3d) %s\n' "$index" "$(display_from_selection "$line")" >&2
  done <<EOF
$lines
EOF

  if [ "$index" -eq 0 ]; then
    printf '  (no visible sessions)\n' >&2
  fi

  printf '\n[number]=switch [n]=project [g]=git repo [a]=adhoc [r]=rename [x]=kill [h]=numbers [q]=close: ' >&2
  if ! read -r choice; then
    return 1
  fi
  case "$choice" in
    q|Q|"")
      return 1
      ;;
    h|H)
      toggle_numbered_sessions
      return 0
      ;;
    n|N)
      if "$SCRIPT_DIR/actions/create-project-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        if should_close_after_switch; then
          return 1
        fi
        return 0
      fi
      return 0
      ;;
    g|G)
      if "$SCRIPT_DIR/actions/create-current-git-project-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        if should_close_after_switch; then
          return 1
        fi
        return 0
      fi
      return 0
      ;;
    a|A)
      if "$SCRIPT_DIR/actions/create-adhoc-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        if should_close_after_switch; then
          return 1
        fi
        return 0
      fi
      return 0
      ;;
    r|R)
      session_name="$(prompt_session_target "$lines" 'Rename session number')" || return 0
      "$SCRIPT_DIR/actions/rename-session.sh" \
        --client "$client_name" \
        --session "$session_name" || true
      return 0
      ;;
    x|X)
      session_name="$(prompt_session_target "$lines" 'Kill session number')" || return 0
      "$SCRIPT_DIR/actions/kill-session.sh" \
        --client "$client_name" \
        --session "$session_name" || true
      return 0
      ;;
  esac

  if ! [[ "$choice" =~ ^[0-9]+$ ]]; then
    echo 'tmux-session-sidebar: invalid selection' >&2
    return 0
  fi

  session_name="$(session_from_index "$lines" "$choice")" || {
    echo 'tmux-session-sidebar: invalid selection' >&2
    return 0
  }

  "$SCRIPT_DIR/actions/switch-session.sh" \
    --client "$client_name" \
    --session "$session_name" \
    --sidebar-pane "$sidebar_pane_id" || return 0

  if should_close_after_switch; then
    return 1
  fi
  return 0
}

main() {
  local use_fzf

  if [ "$render_entries_only" = "on" ]; then
    render_session_entries
    exit 0
  fi

  if [ "$refresh_loop_mode" = "on" ]; then
    run_fzf_refresh_loop
    exit 0
  fi

  while :; do
    use_fzf="$(sidebar_get_option @session-sidebar-use-fzf on)"

    if [ "$use_fzf" != "off" ] && [ -n "$FZF_BIN" ]; then
      run_fzf_browser || exit 0
      continue
    fi

    run_fallback_browser || exit 0
  done
}

main
