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
[ -n "$DIRNAME_BIN" ] || { echo 'tmux-session-sidebar: dirname not found' >&2; exit 1; }
[ -n "$PWD_BIN" ] || { echo 'tmux-session-sidebar: pwd not found' >&2; exit 1; }
PLUGIN_DIR="$(cd "$("$DIRNAME_BIN" "${BASH_SOURCE[0]}")/.." && "$PWD_BIN")" || exit 1
BIN_DIR="$PLUGIN_DIR/.bin"
runtime_bin="$BIN_DIR/tmux-session-sidebar"
stamp_file="$BIN_DIR/.build-fingerprint"
RELEASE_REPO="${TMUX_SESSION_SIDEBAR_RELEASE_REPO:-bnema/tmux-session-sidebar}"

source_fingerprint() {
  local go_version git_rev
  go_version="$($GO_BIN version 2>/dev/null || true)"

  if [ -n "$GIT_BIN" ]; then
    git_rev="$($GIT_BIN -C "$PLUGIN_DIR" rev-parse HEAD 2>/dev/null || true)"
    if [ -n "$git_rev" ] && [ -z "$("$GIT_BIN" -C "$PLUGIN_DIR" status --porcelain --untracked-files=all 2>/dev/null)" ]; then
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

release_os() {
  case "$("$UNAME_BIN" -s 2>/dev/null || true)" in
    Linux) printf 'Linux' ;;
    Darwin) printf 'Darwin' ;;
    *) return 1 ;;
  esac
}

release_arch() {
  case "$("$UNAME_BIN" -m 2>/dev/null || true)" in
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
}

download_release_runtime() {
  local archive arch asset checksums os tmp_dir tmp_runtime url checksums_url
  [ -n "$CURL_BIN" ] || return 1
  [ -n "$TAR_BIN" ] || return 1
  [ -n "$UNAME_BIN" ] || return 1
  os="$(release_os)" || return 1
  arch="$(release_arch)" || return 1
  asset="tmux-session-sidebar_${os}_${arch}.tar.gz"
  url="https://github.com/$RELEASE_REPO/releases/latest/download/$asset"
  checksums_url="https://github.com/$RELEASE_REPO/releases/latest/download/checksums.txt"
  tmp_dir="$BIN_DIR/download.$$"
  archive="$tmp_dir/$asset"
  checksums="$tmp_dir/checksums.txt"
  rm -rf "$tmp_dir"
  mkdir -p "$tmp_dir" || return 1
  echo "tmux-session-sidebar: downloading runtime from $url" >&2
  "$CURL_BIN" -fsSL -o "$archive" "$url" || { rm -rf "$tmp_dir"; return 1; }
  "$CURL_BIN" -fsSL -o "$checksums" "$checksums_url" || { rm -rf "$tmp_dir"; return 1; }
  verify_release_checksum "$archive" "$asset" "$checksums" || { rm -rf "$tmp_dir"; return 1; }
  "$TAR_BIN" -xzf "$archive" -C "$tmp_dir" tmux-session-sidebar || { rm -rf "$tmp_dir"; return 1; }
  tmp_runtime="$tmp_dir/tmux-session-sidebar"
  chmod +x "$tmp_runtime" || { rm -rf "$tmp_dir"; return 1; }
  validate_runtime "$tmp_runtime" || { rm -rf "$tmp_dir"; return 1; }
  mv "$tmp_runtime" "$runtime_bin" || { rm -rf "$tmp_dir"; return 1; }
  rm -rf "$tmp_dir"
}

if [ -z "$GO_BIN" ]; then
  release_stamp="release:$RELEASE_REPO:latest"
  if [ -x "$runtime_bin" ] && [ "${TMUX_SESSION_SIDEBAR_REFRESH_RELEASE:-}" != "1" ] && [ "$(cat "$stamp_file" 2>/dev/null || true)" = "$release_stamp" ] && validate_runtime "$runtime_bin"; then
    printf '%s\n' "$runtime_bin"
    exit 0
  fi
  mkdir -p "$BIN_DIR"
  if download_release_runtime; then
    printf '%s\n' "$release_stamp" >"$stamp_file"
    printf '%s\n' "$runtime_bin"
    exit 0
  fi
  if validate_runtime "$runtime_bin"; then
    echo 'tmux-session-sidebar: release refresh failed; using cached runtime' >&2
    printf '%s\n' "$runtime_bin"
    exit 0
  fi
  echo 'tmux-session-sidebar: go not found and no released runtime could be installed' >&2
  exit 1
fi

mkdir -p "$BIN_DIR"
fingerprint="$(source_fingerprint)"

if [ ! -x "$runtime_bin" ] || [ ! -f "$stamp_file" ] || [ "$(cat "$stamp_file" 2>/dev/null || true)" != "$fingerprint" ]; then
  echo "tmux-session-sidebar: building runtime at $runtime_bin" >&2
  (cd "$PLUGIN_DIR" && "$GO_BIN" build -o "$runtime_bin" ./cmd/tmux-session-sidebar)
  validate_runtime "$runtime_bin"
  printf '%s\n' "$fingerprint" >"$stamp_file"
fi

printf '%s\n' "$runtime_bin"
