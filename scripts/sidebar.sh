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

session_from_selection() {
  local selection="$1"
  printf '%s' "${selection%%$'\t'*}"
}

session_from_index() {
  local lines="$1"
  local choice="$2"
  local selected_line

  selected_line="$(printf '%s\n' "$lines" | sed -n "${choice}p")"
  [ -n "$selected_line" ] || return 1
  session_from_selection "$selected_line"
}

current_session_name() {
  sidebar_current_session "$client_name"
}

post_switch_should_continue() {
  local close_after_switch
  close_after_switch="$(sidebar_get_option @session-sidebar-close-after-switch on)"
  [ "$close_after_switch" != "on" ]
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

run_fzf_browser() {
  local output key selection session_name

  output="$({
    render_session_lines
  } | fzf \
    --delimiter=$'\t' \
    --with-nth=1,2,3,4 \
    --expect=esc,alt-n,alt-a,alt-r,alt-x \
    --header='Enter: switch  Alt+n: project  Alt+a: adhoc  Alt+r: rename  Alt+x: kill  Esc: close' \
    --prompt='session> ' \
    --height=40%)" || return 1

  key="$(printf '%s\n' "$output" | sed -n '1p')"
  selection="$(printf '%s\n' "$output" | sed -n '2p')"

  case "$key" in
    esc)
      return 1
      ;;
    alt-n)
      if "$SCRIPT_DIR/actions/create-project-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        post_switch_should_continue
        return
      fi
      return 0
      ;;
    alt-a)
      if "$SCRIPT_DIR/actions/create-adhoc-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        post_switch_should_continue
        return
      fi
      return 0
      ;;
    alt-r|alt-x)
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
      post_switch_should_continue
      ;;
  esac
}

run_fallback_browser() {
  local lines line choice session_name index
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

  printf '\n[number]=switch [n]=project [a]=adhoc [r]=rename [x]=kill [q]=close: ' >&2
  read -r choice
  case "$choice" in
    q|Q|"")
      return 1
      ;;
    n|N)
      if "$SCRIPT_DIR/actions/create-project-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        post_switch_should_continue
        return
      fi
      return 0
      ;;
    a|A)
      if "$SCRIPT_DIR/actions/create-adhoc-session.sh" \
        --client "$client_name" \
        --sidebar-pane "$sidebar_pane_id" \
        --source-path "$source_path"; then
        post_switch_should_continue
        return
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

  post_switch_should_continue
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
