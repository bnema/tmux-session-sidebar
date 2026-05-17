#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
REAL_TMUX_BIN="$(command -v tmux 2>/dev/null || true)"
SCRIPT_BIN="$(command -v script 2>/dev/null || true)"
[ -n "$DIRNAME_BIN" ] || { echo 'dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'pwd not found' >&2; exit 1; }
[ -n "$REAL_TMUX_BIN" ] || { echo 'tmux not found' >&2; exit 1; }
[ -n "$SCRIPT_BIN" ] || { echo 'script not found' >&2; exit 1; }
REPO_DIR="$(cd "$($DIRNAME_BIN "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1

work_dir="$(mktemp -d)"
sock="tss_test_sidebar_quick_switch_follows_open_sidebar_$$"
client_log="$work_dir/client.log"
fake_bin="$work_dir/bin"
client_pid=""
mkdir -p "$fake_bin"

cleanup() {
  if [ -n "$client_pid" ]; then
    kill "$client_pid" 2>/dev/null || true
  fi
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" kill-server 2>/dev/null || true
  rm -rf "$work_dir"
}
trap cleanup EXIT

cat >"$fake_bin/tmux" <<EOF
#!/usr/bin/env bash
exec "$REAL_TMUX_BIN" -L "$sock" "\$@"
EOF
chmod +x "$fake_bin/tmux"

env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s alpha 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -f /dev/null -L "$sock" new-session -d -s beta 'sleep 9999'
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -g @session-sidebar-heat-colors off
"$SCRIPT_BIN" -q -c "env -u TMUX TERM=xterm-256color tmux -L $sock attach-session -t alpha" "$client_log" >/dev/null 2>&1 &
client_pid=$!

client_name=""
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  client_name="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_name}' | head -n1)"
  if [ -n "$client_name" ]; then
    break
  fi
  sleep 0.2
done
[ -n "$client_name" ] || { echo 'no tmux client found' >&2; exit 1; }

alpha_window_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-windows -t '=alpha' -F '#{window_id}' | head -n1)"
beta_window_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-windows -t '=beta' -F '#{window_id}' | head -n1)"
sidebar_pane_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" split-window -P -F '#{pane_id}' -t "$alpha_window_id" -hbf -l 20 'sleep 9999')"

env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-pane 1
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-client "$client_name"
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-source-path "$work_dir"
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-active-filter beta
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$sidebar_pane_id" @session-sidebar-show-numbered-sessions off

# With heat colors disabled, visible sessions are listed by tmux's stable session order;
# alpha is slot 1 and beta is slot 2 in this fixture.
env -u TMUX PATH="$fake_bin:$PATH" "$REPO_DIR/scripts/actions/quick-switch-session.sh" --client "$client_name" --index 2

client_session=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  client_session="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_session}' | head -n1)"
  if [ "$client_session" = 'beta' ]; then
    break
  fi
  sleep 0.2
done
[ "$client_session" = 'beta' ] || {
  echo "expected Ctrl-number quick switch to switch client to beta, got: ${client_session:-<empty>}" >&2
  exit 1
}

alpha_sidebar_count="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$alpha_window_id" -F '#{@session-sidebar-pane}' | grep -Fc -- '1' || true)"
[ "$alpha_sidebar_count" -eq 1 ] || {
  echo 'expected quick switch to keep the original sidebar available in the previous session' >&2
  exit 1
}

new_sidebar_pane_id=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  new_sidebar_pane_id="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$beta_window_id" -F '#{pane_id} #{@session-sidebar-pane}' | awk '$2 == 1 { print $1; exit }')"
  if [ -n "$new_sidebar_pane_id" ]; then
    break
  fi
  sleep 0.2
done
[ -n "$new_sidebar_pane_id" ] || {
  echo 'expected quick switch to recreate the open sidebar in the target session' >&2
  env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -a -F '#{session_name} #{window_id} #{pane_id} #{@session-sidebar-pane} #{pane_current_command}' >&2
  exit 1
}

show_numbered_state="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -p -t "$new_sidebar_pane_id" -vq @session-sidebar-show-numbered-sessions)"
[ "$show_numbered_state" = 'off' ] || {
  echo "expected replacement sidebar to preserve numeric-session visibility state as off, got $show_numbered_state" >&2
  exit 1
}

