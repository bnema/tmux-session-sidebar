#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(cd -- "${BASH_SOURCE[0]%/*}" && pwd -P)/_bootstrap.sh"

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
