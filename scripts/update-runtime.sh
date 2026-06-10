#!/usr/bin/env bash
set -euo pipefail

DIRNAME_BIN="$(command -v dirname 2>/dev/null || true)"
PWD_BIN="$(command -v pwd 2>/dev/null || true)"
GO_BIN="$(command -v go 2>/dev/null || true)"
GIT_BIN="$(command -v git 2>/dev/null || true)"
FIND_BIN="$(command -v find 2>/dev/null || true)"
SORT_BIN="$(command -v sort 2>/dev/null || true)"
CKSUM_BIN="$(command -v cksum 2>/dev/null || true)"
CURL_BIN="$(command -v curl 2>/dev/null || true)"
TAR_BIN="$(command -v tar 2>/dev/null || true)"
UNAME_BIN="$(command -v uname 2>/dev/null || true)"
SHA256SUM_BIN="$(command -v sha256sum 2>/dev/null || true)"
SHASUM_BIN="$(command -v shasum 2>/dev/null || true)"
OPENSSL_BIN="$(command -v openssl 2>/dev/null || true)"
CP_BIN="$(command -v cp 2>/dev/null || true)"
RM_BIN="$(command -v rm 2>/dev/null || true)"
MKDIR_BIN="$(command -v mkdir 2>/dev/null || true)"
MV_BIN="$(command -v mv 2>/dev/null || true)"
CHMOD_BIN="$(command -v chmod 2>/dev/null || true)"
SLEEP_BIN="$(command -v sleep 2>/dev/null || true)"
GREP_BIN="$(command -v grep 2>/dev/null || true)"
CAT_BIN="$(command -v cat 2>/dev/null || true)"
TR_BIN="$(command -v tr 2>/dev/null || true)"
PS_BIN="$(command -v ps 2>/dev/null || true)"
PKILL_BIN="$(command -v pkill 2>/dev/null || true)"
TMUX_BIN="$(command -v tmux 2>/dev/null || true)"

