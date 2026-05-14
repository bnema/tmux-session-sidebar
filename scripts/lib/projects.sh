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

sidebar_pick_project() {
  # Usage: sidebar_pick_project
  # Presents the project list to the user and writes the selected path to stdout.
  # Uses fzf when available and @session-sidebar-use-fzf is not off;
  # otherwise falls back to a simple numbered menu on stderr, selection on stdout.
  local use_fzf projects selected choice line fzf_cmd
  local -a lines=()
  local i=0

  use_fzf="$(sidebar_get_option @session-sidebar-use-fzf on)"
  fzf_cmd="$(command -v fzf 2>/dev/null || true)"

  projects="$(sidebar_list_projects)"
  if [ -z "$projects" ]; then
    echo "tmux-session-sidebar: no projects found in configured roots" >&2
    return 1
  fi

  if [ "$use_fzf" != "off" ] && [ -n "$fzf_cmd" ]; then
    selected="$(printf '%s\n' "$projects" | "$fzf_cmd" --prompt='project> ' --height=40%)" || return 1
    printf '%s' "$selected"
    return 0
  fi

  while IFS= read -r line; do
    [ -z "$line" ] && continue
    i=$((i + 1))
    lines+=("$line")
    printf '%3d) %s\n' "$i" "$line" >&2
  done <<< "$projects"

  [ "$i" -gt 0 ] || return 1

  printf '\nSelect project number [1-%d]: ' "$i" >&2
  if ! read -r choice; then
    echo 'tmux-session-sidebar: project selection cancelled' >&2
    return 1
  fi
  if [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -ge 1 ] && [ "$choice" -le "$i" ]; then
    printf '%s' "${lines[$((choice - 1))]}"
    return 0
  fi

  echo "tmux-session-sidebar: invalid project selection" >&2
  return 1
}
