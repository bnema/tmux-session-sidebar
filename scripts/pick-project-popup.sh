#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
SCRIPT_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")" && "$PWD_BIN")" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/lib/projects.sh"

output_file=""
fzf_cmd="$(command -v fzf 2>/dev/null || true)"

while [ $# -gt 0 ]; do
  case "$1" in
    --output-file)
      require_arg "$1" "${2:-}"
      output_file="$2"
      shift 2
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

[ -n "$output_file" ] || { echo 'tmux-session-sidebar: missing popup output file' >&2; exit 1; }
[ -n "$fzf_cmd" ] || { echo 'tmux-session-sidebar: fzf not found for project popup' >&2; exit 1; }

selected_project="$(sidebar_pick_project_fzf "$fzf_cmd")" || exit 1
printf '%s' "$selected_project" > "$output_file"
