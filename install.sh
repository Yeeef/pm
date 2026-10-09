#!/bin/sh
# Install Go pm on this machine: gh release download pm-v<X> -R Yeeef/yeeef-agents -p install.sh -O - | sh
#
# Each release serves its own copy, with its version filled in where @VERSION@ stands (the release workflow does it).
# It picks the release tarball for `uname -s`/`uname -m`, downloads it and the release's SHA256SUMS, checks the
# tarball's sha256, and installs its pm to ${PM_BIN_DIR:-$HOME/.local/bin}/pm by renaming a temp file, so a pm
# running there is never seen half-written. Any failure stops it with nothing installed. The repo is private, so it
# downloads through the GitHub API ($PM_RELEASE_API replaces its URL, for tests) with curl and a token from $GH_TOKEN,
# else from `gh auth token`; the token goes to the API alone, never to the storage host an asset redirects to.
# $PM_RELEASE_URL names a mirror instead, <url>/pm-v<X>/<asset>, downloaded with no token.
set -eu

version='@VERSION@'
mirror=${PM_RELEASE_URL:-}
api=${PM_RELEASE_API:-https://api.github.com/repos/Yeeef/yeeef-agents}
bindir=${PM_BIN_DIR:-$HOME/.local/bin}

fail() {
  echo "install.sh: error: $*" >&2
  exit 1
}

case $version in @*) fail "this copy names no version; run the one release pm-v<X> serves" ;; esac
while :; do
  case $mirror in */) mirror=${mirror%/} ;; *) break ;; esac
done
while :; do
  case $api in */) api=${api%/} ;; *) break ;; esac
done
if [ -n "$mirror" ]; then url=$mirror/pm-v$version; else url=$api/releases/tags/pm-v$version; fi

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

if [ -z "$mirror" ]; then
  command -v curl >/dev/null 2>&1 || fail "curl is not installed to download $url"
  token=${GH_TOKEN:-}
  if [ -z "$token" ]; then token=$(gh auth token 2>/dev/null) || token=''; fi
  [ -n "$token" ] || fail "release pm-v$version is downloaded through the GitHub API, which needs a token: set GH_TOKEN, or log in with gh auth login so that gh auth token prints one"
  # curl sends a -H header to the URL's host alone, not to the host a redirect names
  fetch() { curl -fsSL --connect-timeout 10 --max-time 300 -H "Authorization: Bearer $token" -H "Accept: $3" -o "$2" "$1"; }
elif command -v curl >/dev/null 2>&1; then
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
sums_url=$url/SHA256SUMS
tar_url=$url/$asset
if [ -z "$mirror" ]; then
  fetch "$url" "$tmp/release.json" application/vnd.github+json || fail "could not download $url"
  # each asset's API url comes before its name; the url of whatever else the release names is no asset's
  asset_url() {
    grep -o -e '"url": *"[^"]*/releases/assets/[0-9]*"' -e '"name": *"[^"]*"' "$tmp/release.json" |
      awk -v want="$1" '/^"url"/ { u = $0; sub(/^"url": *"/, "", u); sub(/"$/, "", u); next }
        { n = $0; sub(/^"name": *"/, "", n); sub(/"$/, "", n); if (n == want && u != "") { print u; exit } }'
  }
  sums_url=$(asset_url SHA256SUMS)
  [ -n "$sums_url" ] || fail "$url has no asset SHA256SUMS"
  tar_url=$(asset_url "$asset")
  [ -n "$tar_url" ] || fail "$url has no asset $asset"
fi
fetch "$sums_url" "$tmp/SHA256SUMS" application/octet-stream || fail "could not download $sums_url"
want=$(awk -v asset="$asset" 'NF == 2 && $2 == asset { print $1; exit }' "$tmp/SHA256SUMS")
case $want in
  *[!0-9a-f]* | '') fail "$sums_url has no line for $asset" ;;
esac
[ ${#want} -eq 64 ] || fail "$sums_url has no line for $asset"
fetch "$tar_url" "$tmp/$asset" application/octet-stream || fail "could not download $tar_url"
got=$(sha256 "$tmp/$asset")
[ "$got" = "$want" ] || fail "$tar_url has sha256 $got, but SHA256SUMS says $want; nothing was installed"
mkdir "$tmp/x"
tar -xzf "$tmp/$asset" -C "$tmp/x" pm 2>/dev/null && [ -f "$tmp/x/pm" ] && [ ! -L "$tmp/x/pm" ] ||
  fail "$tar_url is not a gzip tar holding pm"

mkdir -p "$bindir"
new="$bindir/.pm.install.$$"
cp "$tmp/x/pm" "$new" && chmod 0755 "$new" && mv -f "$new" "$bindir/pm" || {
  rm -f "$new"
  fail "could not install $bindir/pm"
}

echo "installed pm $version at $bindir/pm (from $tar_url, sha256 $got)"
case :$PATH: in
  *":$bindir:"*) ;;
  *) echo "$bindir is not on PATH: add it, e.g. export PATH=\"$bindir:\$PATH\" in your shell's profile" ;;
esac
echo "next, set up each clone that uses pm: run pm init in it"
