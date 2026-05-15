#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
REAL_TMUX_BIN="$(command -v tmux 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
[ -n "$REAL_TMUX_BIN" ] || { echo 'tmux not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
sock="tss_test_popup_picker"
client_log="$work_dir/client.log"
wrapper_log="$work_dir/tmux-wrapper.log"
project_root="$work_dir/projects"
fake_bin="$work_dir/bin"
mkdir -p "$project_root/alpha" "$project_root/beta" "$fake_bin"

cleanup() {
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" kill-server 2>/dev/null || true
  rm -rf "$work_dir"
}
trap cleanup EXIT

cat >"$fake_bin/tmux" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "$wrapper_log"
exec "$REAL_TMUX_BIN" "\$@"
EOF
chmod +x "$fake_bin/tmux"

cat >"$fake_bin/fzf" <<'EOF'
#!/usr/bin/env bash
awk 'NR == 2 { print; found = 1; exit } END { if (!found && NR >= 1) print $0 }'
EOF
chmod +x "$fake_bin/fzf"

env -u TMUX PATH="$fake_bin:$PATH" tmux -f /dev/null -L "$sock" new-session -d -s alpha "sleep 9999"
script -q -c "env -u TMUX PATH='$fake_bin:$PATH' TERM=xterm-256color tmux -L $sock attach-session -t alpha" "$client_log" >/dev/null 2>&1 &
client_pid=$!
sleep 1

client_name="$(env -u TMUX PATH="$fake_bin:$PATH" tmux -L "$sock" list-clients -F '#{client_name}' | head -n1)"
[ -n "$client_name" ] || { echo 'no tmux client found' >&2; exit 1; }

env -u TMUX PATH="$fake_bin:$PATH" tmux -L "$sock" set-option -g @session-sidebar-project-roots "$project_root"
env -u TMUX PATH="$fake_bin:$PATH" tmux -L "$sock" run-shell "$(printf '%q ' "$REPO_DIR/scripts/actions/create-project-session.sh" --client "$client_name" --source-path "$project_root")"
sleep 1

kill "$client_pid" 2>/dev/null || true

if ! env -u TMUX PATH="$fake_bin:$PATH" tmux -L "$sock" has-session -t '=beta' 2>/dev/null; then
  echo 'expected beta session to be created from selected project' >&2
  exit 1
fi

if ! grep -q 'display-popup' "$wrapper_log"; then
  echo 'expected project picker to use tmux display-popup' >&2
  printf 'tmux wrapper log:\n' >&2
  cat "$wrapper_log" >&2
  exit 1
fi

echo 'ok: popup project picker created beta session via display-popup'