[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
[ -n "$MKDIR_BIN" ] || { echo 'tmux-session-sidebar: mkdir not found' >&2; exit 1; }
[ -n "$MV_BIN" ] || { echo 'tmux-session-sidebar: mv not found' >&2; exit 1; }
[ -n "$RM_BIN" ] || { echo 'tmux-session-sidebar: rm not found' >&2; exit 1; }
[ -n "$CHMOD_BIN" ] || { echo 'tmux-session-sidebar: chmod not found' >&2; exit 1; }

PLUGIN_DIR="$(cd "$("$DIRNAME_BIN" "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1
BIN_DIR="$PLUGIN_DIR/.bin"
runtime_bin="$BIN_DIR/tmux-session-sidebar"
stamp_file="$BIN_DIR/.build-fingerprint"
dev_marker_file="$BIN_DIR/.dev-runtime"
RELEASE_REPO="${TMUX_SESSION_SIDEBAR_RELEASE_REPO:-bnema/tmux-session-sidebar}"
STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/tmux-session-sidebar"
TMUX_CONF="${TMUX_CONF:-}"
SIDEBAR_SESSION_NAME="${SIDEBAR_SESSION_NAME:-__tmux-session-sidebar}"
UPDATE_LOG="${TMUX_SESSION_SIDEBAR_UPDATE_LOG:-$BIN_DIR/update-runtime.log}"
lock_dir="$BIN_DIR/update-runtime.lock"
lock_acquired=0

log_update() {
  local message="$1"
  printf 'tmux-session-sidebar: %s\n' "$message" >&2
  if "$MKDIR_BIN" -p "$BIN_DIR" 2>/dev/null; then
    printf 'tmux-session-sidebar: %s\n' "$message" >>"$UPDATE_LOG" 2>/dev/null || true
  fi
}

cleanup_lock() {
  if [ "$lock_acquired" = 1 ]; then
    "$RM_BIN" -rf "$lock_dir" 2>/dev/null || true
    lock_acquired=0
  fi
}

acquire_update_lock() {
  local attempt wait_retries
  "$MKDIR_BIN" -p "$BIN_DIR"
  wait_retries="${TMUX_SESSION_SIDEBAR_UPDATE_LOCK_RETRIES:-100}"
  case "$wait_retries" in
    *[!0-9]*|"") wait_retries=100 ;;
  esac
  for ((attempt = 1; attempt <= wait_retries; attempt++)); do
    if "$MKDIR_BIN" "$lock_dir" 2>/dev/null; then
      lock_acquired=1
      trap cleanup_lock EXIT
      return 0
    fi
    [ -n "$SLEEP_BIN" ] || { log_update 'sleep not found while waiting for update lock'; return 1; }
    "$SLEEP_BIN" 0.1
  done
  log_update "could not acquire update lock: $lock_dir"
  return 1
}

source_fingerprint() {
  local go_version git_rev
  [ -n "$GO_BIN" ] || { echo 'tmux-session-sidebar: go not found' >&2; exit 1; }
  go_version="$($GO_BIN version 2>/dev/null || true)"

  if [ -n "$GIT_BIN" ]; then
    git_rev="$($GIT_BIN -C "$PLUGIN_DIR" rev-parse HEAD 2>/dev/null || true)"
    if [ -n "$git_rev" ] && [ -z "$($GIT_BIN -C "$PLUGIN_DIR" status --porcelain --untracked-files=all 2>/dev/null)" ]; then
      printf 'git:%s\ngo:%s\n' "$git_rev" "$go_version"
      return 0
    fi
  fi

  [ -n "$FIND_BIN" ] || { echo 'tmux-session-sidebar: find not found' >&2; exit 1; }
  [ -n "$SORT_BIN" ] || { echo 'tmux-session-sidebar: sort not found' >&2; exit 1; }
  [ -n "$CKSUM_BIN" ] || { echo 'tmux-session-sidebar: cksum not found' >&2; exit 1; }

  {
    printf 'source\ngo:%s\n' "$go_version"
    cd "$PLUGIN_DIR"
    for path in go.mod go.sum cmd internal core adapters ports; do
      [ -e "$path" ] || continue
      if [ -d "$path" ]; then
        "$FIND_BIN" "$path" -type f
      else
        printf '%s\n' "$path"
      fi
    done | "$SORT_BIN" | while IFS= read -r file; do
      printf '%s ' "$file"
      "$CKSUM_BIN" "$file"
    done
  }
}

plugin_head() {
  local head
  if [ -n "$GIT_BIN" ]; then
    head="$($GIT_BIN -C "$PLUGIN_DIR" rev-parse HEAD 2>/dev/null || true)"
    [ -n "$head" ] && { printf '%s\n' "$head"; return 0; }
  fi
  printf 'unknown\n'
}

release_os() {
  [ -n "$UNAME_BIN" ] || return 1
  case "$($UNAME_BIN -s 2>/dev/null || true)" in
    Linux) printf 'Linux' ;;
    Darwin) printf 'Darwin' ;;
    *) return 1 ;;
  esac
}

release_arch() {
  [ -n "$UNAME_BIN" ] || return 1
  case "$($UNAME_BIN -m 2>/dev/null || true)" in
    x86_64|amd64) printf 'x86_64' ;;
    aarch64|arm64) printf 'arm64' ;;
    *) return 1 ;;
  esac
}

validate_runtime() {
  local bin="$1" output
  [ -x "$bin" ] || return 1
  output="$($bin version 2>/dev/null || true)"
  case "$output" in
    tmux-session-sidebar\ *) return 0 ;;
    *)
      echo "tmux-session-sidebar: runtime validation failed: $bin version did not produce valid output (expected 'tmux-session-sidebar <version>')" >&2
      return 1
      ;;
  esac
}

runtime_version() {
  local bin="$1" first ignored version
  first="$($bin version 2>/dev/null || true)"
  first="${first%%$'\n'*}"
  read -r ignored version ignored <<EOF
$first
EOF
  [ -n "$version" ] || version=unknown
  printf '%s\n' "$version"
}

sha256_file() {
  local checksum file="$1" ignored
  if [ -n "$SHA256SUM_BIN" ]; then
    checksum="$($SHA256SUM_BIN "$file")" || return 1
    read -r checksum ignored <<EOF
$checksum
EOF
    printf '%s\n' "$checksum"
    return 0
  fi
  if [ -n "$SHASUM_BIN" ]; then
    checksum="$($SHASUM_BIN -a 256 "$file")" || return 1
    read -r checksum ignored <<EOF
$checksum
EOF
    printf '%s\n' "$checksum"
    return 0
  fi
  echo 'tmux-session-sidebar: sha256sum or shasum not found' >&2
  return 1
}

