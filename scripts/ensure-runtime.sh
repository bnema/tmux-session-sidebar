#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
GO_BIN="$(command -v go 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
PLUGIN_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1
BIN_DIR="$PLUGIN_DIR/bin"
BIN_PATH="$BIN_DIR/tmux-session-sidebar"

if [ -x "$BIN_PATH" ]; then
  printf '%s\n' "$BIN_PATH"
  exit 0
fi

[ -n "$GO_BIN" ] || { echo 'tmux-session-sidebar: go not found and runtime binary is missing' >&2; exit 1; }
mkdir -p "$BIN_DIR"
(cd "$PLUGIN_DIR" && "$GO_BIN" build -o "$BIN_PATH" ./cmd/tmux-session-sidebar)
printf '%s\n' "$BIN_PATH"
