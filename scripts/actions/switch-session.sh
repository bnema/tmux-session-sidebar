#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(cd -- "${BASH_SOURCE[0]%/*}" && pwd -P)/_bootstrap.sh"

client_name=""
session_name=""
sidebar_pane_id=""

while [ $# -gt 0 ]; do
  case "$1" in
    --client)
      require_arg "$1" "${2:-}"
      client_name="$2"
      shift 2
      ;;
    --session)
      require_arg "$1" "${2:-}"
      session_name="$2"
      shift 2
      ;;
    --sidebar-pane)
      require_arg "$1" "${2:-}"
      sidebar_pane_id="$2"
      shift 2
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

client_name="$(sidebar_current_client "$client_name")" || exit 1
[ -n "$session_name" ] || {
  echo 'tmux-session-sidebar: missing session name' >&2
  exit 1
}

if ! sidebar_validate_session_name "$session_name"; then
  exit 1
fi

if ! sidebar_session_exists "$session_name"; then
  "$TMUX_BIN" display-message "tmux-session-sidebar: session '$session_name' does not exist"
  exit 1
fi

close_after_switch="$(sidebar_get_option @session-sidebar-close-after-switch off)"
session_target="$(sidebar_session_target "$session_name")" || exit 1
previous_session_name="$(sidebar_current_session "$client_name" 2>/dev/null || true)"
previous_sidebar_window_id=""
sidebar_source_path=""
sidebar_show_numbered="off"
sidebar_active_filter=""
if [ -n "$sidebar_pane_id" ]; then
  previous_sidebar_window_id="$(sidebar_pane_window_id "$sidebar_pane_id" 2>/dev/null || true)"
  sidebar_source_path="$(sidebar_get_pane_option "$sidebar_pane_id" @session-sidebar-source-path '')"
  sidebar_show_numbered="$(sidebar_get_pane_option "$sidebar_pane_id" @session-sidebar-show-numbered-sessions off)"
  sidebar_active_filter="$(sidebar_get_pane_option "$sidebar_pane_id" @session-sidebar-active-filter '')"
fi

"$TMUX_BIN" switch-client -c "$client_name" -t "$session_target"

if [ -n "$previous_session_name" ] && [ "$previous_session_name" != "$session_name" ]; then
  sidebar_sync_session_heat "$previous_session_name" >/dev/null || true
fi
sidebar_sync_session_heat "$session_name" >/dev/null || true
if [ -n "$sidebar_pane_id" ]; then
  if [ "$close_after_switch" = "on" ]; then
    sidebar_window_id="$(sidebar_pane_window_id "$sidebar_pane_id" 2>/dev/null || true)"
    "$TMUX_BIN" kill-pane -t "$sidebar_pane_id" 2>/dev/null || true
    if [ -n "$sidebar_window_id" ]; then
      sidebar_window_restore_layout "$sidebar_window_id" || true
    fi
  else
    target_window_id="$("$TMUX_BIN" display-message -p -t "$client_name" '#{window_id}' 2>/dev/null || true)"
    current_sidebar_window_id="$(sidebar_pane_window_id "$sidebar_pane_id" 2>/dev/null || true)"
    if [ -n "$target_window_id" ] && [ -n "$current_sidebar_window_id" ] && [ "$current_sidebar_window_id" != "$target_window_id" ]; then
      existing_target_sidebar="$(sidebar_existing_sidebar_pane "$target_window_id" 2>/dev/null || true)"
      if [ -n "$existing_target_sidebar" ] && [ "$existing_target_sidebar" != "$sidebar_pane_id" ]; then
        "$TMUX_BIN" kill-pane -t "$existing_target_sidebar" 2>/dev/null || true
        sidebar_window_restore_layout "$target_window_id" || true
      fi

      sidebar_window_save_layout "$target_window_id" || true
      width="$(sidebar_get_option @session-sidebar-width 20)"
      if [ -z "$sidebar_source_path" ]; then
        sidebar_source_path="$("$TMUX_BIN" display-message -p -t "$client_name" '#{pane_current_path}' 2>/dev/null || true)"
      fi
      if [ -n "$sidebar_source_path" ] && [ ! -d "$sidebar_source_path" ]; then
        sidebar_source_path=""
      fi
      quoted_script=""
      printf -v quoted_script '%q' "$SIDEBAR_SCRIPT_DIR/sidebar.sh"
      printf -v sidebar_cmd '%s --client %q --show-numbered-sessions %q' \
        "$quoted_script" \
        "$client_name" \
        "$sidebar_show_numbered"
      if [ -n "$sidebar_source_path" ]; then
        printf -v sidebar_cmd '%s --source-path %q' "$sidebar_cmd" "$sidebar_source_path"
      fi
      if [ -n "$sidebar_active_filter" ]; then
        printf -v sidebar_cmd '%s --active-filter %q' "$sidebar_cmd" "$sidebar_active_filter"
      fi
      split_args=(-P -F '#{pane_id}' -t "$target_window_id" -hbf -l "$width")
      if [ -n "$sidebar_source_path" ]; then
        split_args+=(-c "$sidebar_source_path")
      fi
      split_args+=("$sidebar_cmd")
      new_sidebar_pane_id="$("$TMUX_BIN" split-window "${split_args[@]}" 2>/dev/null || true)"
      if [ -n "$new_sidebar_pane_id" ]; then
        sidebar_set_pane_option "$new_sidebar_pane_id" @session-sidebar-pane 1 >/dev/null 2>&1 || true
        sidebar_set_pane_option "$new_sidebar_pane_id" @session-sidebar-client "$client_name" >/dev/null 2>&1 || true
        sidebar_set_pane_option "$new_sidebar_pane_id" @session-sidebar-show-numbered-sessions "$sidebar_show_numbered" >/dev/null 2>&1 || true
        if [ -n "$sidebar_source_path" ]; then
          sidebar_set_pane_option "$new_sidebar_pane_id" @session-sidebar-source-path "$sidebar_source_path" >/dev/null 2>&1 || true
        fi
        if [ -n "$sidebar_active_filter" ]; then
          sidebar_set_pane_option "$new_sidebar_pane_id" @session-sidebar-active-filter "$sidebar_active_filter" >/dev/null 2>&1 || true
        fi
        "$TMUX_BIN" kill-pane -t "$sidebar_pane_id" 2>/dev/null || true
        if [ -n "$previous_sidebar_window_id" ] && [ "$previous_sidebar_window_id" != "$target_window_id" ]; then
          sidebar_window_restore_layout "$previous_sidebar_window_id" || true
        fi
      fi
    fi
  fi
fi

"$SCRIPT_DIR/refresh-sidebars.sh" >/dev/null 2>&1 || true
