#!/usr/bin/env bash
# Strict mode is intentionally not enabled here because this file is sourced by
# other scripts and must not silently change the caller's shell options.
# tmux-session-sidebar — shared project helpers
# Sourceable with no side effects. Sources tmux.sh relative to this file.

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
if [ -z "$DIRNAME_BIN" ] || [ -z "$PWD_BIN" ]; then
  echo "tmux-session-sidebar: unable to resolve dirname/pwd" >&2
  if (return 0 2>/dev/null); then
    return 1
  else
    exit 1
  fi
fi

# Source tmux helpers from the same directory
_sidebar_lib_dir="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")" && "$PWD_BIN")" || {
  echo "tmux-session-sidebar: unable to locate script directory" >&2
  if (return 0 2>/dev/null); then
    return 1
  else
    exit 1
  fi
}
if [ ! -f "$_sidebar_lib_dir/tmux.sh" ]; then
  echo "tmux-session-sidebar: missing tmux helper library: $_sidebar_lib_dir/tmux.sh" >&2
  if (return 0 2>/dev/null); then
    return 1
  else
    exit 1
  fi
fi
# shellcheck source=/dev/null
source "$_sidebar_lib_dir/tmux.sh"
unset _sidebar_lib_dir
unset DIRNAME_BIN
unset PWD_BIN

BASENAME_BIN="$(sidebar_require_command basename)" || _sidebar_return_or_exit 1
TR_BIN="$(sidebar_require_command tr)" || _sidebar_return_or_exit 1
SED_BIN="$(sidebar_require_command sed)" || _sidebar_return_or_exit 1
FIND_BIN="$(sidebar_require_command find)" || _sidebar_return_or_exit 1
SORT_BIN="$(sidebar_require_command sort)" || _sidebar_return_or_exit 1
MKTEMP_BIN="$(sidebar_require_command mktemp)" || _sidebar_return_or_exit 1
GIT_BIN="$(command -v git 2>/dev/null || true)"
REALPATH_BIN="$(command -v realpath 2>/dev/null || true)"
READLINK_BIN="$(command -v readlink 2>/dev/null || true)"

sidebar_resolve_directory() {
  local path="$1"
  if [ -n "$REALPATH_BIN" ]; then
    "$REALPATH_BIN" "$path" 2>/dev/null && return 0
  fi
  if [ -n "$READLINK_BIN" ]; then
    "$READLINK_BIN" -f "$path" 2>/dev/null && return 0
  fi
  printf '%s\n' "$path"
}

sidebar_project_roots() {
  # Usage: sidebar_project_roots
  # Prints existing project root directories, one per line.
  local roots raw_roots root resolved_root
  raw_roots="$(sidebar_get_option @session-sidebar-project-roots "$HOME/projects")"
  readarray -d ':' -t roots <<< "$raw_roots:"
  for root in "${roots[@]}"; do
    [ -z "$root" ] && continue
    if [ -d "$root" ]; then
      resolved_root="$(sidebar_resolve_directory "$root")"
      if [ -d "$resolved_root" ]; then
        printf '%s\n' "$resolved_root"
      fi
    fi
  done
}

sidebar_list_projects() {
  # Usage: sidebar_list_projects
  # Prints one absolute path per line for immediate child directories under
  # each project root, sorted and deduplicated.
  local root
  while IFS= read -r root; do
    [ -z "$root" ] && continue
    if [ -d "$root" ]; then
      "$FIND_BIN" "$root" -mindepth 1 -maxdepth 1 -type d 2>/dev/null
    fi
  done < <(sidebar_project_roots) | "$SORT_BIN" -u
}

sidebar_derive_session_name() {
  # Usage: sidebar_derive_session_name PATH
  # Produces a stable tmux-safe session name from a filesystem path.
  local path="$1" name
  if [ -z "$path" ]; then
    echo "session"
    return 0
  fi

  name="$("$BASENAME_BIN" "$path")"

  # Normalize to lowercase
  name="$(printf '%s' "$name" | "$TR_BIN" '[:upper:]' '[:lower:]')"

  # Normalize, collapse, and trim in one pass.
  name="$(printf '%s' "$name" | "$SED_BIN" -E 's/[^a-z0-9_-]/_/g; s/_{2,}/_/g; s/-{2,}/-/g; s/^[._-]+//; s/[._-]+$//')"

  # Fallback if name is empty after normalization
  if [ -z "$name" ]; then
    name="session"
  fi

  printf '%s\n' "$name"
}

sidebar_git_project_root() {
  # Usage: sidebar_git_project_root PATH
  # Prints the enclosing git repository root for PATH, if any.
  local path="$1"
  local git_root=""

  [ -n "$path" ] || return 1
  [ -n "$GIT_BIN" ] || return 1

  git_root="$("$GIT_BIN" -C "$path" rev-parse --show-toplevel 2>/dev/null || true)"
  [ -n "$git_root" ] || return 1

  sidebar_resolve_directory "$git_root"
}

