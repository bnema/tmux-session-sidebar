#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local expected="$1" actual="$2" message="$3"
  if [ "$expected" != "$actual" ]; then
    fail "$message: expected '$expected', got '$actual'"
  fi
}

assert_file_contains() {
  local file="$1" needle="$2" message="$3"
  if ! grep -Fq -- "$needle" "$file"; then
    echo "--- $file ---" >&2
    cat "$file" >&2 || true
    fail "$message: missing '$needle'"
  fi
}

TEMP_ROOTS=()
cleanup_temp_roots() {
  local root
  for root in "${TEMP_ROOTS[@]}"; do
    rm -rf "$root"
  done
}
trap cleanup_temp_roots EXIT

new_fixture() {
  local root repo home fakebin gh_log
  root="$(mktemp -d)"
  repo="$root/repo"
  home="$root/home"
  fakebin="$root/fakebin"
  gh_log="$root/gh.log"
  mkdir -p "$repo/scripts" "$home/.ssh" "$fakebin"
  cp "$(dirname "$0")/setup-update-runtime-signing.sh" "$repo/scripts/setup-update-runtime-signing.sh"
  chmod +x "$repo/scripts/setup-update-runtime-signing.sh"
  TEMP_ROOTS+=("$root")

  cat >"$fakebin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$TEST_GH_LOG"
case "$1 $2" in
  "secret set")
    cat >/dev/null
    ;;
  "run rerun")
    ;;
  "run watch")
    ;;
  *)
    echo "unexpected gh invocation: $*" >&2
    exit 1
    ;;
esac
GH
  chmod +x "$fakebin/gh"

  cat >"$fakebin/sleep" <<'SLEEP'
#!/usr/bin/env bash
exit 0
SLEEP
  chmod +x "$fakebin/sleep"

  printf '%s\n' "$root"
}

generate_keypair() {
  local private_key="$1" public_key="$2"
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$private_key" >/dev/null 2>&1
  openssl pkey -in "$private_key" -pubout -out "$public_key" >/dev/null 2>&1
}

run_setup() {
  local root="$1" run_id="$2"
  (
    cd "$root/repo"
    HOME="$root/home" TEST_GH_LOG="$root/gh.log" PATH="$root/fakebin:$PATH" ./scripts/setup-update-runtime-signing.sh "$run_id"
  )
}

test_moves_repo_key_to_ssh_and_reruns_release_job() {
  local root target_key
  root="$(new_fixture)"
  generate_keypair "$root/repo/update-runtime-sign-key.pem" "$root/repo/scripts/update-runtime.pub.pem"

  run_setup "$root" 27323580762

  target_key="$root/home/.ssh/tmux-session-sidebar-release-sign.pem"
  [ -f "$target_key" ] || fail "setup script should move repo private key into ~/.ssh"
  [ ! -f "$root/repo/update-runtime-sign-key.pem" ] || fail "setup script should remove repo-local private key after moving it"
  assert_file_contains "$root/gh.log" "secret set UPDATE_RUNTIME_SIGN_KEY --repo bnema/tmux-session-sidebar" "setup script should push the private key into GitHub Actions secrets"
  assert_file_contains "$root/gh.log" "run rerun 27323580762 --failed" "setup script should rerun failed jobs"
  assert_file_contains "$root/gh.log" "run watch 27323580762 --exit-status" "setup script should watch the rerun"
}

test_uses_existing_ssh_key_when_already_stored() {
  local root target_key
  root="$(new_fixture)"
  target_key="$root/home/.ssh/tmux-session-sidebar-release-sign.pem"
  generate_keypair "$target_key" "$root/repo/scripts/update-runtime.pub.pem"

  run_setup "$root" 27323580762

  [ -f "$target_key" ] || fail "setup script should keep using the existing ~/.ssh key"
  assert_file_contains "$root/gh.log" "secret set UPDATE_RUNTIME_SIGN_KEY --repo bnema/tmux-session-sidebar" "setup script should still refresh the GitHub Actions secret"
}

test_fails_when_private_key_does_not_match_repo_public_key() {
  local root status
  root="$(new_fixture)"
  generate_keypair "$root/home/.ssh/tmux-session-sidebar-release-sign.pem" "$root/repo/scripts/update-runtime.pub.pem"
  generate_keypair "$root/repo/update-runtime-sign-key.pem" "$root/repo/scripts/other.pub.pem"
  mv "$root/repo/update-runtime-sign-key.pem" "$root/home/.ssh/tmux-session-sidebar-release-sign.pem"

  set +e
  run_setup "$root" 27323580762 >/tmp/setup-signing-mismatch.log 2>&1
  status="$?"
  set -e

  [ "$status" -ne 0 ] || fail "setup script should fail when the private key does not match scripts/update-runtime.pub.pem"
  [ ! -f "$root/gh.log" ] || [ ! -s "$root/gh.log" ] || fail "setup script should fail before calling gh when the key mismatches"
}

test_moves_repo_key_to_ssh_and_reruns_release_job
test_uses_existing_ssh_key_when_already_stored
test_fails_when_private_key_does_not_match_repo_public_key

echo "setup-update-runtime-signing tests passed"