verify_release_checksum() {
  local archive="$1" asset="$2" checksums_file="$3" actual expected filename line
  expected=""
  while read -r line; do
    read -r expected filename <<EOF
$line
EOF
    filename="${filename#./}"
    filename="${filename##*/}"
    if [ "$filename" = "$asset" ]; then
      break
    fi
    expected=""
  done <"$checksums_file"
  if [ -z "$expected" ]; then
    echo "tmux-session-sidebar: checksum not found for $asset" >&2
    return 1
  fi
  actual="$(sha256_file "$archive")" || return 1
  if [ "$actual" != "$expected" ]; then
    echo "tmux-session-sidebar: checksum mismatch for $asset" >&2
    return 1
  fi
  printf '%s\n' "$actual"
}

verify_release_signature() {
  local checksums_file="$1" sig_file="$2"
  local pubkey
  pubkey="$(cd "$("$DIRNAME_BIN" "${BASH_SOURCE[0]}")" && "$PWD_BIN")/update-runtime.pub.pem"
  if [ ! -f "$pubkey" ]; then
    echo "tmux-session-sidebar: public key not found at $pubkey" >&2
    return 1
  fi
  if [ ! -f "$sig_file" ]; then
    echo "tmux-session-sidebar: signature file not found at $sig_file" >&2
    return 1
  fi
  if [ -z "$OPENSSL_BIN" ]; then
    echo "tmux-session-sidebar: openssl not found; cannot verify release signature" >&2
    return 1
  fi
  "$OPENSSL_BIN" dgst -sha256 -verify "$pubkey" -signature "$sig_file" "$checksums_file" 2>/dev/null || {
    echo "tmux-session-sidebar: checksums.txt signature verification failed" >&2
    return 1
  }
}

release_stamp_for() {
  local version="$1" checksum="$2" head
  head="$(plugin_head)"
  printf 'release:%s:latest\nrelease-version:%s\nasset-sha256:%s\nplugin-head:%s\n' "$RELEASE_REPO" "$version" "$checksum" "$head"
}

cached_release_valid() {
  local stamp head
  [ -x "$runtime_bin" ] || return 1
  [ -f "$stamp_file" ] || return 1
  validate_runtime "$runtime_bin" || return 1
  [ -n "$CAT_BIN" ] || return 1
  [ -n "$GREP_BIN" ] || return 1
  stamp="$($CAT_BIN "$stamp_file" 2>/dev/null || true)"
  case "$stamp" in
    release:"$RELEASE_REPO":latest)
      # Legacy stamps did not record plugin HEAD/version, so refresh once when possible.
      return 1
      ;;
  esac
  printf '%s\n' "$stamp" | "$GREP_BIN" -Fqx "release:$RELEASE_REPO:latest" || return 1
  printf '%s\n' "$stamp" | "$GREP_BIN" -Fq 'release-version:' || return 1
  printf '%s\n' "$stamp" | "$GREP_BIN" -Fq 'asset-sha256:' || return 1
  if printf '%s\n' "$stamp" | "$GREP_BIN" -Fq 'plugin-head:'; then
    head="$(plugin_head)"
    printf '%s\n' "$stamp" | "$GREP_BIN" -Fqx "plugin-head:$head" || return 1
  fi
  return 0
}

