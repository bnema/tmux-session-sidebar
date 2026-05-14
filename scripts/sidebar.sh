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

client_name=""
source_path=""
show_numbered_sessions="off"

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

"$TMUX_BIN" set-option -p -t "$sidebar_pane_id" @session-sidebar-pane 1

sidebar_pane_width() {
  local width
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

render_session_label() {
  local session_name="$1"
  local attached_state="$2"
  local window_count="$3"
  local is_current="$4"
  local pane_width="$5"
  local prefix state_code meta name_width rendered_name

  prefix="  "
  if [ "$is_current" = "current" ]; then
    prefix="* "
  fi

  state_code="D"
  if [ "$attached_state" = "attached" ]; then
    state_code="A"
  fi

  meta=" [$state_code:${window_count}w]"
  name_width=$((pane_width - ${#prefix} - ${#meta} - 1))

  if [ "$name_width" -lt 8 ]; then
    meta=" [$state_code]"
    name_width=$((pane_width - ${#prefix} - ${#meta} - 1))
  fi

  if [ "$name_width" -lt 4 ]; then
    name_width=4
  fi

  rendered_name="$(truncate_label "$session_name" "$name_width")"
  printf '%s%s%s' "$prefix" "$rendered_name" "$meta"
}

toggle_numbered_sessions() {
  if [ "$show_numbered_sessions" = "on" ]; then
    show_numbered_sessions="off"
  else
    show_numbered_sessions="on"
  fi
}

numbered_sessions_status_label() {
  if [ "$show_numbered_sessions" = "on" ]; then
    printf 'shown'
  else
    printf 'hidden'
  fi
}

render_session_entries() {
  local pane_width usable_width label
  pane_width="$(sidebar_pane_width)"
  usable_width=$((pane_width - 4))
  if [ "$usable_width" -lt 12 ]; then
    usable_width=12
  fi

  sidebar_list_visible_sessions "$client_name" "$show_numbered_sessions" | while IFS=$'\t' read -r session_name attached_state window_count is_current; do
    [ -z "$session_name" ] && continue
    label="$(render_session_label "$session_name" "$attached_state" "$window_count" "$is_current" "$usable_width")"
    printf '%s\t%s\n' "$session_name" "$label"
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
  close_after_switch="$(sidebar_get_option @session-sidebar-close-after-switch on)"
  [ "$close_after_switch" = "on" ]
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
  local output key selection session_name header_line

  header_line="Enter: switch  Alt+n: project  Alt+a: adhoc  Alt+r: rename  Alt+x: kill  Alt+h: numbers ($(numbered_sessions_status_label))  Esc: close"
  output="$({
    render_session_entries
  } | "$FZF_BIN" \
    --delimiter=$'\t' \
    --with-nth=2 \
    --expect=esc,alt-n,alt-a,alt-r,alt-x,alt-h \
    --header="$header_line" \
    --prompt='session> ' \
    --height=100%)" || return 1

  key="$(printf '%s\n' "$output" | "$SED_BIN" -n '1p')"
  selection="$(printf '%s\n' "$output" | "$SED_BIN" -n '2p')"

  case "$key" in
    esc)
      return 1
      ;;
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
      if should_close_after_switch; then
        return 1
      fi
      return 0
      ;;
  esac
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

  printf '\n[number]=switch [n]=project [a]=adhoc [r]=rename [x]=kill [h]=numbers [q]=close: ' >&2
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
