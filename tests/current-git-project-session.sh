#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
GIT_BIN="$(command -v git 2>/dev/null || true)"
TMUX_BIN_REAL="$(command -v tmux 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
[ -n "$GIT_BIN" ] || { echo 'git not found' >&2; exit 1; }
[ -n "$TMUX_BIN_REAL" ] || { echo 'tmux not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
sock="tss_test_current_git_project"
client_log="$work_dir/client.log"
project_root="$work_dir/anywhere/nested/my-repo"
project_subdir="$project_root/deeper/worktree"
mkdir -p "$project_subdir"

cleanup() {
  env -u TMUX "$TMUX_BIN_REAL" -L "$sock" kill-server 2>/dev/null || true
  rm -rf "$work_dir"
}
trap cleanup EXIT

"$GIT_BIN" init "$project_root" >/dev/null 2>&1

env -u TMUX "$TMUX_BIN_REAL" -L "$sock" new-session -d -s alpha 'sleep 9999'
script -q -c "env -u TMUX TERM=xterm-256color tmux -L $sock attach-session -t alpha" "$client_log" >/dev/null 2>&1 &
client_pid=$!
sleep 1

client_name="$(env -u TMUX "$TMUX_BIN_REAL" -L "$sock" list-clients -F '#{client_name}' | head -n1)"
[ -n "$client_name" ] || { echo 'no tmux client found' >&2; exit 1; }

env -u TMUX "$TMUX_BIN_REAL" -L "$sock" run-shell "$(printf '%q ' "$REPO_DIR/scripts/actions/create-current-git-project-session.sh" --client "$client_name" --source-path "$project_subdir")"
sleep 1

if ! env -u TMUX "$TMUX_BIN_REAL" -L "$sock" has-session -t '=my-repo' 2>/dev/null; then
  echo 'expected my-repo session to be created from current git root' >&2
  exit 1
fi

kind="$(env -u TMUX "$TMUX_BIN_REAL" -L "$sock" show-options -t my-repo -vq @session-sidebar-kind)"
[ "$kind" = 'project' ] || {
  echo "expected @session-sidebar-kind=project, got: ${kind:-<empty>}" >&2
  exit 1
}

stored_path="$(env -u TMUX "$TMUX_BIN_REAL" -L "$sock" show-options -t my-repo -vq @session-sidebar-project-path)"
[ "$stored_path" = "$project_root" ] || {
  echo "expected project path $project_root, got: ${stored_path:-<empty>}" >&2
  exit 1
}

client_session="$(env -u TMUX "$TMUX_BIN_REAL" -L "$sock" list-clients -F '#{client_session}' | head -n1)"
[ "$client_session" = 'my-repo' ] || {
  echo "expected client to switch to my-repo, got: ${client_session:-<empty>}" >&2
  exit 1
}

missing_path="$work_dir/not-a-repo"
mkdir -p "$missing_path"
if env -u TMUX "$TMUX_BIN_REAL" -L "$sock" run-shell "$(printf '%q ' "$REPO_DIR/scripts/actions/create-current-git-project-session.sh" --client "$client_name" --source-path "$missing_path")"; then
  echo 'expected non-git source path to fail' >&2
  exit 1
fi
sleep 1

if env -u TMUX "$TMUX_BIN_REAL" -L "$sock" has-session -t '=not-a-repo' 2>/dev/null; then
  echo 'did not expect a session to be created for a non-git path' >&2
  exit 1
fi

messages="$(env -u TMUX "$TMUX_BIN_REAL" -L "$sock" show-messages -t "$client_name" 2>/dev/null || true)"
if ! printf '%s\n' "$messages" | grep -Fq 'current path is not inside a git repository'; then
  echo 'expected tmux message for non-git source path failure' >&2
  printf 'messages:\n%s\n' "$messages" >&2
  exit 1
fi

kill "$client_pid" 2>/dev/null || true
rm -f "$client_log"

echo 'ok: current git repo root became a project session and non-git paths fail clearly'
