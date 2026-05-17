#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(cd -- "${BASH_SOURCE[0]%/*}" && pwd -P)/_bootstrap.sh"
# shellcheck source=/dev/null
source "$SIDEBAR_SCRIPT_DIR/lib/projects.sh"

client_name=""
sidebar_pane_id=""
source_path=""
project_path=""

while [ $# -gt 0 ]; do
  case "$1" in
    --client)
      require_arg "$1" "${2:-}"
      client_name="$2"
      shift 2
      ;;
    --sidebar-pane)
      require_arg "$1" "${2:-}"
      sidebar_pane_id="$2"
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
source_path="$(sidebar_current_path "$source_path")" || exit 1

if ! project_path="$(sidebar_git_project_root "$source_path")"; then
  "$TMUX_BIN" display-message 'tmux-session-sidebar: current path is not inside a git repository'
  exit 1
fi

create_args=(
  --client "$client_name"
  --project-path "$project_path"
  --source-path "$source_path"
)
if [ -n "$sidebar_pane_id" ]; then
  create_args+=(--sidebar-pane "$sidebar_pane_id")
fi

exec "$SCRIPT_DIR/create-project-session.sh" "${create_args[@]}"