download_release_candidate() {
  local candidate="$1" archive arch asset checksum checksums checksums_url os tmp_dir tmp_runtime url version
  [ -n "$CURL_BIN" ] || return 1
  [ -n "$TAR_BIN" ] || return 1
  [ -n "$UNAME_BIN" ] || return 1
  os="$(release_os)" || return 1
  arch="$(release_arch)" || return 1
  asset="tmux-session-sidebar_${os}_${arch}.tar.gz"
  url="https://github.com/$RELEASE_REPO/releases/latest/download/$asset"
  checksums_url="https://github.com/$RELEASE_REPO/releases/latest/download/checksums.txt"
  checksums_sig_url="https://github.com/$RELEASE_REPO/releases/latest/download/checksums.txt.sig"
  tmp_dir="$BIN_DIR/download.$$.$RANDOM"
  archive="$tmp_dir/$asset"
  checksums="$tmp_dir/checksums.txt"
  checksums_sig="$tmp_dir/checksums.txt.sig"
  "$RM_BIN" -rf "$tmp_dir"
  "$MKDIR_BIN" -p "$tmp_dir" || return 1
  log_update "downloading checksums metadata from $checksums_url"
  "$CURL_BIN" -fsSL -o "$checksums" "$checksums_url" || { "$RM_BIN" -rf "$tmp_dir"; return 1; }
  "$CURL_BIN" -fsSL -o "$checksums_sig" "$checksums_sig_url" || {
    echo "tmux-session-sidebar: failed to download signature file; release authenticity cannot be verified" >&2
    "$RM_BIN" -rf "$tmp_dir"; return 2;
  }
  verify_release_signature "$checksums" "$checksums_sig" || { "$RM_BIN" -rf "$tmp_dir"; return 2; }
  log_update "downloading runtime from $url"
  "$CURL_BIN" -fsSL -o "$archive" "$url" || { "$RM_BIN" -rf "$tmp_dir"; return 1; }
  checksum="$(verify_release_checksum "$archive" "$asset" "$checksums")" || { "$RM_BIN" -rf "$tmp_dir"; return 2; }
  "$TAR_BIN" -xzf "$archive" -C "$tmp_dir" tmux-session-sidebar || { "$RM_BIN" -rf "$tmp_dir"; return 2; }
  tmp_runtime="$tmp_dir/tmux-session-sidebar"
  "$CHMOD_BIN" +x "$tmp_runtime" || { "$RM_BIN" -rf "$tmp_dir"; return 2; }
  validate_runtime "$tmp_runtime" || { "$RM_BIN" -rf "$tmp_dir"; return 2; }
  version="$(runtime_version "$tmp_runtime")"
  "$MKDIR_BIN" -p "$("$DIRNAME_BIN" "$candidate")" || { "$RM_BIN" -rf "$tmp_dir"; return 1; }
  "$MV_BIN" "$tmp_runtime" "$candidate" || { "$RM_BIN" -rf "$tmp_dir"; return 1; }
  "$RM_BIN" -rf "$tmp_dir"
  release_stamp_for "$version" "$checksum"
}

build_source_candidate() {
  local candidate="$1"
  [ -n "$GO_BIN" ] || return 1
  "$MKDIR_BIN" -p "$("$DIRNAME_BIN" "$candidate")" || return 1
  log_update "building runtime candidate at $candidate"
  (cd "$PLUGIN_DIR" && "$GO_BIN" build -o "$candidate" ./cmd/tmux-session-sidebar) || return 1
  validate_runtime "$candidate" || return 1
}

atomic_install_runtime() {
  local candidate="$1" stamp="$2" stamp_tmp
  validate_runtime "$candidate" || return 1
  "$MKDIR_BIN" -p "$BIN_DIR" || return 1
  "$MV_BIN" "$candidate" "$runtime_bin" || return 1
  stamp_tmp="$stamp_file.tmp.$$"
  printf '%s' "$stamp" >"$stamp_tmp" || return 1
  "$MV_BIN" "$stamp_tmp" "$stamp_file" || { "$RM_BIN" -f "$stamp_tmp" 2>/dev/null || true; return 1; }
}

ensure_release_runtime() {
  local candidate release_status stamp tmp_dir
  if [ "${TMUX_SESSION_SIDEBAR_REFRESH_RELEASE:-}" != "1" ] && cached_release_valid; then
    printf '%s\n' "$runtime_bin"
    return 0
  fi
  tmp_dir="$BIN_DIR/ensure.$$.$RANDOM"
  candidate="$tmp_dir/tmux-session-sidebar"
  "$RM_BIN" -rf "$tmp_dir"
  "$MKDIR_BIN" -p "$tmp_dir"
  if stamp="$(download_release_candidate "$candidate")"; then
    atomic_install_runtime "$candidate" "$stamp" || { "$RM_BIN" -rf "$tmp_dir"; return 1; }
    "$RM_BIN" -rf "$tmp_dir"
    printf '%s\n' "$runtime_bin"
    return 0
  else
    release_status="$?"
  fi
  "$RM_BIN" -rf "$tmp_dir"
  return "$release_status"
}

