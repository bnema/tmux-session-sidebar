#!/usr/bin/env bash
# Shared setup for action entrypoints. Source this after strict mode is enabled.
# It intentionally keeps SCRIPT_DIR as the actions directory because callers use
# that name to exec sibling action scripts.

SIDEBAR_ACTIONS_DIR="$(cd -- "${BASH_SOURCE[0]%/*}" && pwd -P)" || exit 1
SIDEBAR_SCRIPT_DIR="${SIDEBAR_ACTIONS_DIR%/*}"
SCRIPT_DIR="$SIDEBAR_ACTIONS_DIR"

# shellcheck source=/dev/null
source "$SIDEBAR_SCRIPT_DIR/lib/tmux.sh"
