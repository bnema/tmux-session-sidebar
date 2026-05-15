#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
GREP_BIN="$(command -v grep 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
[ -n "$GREP_BIN" ] || { echo 'tmux-session-sidebar: grep not found' >&2; exit 1; }
SCRIPT_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")" && "$PWD_BIN")" || exit 1
# shellcheck source=/dev/null
source "$SCRIPT_DIR/../lib/tmux.sh"

window_id=""
pane_id=""

while [ $# -gt 0 ]; do
  case "$1" in
    --window)
      require_arg "$1" "${2:-}"
      window_id="$2"
      shift 2
      ;;
    --pane)
      require_arg "$1" "${2:-}"
      pane_id="$2"
      shift 2
      ;;
    *)
      echo "tmux-session-sidebar: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

[ -n "$window_id" ] || {
  echo 'tmux-session-sidebar: missing window id' >&2
  exit 1
}
[ -n "$pane_id" ] || {
  echo 'tmux-session-sidebar: missing pane id' >&2
  exit 1
}

pane_still_exists() {
  "$TMUX_BIN" list-panes -t "$window_id" -F '#{pane_id}' 2>/dev/null | "$GREP_BIN" -Fxq "$pane_id"
}

for _ in 1 2 3 4 5 6 7 8 9 10 \
         11 12 13 14 15 16 17 18 19 20 \
         21 22 23 24 25 26 27 28 29 30 \
         31 32 33 34 35 36 37 38 39 40 \
         41 42 43 44 45 46 47 48 49 50; do
  if ! pane_still_exists; then
    break
  fi
  sleep 0.05
done

if pane_still_exists; then
  exit 0
fi

sidebar_window_restore_layout "$window_id" || true
