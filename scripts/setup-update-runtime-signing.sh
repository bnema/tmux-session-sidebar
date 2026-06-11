#!/usr/bin/env bash
set -euo pipefail

REPO_SLUG="${REPO_SLUG:-bnema/tmux-session-sidebar}"
SECRET_NAME="${SECRET_NAME:-UPDATE_RUNTIME_SIGN_KEY}"
KEY_NAME="${KEY_NAME:-tmux-session-sidebar-release-sign.pem}"
RUN_ID="${1:-}"

log() {
  printf 'tmux-session-sidebar: %s\n' "$*" >&2
}

fail() {
  log "$*"
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 not found"
}

find_private_key() {
  local ssh_key repo_key
  ssh_key="$HOME/.ssh/$KEY_NAME"
  repo_key="$(pwd)/update-runtime-sign-key.pem"

  if [ -f "$ssh_key" ]; then
    printf '%s\n' "$ssh_key"
    return 0
  fi

  if [ -f "$repo_key" ]; then
    mkdir -p "$HOME/.ssh"
    chmod 0700 "$HOME/.ssh"
    mv "$repo_key" "$ssh_key"
    chmod 0600 "$ssh_key"
    log "moved release signing key to $ssh_key"
    printf '%s\n' "$ssh_key"
    return 0
  fi

  fail "no release signing private key found at $ssh_key or $repo_key"
}

verify_key_matches_repo_pubkey() {
  local private_key="$1" pubkey_path
  pubkey_path="scripts/update-runtime.pub.pem"
  [ -f "$pubkey_path" ] || fail "public key not found at $pubkey_path"
  if ! openssl pkey -in "$private_key" -pubout 2>/dev/null | diff -q - "$pubkey_path" >/dev/null 2>&1; then
    fail "private key does not match $pubkey_path"
  fi
}

sync_github_secret() {
  local private_key="$1"
  gh secret set "$SECRET_NAME" --repo "$REPO_SLUG" <"$private_key"
  log "updated GitHub Actions secret $SECRET_NAME for $REPO_SLUG"
}

rerun_failed_release_job() {
  local run_id="$1"
  [ -n "$run_id" ] || return 0
  sleep 5
  gh run rerun "$run_id" --failed
  gh run watch "$run_id" --exit-status
}

main() {
  local private_key
  require_command openssl
  require_command gh
  require_command diff
  require_command sleep

  private_key="$(find_private_key)"
  verify_key_matches_repo_pubkey "$private_key"
  sync_github_secret "$private_key"
  rerun_failed_release_job "$RUN_ID"

  log "release signing key is stored at $private_key"
}

main "$@"
