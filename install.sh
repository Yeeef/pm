#!/bin/sh
# Install Go pm on this machine: curl -fsSL https://github.com/Yeeef/yeeef-agents/releases/download/pm-v<X>/install.sh | sh
#
# Each release serves its own copy, with its version filled in where @VERSION@ stands (the release workflow does it).
# It picks the release tarball for `uname -s`/`uname -m`, downloads it and the release's SHA256SUMS, checks the
# tarball's sha256, and installs its pm to ${PM_BIN_DIR:-$HOME/.local/bin}/pm by renaming a temp file, so a pm
# running there is never seen half-written. Any failure stops it with nothing installed. $PM_RELEASE_URL replaces
# the release base URL (mirrors, tests).
set -eu

version='@VERSION@'
base=${PM_RELEASE_URL:-https://github.com/Yeeef/yeeef-agents/releases/download}
bindir=${PM_BIN_DIR:-$HOME/.local/bin}

fail() {
  echo "install.sh: error: $*" >&2
  exit 1
}

case $version in @*) fail "this copy names no version; run the one release pm-v<X> serves" ;; esac
while :; do
  case $base in */) base=${base%/} ;; *) break ;; esac
done
url=$base/pm-v$version

os=$(uname -s)
arch=$(uname -m)
# an x86_64 shell under Rosetta on Apple silicon: the machine runs the arm64 binary natively
if [ "$os-$arch" = Darwin-x86_64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = 1 ]; then
  arch=arm64
fi
case $os-$arch in
  Darwin-arm64) platform=darwin-arm64 ;;
  Linux-x86_64 | Linux-amd64) platform=linux-amd64 ;;
  *) fail "release pm-v$version has no binary for $os $arch (only darwin-arm64 and linux-amd64): $url" ;;
esac
asset=pm-$version-$platform.tar.gz

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL --connect-timeout 10 --max-time 300 -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -T 10 -O "$2" "$1"; }
else
  fail "neither curl nor wget is installed to download $url"
fi
if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
else
  fail "neither sha256sum nor shasum is installed to check $asset"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fetch "$url/SHA256SUMS" "$tmp/SHA256SUMS" || fail "could not download $url/SHA256SUMS"
want=$(awk -v asset="$asset" 'NF == 2 && $2 == asset { print $1; exit }' "$tmp/SHA256SUMS")
case $want in
  *[!0-9a-f]* | '') fail "$url/SHA256SUMS has no line for $asset" ;;
esac
[ ${#want} -eq 64 ] || fail "$url/SHA256SUMS has no line for $asset"
fetch "$url/$asset" "$tmp/$asset" || fail "could not download $url/$asset"
got=$(sha256 "$tmp/$asset")
[ "$got" = "$want" ] || fail "$url/$asset has sha256 $got, but SHA256SUMS says $want; nothing was installed"
mkdir "$tmp/x"
tar -xzf "$tmp/$asset" -C "$tmp/x" pm 2>/dev/null && [ -f "$tmp/x/pm" ] && [ ! -L "$tmp/x/pm" ] ||
  fail "$url/$asset is not a gzip tar holding pm"

mkdir -p "$bindir"
new="$bindir/.pm.install.$$"
cp "$tmp/x/pm" "$new" && chmod 0755 "$new" && mv -f "$new" "$bindir/pm" || {
  rm -f "$new"
  fail "could not install $bindir/pm"
}

echo "installed pm $version at $bindir/pm (from $url/$asset, sha256 $got)"
case :$PATH: in
  *":$bindir:"*) ;;
  *) echo "$bindir is not on PATH: add it, e.g. export PATH=\"$bindir:\$PATH\" in your shell's profile" ;;
esac
echo "next, set up each clone that uses pm: run pm init in it"
