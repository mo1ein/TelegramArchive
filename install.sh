#!/bin/sh
set -eu

fail() { printf 'ta install: %s\n' "$*" >&2; exit 1; }

install_dir=${HOME:?HOME is not set}/.local/bin
version=latest
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dir|--version)
      option=$1
      [ "$#" -ge 2 ] && [ -n "$2" ] || fail "$option requires a value"
      case "$option" in --dir) install_dir=$2;; --version) version=$2;; esac
      shift 2;;
    -h|--help)
      printf 'Usage: sh install.sh [--dir DIRECTORY] [--version vX.Y.Z]\nInstalls ta in ~/.local/bin by default.\n'
      exit 0;;
    *) fail "unknown option: $1";;
  esac
done

case "$(uname -s)" in
  Linux) platform=linux;;
  Darwin) platform=darwin;;
  *) fail 'unsupported operating system; download a binary from GitHub Releases';;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64;;
  aarch64|arm64) arch=arm64;;
  *) fail 'unsupported architecture; use go install or build from source';;
esac
for tool in curl tar awk grep install; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"
done
if command -v sha256sum >/dev/null 2>&1; then
  hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  hash_tool=shasum
else
  fail 'sha256sum or shasum is required'
fi

releases=https://github.com/mo1ein/TelegramArchive/releases
if [ "$version" = latest ]; then
  release_url=$(curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 15 --output /dev/null --write-out '%{url_effective}' "$releases/latest") \
    || fail 'cannot find the latest release; check GitHub Releases or install from source'
  version=${release_url##*/}
fi
printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' \
  || fail "invalid version: $version (expected vX.Y.Z)"

archive=ta_${version#v}_${platform}_${arch}.tar.gz
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/ta-install.XXXXXX")
trap 'rm -rf "$tmp_dir"' 0
trap 'exit 1' 1 2 15
download() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --retry 3 --connect-timeout 15 --output "$2" "$1"
}
printf 'Downloading ta %s for %s/%s...\n' "$version" "$platform" "$arch"
download "$releases/download/$version/$archive" "$tmp_dir/$archive" \
  || fail 'binary download failed; check the release version and your connection'
download "$releases/download/$version/checksums.txt" "$tmp_dir/checksums.txt" \
  || fail 'checksum download failed'
expected=$(awk -v name="$archive" '$2 == name {print $1}' "$tmp_dir/checksums.txt")
[ "${#expected}" -eq 64 ] || fail 'missing or invalid checksum'
case "$expected" in *[!0-9a-f]*) fail 'missing or invalid checksum';; esac
if [ "$hash_tool" = sha256sum ]; then
  actual=$(sha256sum "$tmp_dir/$archive" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$tmp_dir/$archive" | awk '{print $1}')
fi
[ "$actual" = "$expected" ] || fail 'checksum mismatch; existing installation was not changed'

tar -xzf "$tmp_dir/$archive" -C "$tmp_dir" ta
mkdir -p "$install_dir"
install -m 755 "$tmp_dir/ta" "$install_dir/ta"
printf 'Installed ta %s to %s/ta\n' "$version" "$install_dir"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) printf 'Add %s to your PATH, then run: ta --help\n' "$install_dir";;
esac