sidebar_shorten_home_path() {
  local path="$1"
  case "$path" in
    "$HOME")
      printf '~'
      ;;
    "$HOME"/*)
      printf '%s/%s' '~' "${path#"$HOME"/}"
      ;;
    *)
      printf '%s' "$path"
      ;;
  esac
}

sidebar_render_project_entry() {
  local project_path="$1"
  local project_name parent_dir parent_display

  project_name="$("$BASENAME_BIN" "$project_path")"
  parent_dir="${project_path%/*}"
  if [ "$parent_dir" = "$project_path" ]; then
    parent_dir='.'
  fi
  parent_display="$(sidebar_shorten_home_path "$parent_dir")"

  printf '%s\t%s\t[%s]\n' "$project_path" "$project_name" "$parent_display"
}

sidebar_render_project_entries() {
  local project_path
  while IFS= read -r project_path; do
    [ -z "$project_path" ] && continue
    sidebar_render_project_entry "$project_path"
  done < <(sidebar_list_projects)
}

sidebar_project_path_from_selection() {
  local selection="$1"
  printf '%s' "${selection%%$'\t'*}"
}

sidebar_project_display_from_selection() {
  local selection="$1"
  local display=""

  if [[ "$selection" != *$'\t'* ]]; then
    printf '%s' "$selection"
    return 0
  fi

  display="${selection#*$'\t'}"
  display="${display//$'\t'/  }"
  printf '%s' "$display"
}

sidebar_pick_project_fzf() {
  local fzf_cmd="$1"
  local entries selected

  entries="$(sidebar_render_project_entries)"
  if [ -z "$entries" ]; then
    echo "tmux-session-sidebar: no projects found in configured roots" >&2
    return 1
  fi

  selected="$(printf '%s\n' "$entries" | "$fzf_cmd" \
    --delimiter=$'\t' \
    --with-nth=2,3 \
    --prompt='project> ' \
    --header='Enter: create session  Esc: cancel' \
    --height=100%)" || return 1

  sidebar_project_path_from_selection "$selected"
}

sidebar_project_picker_popup_script() {
  local popup_script
  popup_script="$(cd "${BASH_SOURCE[0]%/*}/.." && pwd)/pick-project-popup.sh" || return 1
  printf '%s' "$popup_script"
}

sidebar_tmux_supports_popups() {
  "$TMUX_BIN" list-commands display-popup >/dev/null 2>&1
}

sidebar_pick_project_popup() {
  local client_name="$1"
  local start_dir="${2:-}"
  local output_file popup_script selected

  [ -n "$client_name" ] || return 1

  popup_script="$(sidebar_project_picker_popup_script)" || return 1
  [ -x "$popup_script" ] || return 1

  output_file="$("$MKTEMP_BIN" "${TMPDIR:-/tmp}/tmux-session-sidebar-project.XXXXXX")" || return 1

  if [ -z "$start_dir" ] || [ ! -d "$start_dir" ]; then
    start_dir="$HOME"
  fi

  if ! "$TMUX_BIN" display-popup \
    -c "$client_name" \
    -d "$start_dir" \
    -E \
    -w '80%' \
    -h '80%' \
    -T 'Project session' \
    "$popup_script" --output-file "$output_file"; then
    rm -f "$output_file"
    return 1
  fi

  selected=''
  if [ -r "$output_file" ]; then
    selected="$(<"$output_file")"
  fi
  rm -f "$output_file"

  [ -n "$selected" ] || return 1
  printf '%s' "$selected"
}

sidebar_pick_project() {
  # Usage: sidebar_pick_project [CLIENT] [START_DIR]
  # Presents the project list to the user and writes the selected path to stdout.
  # Uses fzf when available and @session-sidebar-use-fzf is not off; when a tmux
  # client is available, the fzf project picker runs in a popup for readability.
  # Otherwise it falls back to inline fzf or a simple numbered menu.
  local client_name="${1:-}"
  local start_dir="${2:-}"
  local use_fzf projects choice line fzf_cmd popup_script
  local -a lines=()
  local i=0

  use_fzf="$(sidebar_get_option @session-sidebar-use-fzf on)"
  fzf_cmd="$(command -v fzf 2>/dev/null || true)"

  if [ "$use_fzf" != "off" ] && [ -n "$fzf_cmd" ]; then
    if [ -n "$client_name" ] && sidebar_tmux_supports_popups && popup_script="$(sidebar_project_picker_popup_script 2>/dev/null || true)" && [ -x "$popup_script" ]; then
      sidebar_pick_project_popup "$client_name" "$start_dir" || return 1
      return 0
    fi

    sidebar_pick_project_fzf "$fzf_cmd"
    return 0
  fi

  projects="$(sidebar_list_projects)"
  if [ -z "$projects" ]; then
    echo "tmux-session-sidebar: no projects found in configured roots" >&2
    return 1
  fi

  while IFS= read -r line; do
    [ -z "$line" ] && continue
    i=$((i + 1))
    line="$(sidebar_render_project_entry "$line")"
    lines+=("$line")
    printf '%3d) %s\n' "$i" "$(sidebar_project_display_from_selection "$line")" >&2
  done <<< "$projects"

  [ "$i" -gt 0 ] || return 1

  printf '\nSelect project number [1-%d]: ' "$i" >&2
  if ! read -r choice; then
    echo 'tmux-session-sidebar: project selection cancelled' >&2
    return 1
  fi
  if [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -ge 1 ] && [ "$choice" -le "$i" ]; then
    sidebar_project_path_from_selection "${lines[$((choice - 1))]}"
    return 0
  fi

  echo "tmux-session-sidebar: invalid project selection" >&2
  return 1
}