ensure_source_runtime() {
  local candidate fingerprint tmp_dir
  fingerprint="$(source_fingerprint)"
  if [ -n "$CAT_BIN" ] && [ -x "$runtime_bin" ] && [ -f "$stamp_file" ] && [ "$($CAT_BIN "$stamp_file" 2>/dev/null || true)" = "$fingerprint" ] && validate_runtime "$runtime_bin"; then
    printf '%s\n' "$runtime_bin"
    return 0
  fi
  tmp_dir="$BIN_DIR/source.$$.$RANDOM"
  candidate="$tmp_dir/tmux-session-sidebar"
  "$RM_BIN" -rf "$tmp_dir"
  "$MKDIR_BIN" -p "$tmp_dir"
  build_source_candidate "$candidate" || { "$RM_BIN" -rf "$tmp_dir"; return 1; }
  atomic_install_runtime "$candidate" "$fingerprint" || { "$RM_BIN" -rf "$tmp_dir"; return 1; }
  "$RM_BIN" -rf "$tmp_dir"
  printf '%s\n' "$runtime_bin"
}

prefer_source_runtime() {
  [ "${TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE:-}" = "1" ] || [ -f "$dev_marker_file" ]
}

ensure_runtime() {
  local release_status
  "$MKDIR_BIN" -p "$BIN_DIR"
  if ! prefer_source_runtime; then
    if ensure_release_runtime; then
      return 0
    else
      release_status="$?"
    fi
    if [ "$release_status" -eq 2 ]; then
      log_update 'release artifact failed validation; existing runtime left untouched'
      return 1
    fi
    if [ -z "$GO_BIN" ]; then
      if validate_runtime "$runtime_bin"; then
        log_update 'release refresh failed; using cached runtime'
        printf '%s\n' "$runtime_bin"
        return 0
      fi
      echo 'tmux-session-sidebar: go not found and no released runtime could be installed' >&2
      return 1
    fi
    log_update 'release download failed; falling back to local go build'
  fi
  ensure_source_runtime
}

is_runtime_pid() {
  local pid="$1" subcommand="$2" command
  [ -n "$pid" ] || return 1
  case "$pid" in
    *[!0-9]*) return 1 ;;
  esac
  [ -n "$PS_BIN" ] || return 1
  command="$($PS_BIN -o command= -p "$pid" 2>/dev/null || true)"
  case "$command" in
    "$runtime_bin daemon $subcommand"|"$runtime_bin daemon $subcommand "*) return 0 ;;
  esac
  return 1
}

wait_for_pid_exit() {
  local pid="$1" subcommand="$2" attempt wait_retries
  wait_retries="${TMUX_SESSION_SIDEBAR_STOP_RETRIES:-30}"
  case "$wait_retries" in
    *[!0-9]*|"") wait_retries=30 ;;
  esac
  for ((attempt = 1; attempt <= wait_retries; attempt++)); do
    if ! is_runtime_pid "$pid" "$subcommand"; then
      return 0
    fi
    [ -n "$SLEEP_BIN" ] || { log_update 'sleep not found while waiting for runtime process exit'; return 1; }
    "$SLEEP_BIN" 0.1
  done
  ! is_runtime_pid "$pid" "$subcommand"
}

stop_pid() {
  local pid="$1" subcommand="$2"
  is_runtime_pid "$pid" "$subcommand" || return 0
  kill "$pid" 2>/dev/null || true
  if wait_for_pid_exit "$pid" "$subcommand"; then
    return 0
  fi
  log_update "runtime pid $pid did not stop after TERM; sending KILL"
  kill -KILL "$pid" 2>/dev/null || true
  wait_for_pid_exit "$pid" "$subcommand"
}

