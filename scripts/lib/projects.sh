#!/usr/bin/env bash
# tmux-session-sidebar — shared project helpers
# Sourceable with no side effects. Sources tmux.sh relative to this file.

# Source tmux helpers from the same directory
SIDEBAR_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" || exit 1
if [ -f "$SIDEBAR_LIB_DIR/tmux.sh" ]; then
  # shellcheck source=/dev/null
  source "$SIDEBAR_LIB_DIR/tmux.sh"
fi

sidebar_project_roots() {
  # Usage: sidebar_project_roots
  # Prints existing project root directories, one per line.
  local roots raw_roots root
  raw_roots="$(sidebar_get_option @session-sidebar-project-roots "$HOME/projects")"
  IFS=':' read -r -a roots <<< "$raw_roots"
  for root in "${roots[@]}"; do
    [ -z "$root" ] && continue
    if [ -d "$root" ]; then
      printf '%s\n' "$root"
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
      find -L "$root" -mindepth 1 -maxdepth 1 -type d 2>/dev/null
    fi
  done < <(sidebar_project_roots) | sort -u
}

sidebar_derive_session_name() {
  # Usage: sidebar_derive_session_name PATH
  # Produces a stable tmux-safe session name from a filesystem path.
  local path="$1" name
  if [ -z "$path" ]; then
    echo "session"
    return 0
  fi

  name="$(basename "$path")"

  # Normalize to lowercase
  name="$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')"

  # Replace anything not alphanumeric or dash with underscore
  name="$(printf '%s' "$name" | sed 's/[^A-Za-z0-9-]/_/g')"

  # Collapse repeated underscores and dashes
  name="$(printf '%s' "$name" | sed -E 's/_{2,}/_/g; s/-{2,}/-/g')"

  # Remove leading punctuation (dots, dashes, underscores)
  name="$(printf '%s' "$name" | sed -E 's/^[._-]+//')"

  # Remove trailing punctuation
  name="$(printf '%s' "$name" | sed -E 's/[._-]+$//')"

  # Fallback if name is empty after normalization
  if [ -z "$name" ]; then
    name="session"
  fi

  printf '%s\n' "$name"
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

sidebar_pick_project() {
  # Usage: sidebar_pick_project
  # Presents the project list to the user and writes the selected path to stdout.
  # Uses fzf when available and @session-sidebar-use-fzf is not off;
  # otherwise falls back to a simple numbered menu on stderr, selection on stdout.
  local use_fzf fzf_available projects selected choice line
  local -a lines=()
  local i=0

  use_fzf="$(sidebar_get_option @session-sidebar-use-fzf on)"
  fzf_available=false
  if command -v fzf >/dev/null 2>&1; then
    fzf_available=true
  fi

  projects="$(sidebar_list_projects)"
  if [ -z "$projects" ]; then
    echo "tmux-session-sidebar: no projects found in configured roots" >&2
    return 1
  fi

  if [ "$use_fzf" != "off" ] && $fzf_available; then
    selected="$(printf '%s\n' "$projects" | fzf --prompt='project> ' --height=40%)" || return 1
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
  read -r choice
  if [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -ge 1 ] && [ "$choice" -le "$i" ]; then
    printf '%s' "${lines[$((choice - 1))]}"
    return 0
  fi

  echo "tmux-session-sidebar: invalid project selection" >&2
  return 1
}
