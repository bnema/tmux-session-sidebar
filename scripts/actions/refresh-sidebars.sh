#!/usr/bin/env bash
set -euo pipefail

CURL_BIN="$(command -v curl 2>/dev/null || true)"
[ -n "$CURL_BIN" ] || exit 0
# shellcheck source=/dev/null
source "$(cd -- "${BASH_SOURCE[0]%/*}" && pwd -P)/_bootstrap.sh"

pane_id=""

while [ $# -gt 0 ]; do
  case "$1" in
    --pane)
      require_arg "$1" "${2:-}"
      pane_id="$2"
      shift 2
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

quote_args() {
  local quoted="" arg
  for arg in "$@"; do
    printf -v quoted '%s%q ' "$quoted" "$arg"
  done
  printf '%s' "$quoted"
}

render_entries_command_for_pane() {
  local target_pane="$1"
  local client_name show_numbered source_path active_filter
  local -a args

  client_name="$(sidebar_get_pane_option "$target_pane" @session-sidebar-client '')"
  [ -n "$client_name" ] || return 1
  show_numbered="$(sidebar_get_pane_option "$target_pane" @session-sidebar-show-numbered-sessions off)"
  source_path="$(sidebar_get_pane_option "$target_pane" @session-sidebar-source-path '')"
  active_filter="$(sidebar_get_pane_option "$target_pane" @session-sidebar-active-filter '')"

  args=("$SCRIPT_DIR/../sidebar.sh" --client "$client_name" --show-numbered-sessions "$show_numbered" --sidebar-pane "$target_pane")
  if [ -n "$active_filter" ]; then
    args+=(--active-filter "$active_filter")
  fi
  if [ -n "$source_path" ]; then
    args+=(--source-path "$source_path")
  fi
  args+=(--render-entries)
  quote_args "${args[@]}"
}

current_session_position_for_pane() {
  local target_pane="$1"
  local client_name show_numbered current_session session_name position

  client_name="$(sidebar_get_pane_option "$target_pane" @session-sidebar-client '')"
  [ -n "$client_name" ] || return 1
  show_numbered="$(sidebar_get_pane_option "$target_pane" @session-sidebar-show-numbered-sessions off)"
  current_session="$(sidebar_current_session "$client_name" 2>/dev/null || true)"
  [ -n "$current_session" ] || return 1

  position=0
  while IFS=$'\t' read -r session_name _ _ _; do
    [ -n "$session_name" ] || continue
    position=$((position + 1))
    if [ "$session_name" = "$current_session" ]; then
      printf '%s' "$position"
      return 0
    fi
  done < <(sidebar_list_visible_sessions "$client_name" "$show_numbered")

  return 1
}

refresh_pane() {
  local target_pane="$1"
  local socket_path payload render_command current_position

  [ -n "$target_pane" ] || return 0
  socket_path="$(sidebar_get_pane_option "$target_pane" @session-sidebar-refresh-socket '')"
  [ -n "$socket_path" ] || return 0

  render_command="$(render_entries_command_for_pane "$target_pane")" || return 0
  payload="reload-sync($render_command)"
  current_position="$(current_session_position_for_pane "$target_pane" 2>/dev/null || true)"
  case "$current_position" in
    ''|*[!0-9]*) ;;
    *) payload="pos($current_position)+$payload" ;;
  esac
  "$CURL_BIN" --silent --show-error --unix-socket "$socket_path" http -d "$payload" >/dev/null 2>&1 || true
}

if [ -n "$pane_id" ]; then
  refresh_pane "$pane_id"
  exit 0
fi

while IFS= read -r sidebar_pane; do
  [ -n "$sidebar_pane" ] || continue
  refresh_pane "$sidebar_pane"
done < <(sidebar_list_sidebar_panes)