ere_escape() {
  local char escaped input="$1" index
  escaped=""
  for ((index = 0; index < ${#input}; index++)); do
    char="${input:index:1}"
    case "$char" in
      [\\.^\$\*+?\(\)\[\]\{\}\|]) escaped="$escaped\\$char" ;;
      *) escaped="$escaped$char" ;;
    esac
  done
  printf '%s\n' "$escaped"
}

pkill_runtime_subcommand() {
  local pattern status subcommand="$1"
  if [ -z "$PKILL_BIN" ]; then
    log_update "pkill not found; cannot stop runtime $subcommand processes"
    return 1
  fi
  pattern="$(ere_escape "$runtime_bin daemon $subcommand")"
  "$PKILL_BIN" -f "$pattern" 2>/dev/null
  status="$?"
  case "$status" in
    0|1) return 0 ;;
    *) log_update "pkill failed for runtime $subcommand processes"; return "$status" ;;
  esac
}

stop_runtime_processes() {
  local pid_file old_pid
  pid_file="$STATE_DIR/daemon.pid"
  if [ -f "$pid_file" ]; then
    if [ -n "$TR_BIN" ]; then
      old_pid="$($TR_BIN -d '\n' <"$pid_file" 2>/dev/null || true)"
    elif [ -n "$CAT_BIN" ]; then
      old_pid="$($CAT_BIN "$pid_file" 2>/dev/null || true)"
      old_pid="${old_pid%%$'\n'*}"
    else
      old_pid=""
    fi
    if ! stop_pid "$old_pid" serve; then
      log_update "failed to stop daemon pid $old_pid"
      return 1
    fi
  fi
  pkill_runtime_subcommand serve-ui || return 1
  pkill_runtime_subcommand bootstrap || return 1
  pkill_runtime_subcommand serve || return 1
  [ -n "$TMUX_BIN" ] || return 0
  "$TMUX_BIN" kill-session -t "$SIDEBAR_SESSION_NAME" 2>/dev/null || true
}

source_tmux_runtime() {
  local config_file config_files sourced
  [ -n "$TMUX_BIN" ] || { echo 'tmux-session-sidebar: tmux not found' >&2; return 1; }
  if [ -n "$TMUX_CONF" ]; then
    "$TMUX_BIN" source-file "$TMUX_CONF"
    return
  fi
  config_files="$($TMUX_BIN display-message -p '#{config_files}' 2>/dev/null || true)"
  sourced=0
  if [ -n "$config_files" ]; then
    local IFS=,
    for config_file in $config_files; do
      [ -n "$config_file" ] || continue
      [ -f "$config_file" ] || continue
      "$TMUX_BIN" source-file "$config_file"
      sourced=1
    done
  fi
  if [ "$sourced" = 1 ]; then
    return
  fi
  config_file="${XDG_CONFIG_HOME:-$HOME/.config}/tmux/tmux.conf"
  if [ -f "$config_file" ]; then
    "$TMUX_BIN" source-file "$config_file"
    return
  fi
  "$TMUX_BIN" source-file "$HOME/.tmux.conf"
}

restart_tmux_runtime() {
  stop_runtime_processes
  source_tmux_runtime
}

restore_backup() {
  local backup_dir="$1"
  [ -n "$CP_BIN" ] || { log_update 'cp not found; cannot restore runtime backup'; return 1; }
  [ -n "$CHMOD_BIN" ] || { log_update 'chmod not found; cannot restore runtime permissions'; return 1; }
  [ -n "$RM_BIN" ] || { log_update 'rm not found; cannot remove missing backup targets'; return 1; }
  if [ -f "$backup_dir/tmux-session-sidebar" ]; then
    "$CP_BIN" "$backup_dir/tmux-session-sidebar" "$runtime_bin" || return 1
    "$CHMOD_BIN" +x "$runtime_bin" || return 1
  else
    "$RM_BIN" -f "$runtime_bin" || return 1
  fi
  if [ -f "$backup_dir/.build-fingerprint" ]; then
    "$CP_BIN" "$backup_dir/.build-fingerprint" "$stamp_file" || return 1
  else
    "$RM_BIN" -f "$stamp_file" || return 1
  fi
}

