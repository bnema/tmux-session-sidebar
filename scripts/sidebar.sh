#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/lib/tmux.sh"

client_name=""
source_path=""

require_arg() {
  local flag="$1"
  local value="${2:-}"
  if [ -z "$value" ]; then
    echo "tmux-session-sidebar: missing value for $flag" >&2
    exit 1
  fi
}

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
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

client_name="$(sidebar_current_client "$client_name")" || exit 1
sidebar_pane_id="$(sidebar_current_pane_id)" || exit 1

tmux set-option -p -t "$sidebar_pane_id" @session-sidebar-pane 1

render_session_lines() {
  sidebar_list_sessions "$client_name" | while IFS=$'\t' read -r session_name attached_state window_count is_current; do
    [ -z "$session_name" ] && continue
    marker=" "
    if [ "$is_current" = "current" ]; then
      marker="*"
    fi
    printf '%s\t%s\t%s\t%s\n' "$session_name" "$marker" "$attached_state" "$window_count"
  done
}

run_fzf_browser() {
  local output key selection session_name close_after_switch

  output="$({
    render_session_lines
  } | fzf \
    --delimiter=$'\t' \
    --with-nth=1,2,3,4 \
    --expect=esc \
    --header='Enter: switch  Esc: close' \
    --prompt='session> ' \
    --height=40%)" || return 1

  key="$(printf '%s\n' "$output" | sed -n '1p')"
  selection="$(printf '%s\n' "$output" | sed -n '2p')"

  case "$key" in
    esc)
      return 1
      ;;
    "")
      ;;
    *)
      if [ -z "$selection" ]; then
        selection="$key"
        key=""
      fi
      ;;
  esac

  [ -n "$selection" ] || return 1

  session_name="${selection%%$'\t'*}"
  [ -n "$session_name" ] || return 1

  "$SCRIPT_DIR/actions/switch-session.sh" \
    --client "$client_name" \
    --session "$session_name" \
    --sidebar-pane "$sidebar_pane_id"

  close_after_switch="$(sidebar_get_option @session-sidebar-close-after-switch on)"
  [ "$close_after_switch" != "on" ]
}

run_fallback_browser() {
  local lines line choice selected_line session_name index close_after_switch
  lines="$(render_session_lines)"
  [ -n "$lines" ] || return 1

  printf 'tmux session sidebar (%s)\n\n' "$source_path" >&2
  index=0
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    index=$((index + 1))
    printf '%3d) %s\n' "$index" "$line" >&2
  done <<EOF
$lines
EOF

  printf '\nSelect session number or q to close: ' >&2
  read -r choice
  case "$choice" in
    q|Q|"")
      return 1
      ;;
  esac

  if ! [[ "$choice" =~ ^[0-9]+$ ]]; then
    echo 'tmux-session-sidebar: invalid selection' >&2
    return 1
  fi

  selected_line="$(printf '%s\n' "$lines" | sed -n "${choice}p")"
  [ -n "$selected_line" ] || {
    echo 'tmux-session-sidebar: invalid selection' >&2
    return 1
  }

  session_name="${selected_line%%$'\t'*}"
  "$SCRIPT_DIR/actions/switch-session.sh" \
    --client "$client_name" \
    --session "$session_name" \
    --sidebar-pane "$sidebar_pane_id"

  close_after_switch="$(sidebar_get_option @session-sidebar-close-after-switch on)"
  [ "$close_after_switch" != "on" ]
}

main() {
  local use_fzf

  while :; do
    use_fzf="$(sidebar_get_option @session-sidebar-use-fzf on)"

    if [ "$use_fzf" != "off" ] && command -v fzf >/dev/null 2>&1; then
      run_fzf_browser || exit 0
      continue
    fi

    run_fallback_browser || exit 0
  done
}

main