source_path_state="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -p -t "$new_sidebar_pane_id" -vq @session-sidebar-source-path)"
[ "$source_path_state" = "$work_dir" ] || {
  echo "expected replacement sidebar to preserve source path as $work_dir, got ${source_path_state:-<empty>}" >&2
  exit 1
}

active_filter_state="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -p -t "$new_sidebar_pane_id" -vq @session-sidebar-active-filter)"
[ "$active_filter_state" = 'beta' ] || {
  echo "expected replacement sidebar to preserve active filter as beta, got ${active_filter_state:-<empty>}" >&2
  exit 1
}

env -u TMUX PATH="$fake_bin:$PATH" "$REPO_DIR/scripts/actions/quick-switch-session.sh" --client "$client_name" --index 1

client_session=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  client_session="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_session}' | head -n1)"
  if [ "$client_session" = 'alpha' ]; then
    break
  fi
  sleep 0.2
done
[ "$client_session" = 'alpha' ] || {
  echo "expected Ctrl-number quick switch to switch client back to alpha, got: ${client_session:-<empty>}" >&2
  exit 1
}

if ! env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$alpha_window_id" -F '#{pane_id}' | grep -Fxq "$sidebar_pane_id"; then
  echo 'expected quick switch back to reuse the original alpha sidebar pane instead of recreating it' >&2
  exit 1
fi

if ! env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$beta_window_id" -F '#{pane_id}' | grep -Fxq "$new_sidebar_pane_id"; then
  echo 'expected quick switch back to keep the beta sidebar pane available for reuse' >&2
  exit 1
fi

beta_source_path="$work_dir/beta-source"
mkdir -p "$beta_source_path"
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$new_sidebar_pane_id" @session-sidebar-source-path "$beta_source_path"
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$new_sidebar_pane_id" @session-sidebar-active-filter alpha
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$new_sidebar_pane_id" @session-sidebar-show-numbered-sessions on
env -u TMUX "$REAL_TMUX_BIN" -L "$sock" set-option -p -t "$new_sidebar_pane_id" @session-sidebar-refresh-socket "$work_dir/missing-refresh.sock"
env -u TMUX PATH="$fake_bin:$PATH" "$REPO_DIR/scripts/actions/quick-switch-session.sh" --client "$client_name" --index 2

client_session=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  client_session="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-clients -F '#{client_session}' | head -n1)"
  if [ "$client_session" = 'beta' ]; then
    break
  fi
  sleep 0.2
done
[ "$client_session" = 'beta' ] || {
  echo "expected Ctrl-number quick switch to switch client back to beta, got: ${client_session:-<empty>}" >&2
  exit 1
}

if env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$beta_window_id" -F '#{pane_id}' | grep -Fxq "$new_sidebar_pane_id"; then
  echo 'expected quick switch to replace a target sidebar with a stale refresh socket' >&2
  exit 1
fi

replacement_beta_sidebar="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" list-panes -t "$beta_window_id" -F '#{pane_id} #{@session-sidebar-pane}' | awk '$2 == 1 { print $1; exit }')"
[ -n "$replacement_beta_sidebar" ] || {
  echo 'expected quick switch to recreate a sidebar after replacing the stale target sidebar' >&2
  exit 1
}

replacement_source_path="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -p -t "$replacement_beta_sidebar" -vq @session-sidebar-source-path)"
[ "$replacement_source_path" = "$beta_source_path" ] || {
  echo "expected stale target sidebar replacement to preserve source path as $beta_source_path, got ${replacement_source_path:-<empty>}" >&2
  exit 1
}

replacement_show_numbered="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -p -t "$replacement_beta_sidebar" -vq @session-sidebar-show-numbered-sessions)"
[ "$replacement_show_numbered" = 'on' ] || {
  echo "expected stale target sidebar replacement to preserve numeric-session visibility as on, got ${replacement_show_numbered:-<empty>}" >&2
  exit 1
}

replacement_active_filter="$(env -u TMUX "$REAL_TMUX_BIN" -L "$sock" show-options -p -t "$replacement_beta_sidebar" -vq @session-sidebar-active-filter)"
[ "$replacement_active_filter" = 'alpha' ] || {
  echo "expected stale target sidebar replacement to preserve active filter as alpha, got ${replacement_active_filter:-<empty>}" >&2
  exit 1
}

echo 'ok: quick-switch preserves open sidebars, reuses them, and replaces stale target sidebars'