update_runtime_one_shot() {
  local backup_dir candidate release_status stamp stop_rc tmp_dir
  "$MKDIR_BIN" -p "$BIN_DIR"
  acquire_update_lock
  tmp_dir="$BIN_DIR/update.$$.$RANDOM"
  backup_dir="$BIN_DIR/backup.$$.$RANDOM"
  candidate="$tmp_dir/tmux-session-sidebar"
  "$RM_BIN" -rf "$tmp_dir" "$backup_dir"
  "$MKDIR_BIN" -p "$tmp_dir" "$backup_dir"

  if [ "${TMUX_SESSION_SIDEBAR_BUILD_FROM_SOURCE:-}" = "1" ]; then
    stamp="$(source_fingerprint)"
    build_source_candidate "$candidate" || { "$RM_BIN" -rf "$tmp_dir" "$backup_dir"; return 1; }
  else
    if stamp="$(download_release_candidate "$candidate")"; then
      release_status=0
    else
      release_status="$?"
    fi
    if [ "$release_status" -ne 0 ]; then
      "$RM_BIN" -rf "$tmp_dir"
      "$MKDIR_BIN" -p "$tmp_dir" "$backup_dir"
      if [ "$release_status" -eq 2 ]; then
        log_update 'release artifact failed validation; existing runtime left untouched'
        "$RM_BIN" -rf "$tmp_dir" "$backup_dir"
        return 1
      fi
      if [ "${TMUX_SESSION_SIDEBAR_RELEASE_ONLY:-}" = "1" ]; then
        log_update 'release update failed; existing runtime left untouched'
        "$RM_BIN" -rf "$tmp_dir" "$backup_dir"
        return 1
      fi
      if [ -n "$GO_BIN" ]; then
        log_update 'release download failed during update; falling back to local go build'
        stamp="$(source_fingerprint)"
        build_source_candidate "$candidate" || { "$RM_BIN" -rf "$tmp_dir" "$backup_dir"; return 1; }
      else
        log_update 'release update failed; existing runtime left untouched'
        "$RM_BIN" -rf "$tmp_dir" "$backup_dir"
        return 1
      fi
    fi
  fi

  if [ -f "$runtime_bin" ]; then
    "$CP_BIN" "$runtime_bin" "$backup_dir/tmux-session-sidebar" || { "$RM_BIN" -rf "$tmp_dir" "$backup_dir"; return 1; }
  fi
  if [ -f "$stamp_file" ]; then
    "$CP_BIN" "$stamp_file" "$backup_dir/.build-fingerprint" || { "$RM_BIN" -rf "$tmp_dir" "$backup_dir"; return 1; }
  fi

  stop_rc=0
  stop_runtime_processes || stop_rc=$?
  if [ "$stop_rc" -ne 0 ]; then
    "$RM_BIN" -rf "$tmp_dir" "$backup_dir"
    return "$stop_rc"
  fi
  if ! atomic_install_runtime "$candidate" "$stamp"; then
    log_update 'install failed after runtime stop; restoring previous runtime'
    restore_backup "$backup_dir" || { "$RM_BIN" -rf "$tmp_dir" "$backup_dir"; return 1; }
    source_tmux_runtime >/dev/null 2>&1 || true
    "$RM_BIN" -rf "$tmp_dir" "$backup_dir"
    return 1
  fi
  if ! source_tmux_runtime; then
    log_update 'restart failed after runtime update; restoring previous runtime'
    restore_backup "$backup_dir" || { "$RM_BIN" -rf "$tmp_dir" "$backup_dir"; return 1; }
    source_tmux_runtime >/dev/null 2>&1 || true
    "$RM_BIN" -rf "$tmp_dir" "$backup_dir"
    return 1
  fi
  "$RM_BIN" -rf "$tmp_dir" "$backup_dir"
  log_update "runtime updated and restarted at $runtime_bin"
}

usage() {
  cat >&2 <<'USAGE'
usage: update-runtime.sh [--ensure|--restart-only]

Without flags, force-refresh the release runtime, atomically install it, and restart tmux-session-sidebar.
  --ensure        Ensure a runtime exists and print its path without restarting.
  --restart-only  Stop the current runtime/UI and reload tmux without rebuilding/downloading.
USAGE
}

main() {
  case "${1:-}" in
    "") update_runtime_one_shot ;;
    --ensure) acquire_update_lock; ensure_runtime ;;
    --restart-only) acquire_update_lock; restart_tmux_runtime ;;
    -h|--help) usage ;;
    *) usage; exit 2 ;;
  esac
}

main "$@"
