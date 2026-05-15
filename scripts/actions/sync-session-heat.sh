#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
SCRIPT_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")" && "$PWD_BIN")" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/../lib/tmux.sh"

sync_all="off"
sessions=()

while [ $# -gt 0 ]; do
  case "$1" in
    --session)
      require_arg "$1" "${2:-}"
      sessions+=("$2")
      shift 2
      ;;
    --all)
      sync_all="on"
      shift
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [ "$sync_all" = "on" ]; then
  sidebar_sync_all_session_heat >/dev/null || true
fi

declare -A seen_map=()
for session_name in "${sessions[@]}"; do
  [ -n "$session_name" ] || continue
  [ -n "${seen_map[$session_name]:-}" ] && continue
  seen_map[$session_name]=1
  sidebar_sync_session_heat "$session_name" >/dev/null || true
done
