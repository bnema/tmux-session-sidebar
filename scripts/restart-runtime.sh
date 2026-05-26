#!/usr/bin/env bash
set -euo pipefail

TMUX_BIN="$(command -v tmux 2>/dev/null || true)"
PKILL_BIN="$(command -v pkill 2>/dev/null || true)"
TMUX_CONF="${TMUX_CONF:-$HOME/.tmux.conf}"
SIDEBAR_SESSION_NAME="${SIDEBAR_SESSION_NAME:-__tmux-session-sidebar}"

[ -n "$TMUX_BIN" ] || { echo 'tmux-session-sidebar: tmux not found' >&2; exit 1; }
[ -n "$PKILL_BIN" ] || { echo 'tmux-session-sidebar: pkill not found' >&2; exit 1; }

"$PKILL_BIN" -f 'tmux-session-sidebar daemon serve-ui' 2>/dev/null || true
"$PKILL_BIN" -f 'tmux-session-sidebar daemon serve' 2>/dev/null || true
"$TMUX_BIN" kill-session -t "$SIDEBAR_SESSION_NAME" 2>/dev/null || true
"$TMUX_BIN" source-file "$TMUX_CONF"

echo "tmux-session-sidebar runtime restarted via $TMUX_CONF"
