#!/bin/sh
# Install Go pm on this machine: curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v<X>/install.sh | sh
#
# Each release serves its own copy, with its version filled in where @VERSION@ stands (the release workflow does it).
# It picks the release tarball for `uname -s`/`uname -m`, downloads it and the release's SHA256SUMS, checks the
# tarball's sha256, and installs its pm to ${PM_BIN_DIR:-$HOME/.local/bin}/pm by renaming a temp file, so a pm
# running there is never seen half-written. Any failure stops it with nothing installed. Yeeef/pm is public, so it
# downloads with no token from <url>/pm-v<X>/<asset> (url: $PM_RELEASE_URL, a mirror or tests, else GitHub's release
# downloads). When that fails, and curl and a token are at hand, from $GH_TOKEN, else from `gh auth token`, it
# downloads again through the GitHub API ($PM_RELEASE_API replaces its URL, for tests), as a private copy of the repo
# needs; the token goes to the API alone, never to the storage host an asset redirects to.
set -eu

version='@VERSION@'
base=${PM_RELEASE_URL:-https://github.com/Yeeef/pm/releases/download}
api=${PM_RELEASE_API:-https://api.github.com/repos/Yeeef/pm}
bindir=${PM_BIN_DIR:-$HOME/.local/bin}

fail() {
  echo "install.sh: error: $*" >&2
  exit 1
}

case $version in @*) fail "this copy names no version; run the one release pm-v<X> serves" ;; esac
while :; do
  case $base in */) base=${base%/} ;; *) break ;; esac
done
while :; do
  case $api in */) api=${api%/} ;; *) break ;; esac
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

# fetch URL FILE [ACCEPT TOKEN]: the body of GET URL into FILE; with a token, sent to URL's host alone (curl 7.58 and
# later drop an Authorization header on a redirect to another host) and given on stdin, so no process list shows it.
# What curl or wget says goes to $tmp/err, for reason.
if command -v curl >/dev/null 2>&1; then
  fetch() {
    if [ $# -eq 4 ]; then
      printf 'header = "Authorization: Bearer %s"\n' "$4" |
        curl --config - -fsSL --connect-timeout 10 --max-time 300 -H "Accept: $3" -o "$2" "$1" 2>"$tmp/err"
    else
      curl -fsSL --connect-timeout 10 --max-time 300 -o "$2" "$1" 2>"$tmp/err"
    fi
  }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -T 10 -O "$2" "$1" 2>"$tmp/err"; }
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
# reason: ": <the last line curl or wget said>" of the last fetch, or nothing
reason() {
  r=$(tail -n 1 "$tmp/err" 2>/dev/null) || r=''
  if [ -n "$r" ]; then echo ": $r"; fi
}

# get SUMS_URL TAR_URL [ACCEPT TOKEN]: SHA256SUMS and the tarball into $tmp, the tarball checked against its line; says
# what failed and returns 2 for a download that failed, which the token path may try again, and fails for a check
get() {
  sums_url=$1 tar_url=$2
  shift 2
  fetch "$sums_url" "$tmp/SHA256SUMS" "$@" || { why="could not download $sums_url$(reason)"; return 2; }
  want=$(awk -v asset="$asset" 'NF == 2 && $2 == asset { print $1; exit }' "$tmp/SHA256SUMS")
  case $want in
    *[!0-9a-f]* | '') fail "$sums_url has no line for $asset" ;;
  esac
  [ ${#want} -eq 64 ] || fail "$sums_url has no line for $asset"
  fetch "$tar_url" "$tmp/$asset" "$@" || { why="could not download $tar_url$(reason)"; return 2; }
  got=$(sha256 "$tmp/$asset")
  [ "$got" = "$want" ] || fail "$tar_url has sha256 $got, but SHA256SUMS says $want; nothing was installed"
}

st=0
get "$url/SHA256SUMS" "$url/$asset" || st=$?
if [ "$st" -eq 2 ]; then
  token=${GH_TOKEN:-}
  if [ -z "$token" ]; then token=$(gh auth token 2>/dev/null) || token=''; fi
  [ -n "$token" ] && command -v curl >/dev/null 2>&1 || fail "$why"
  rel=$api/releases/tags/pm-v$version
  fetch "$rel" "$tmp/release.json" application/vnd.github+json "$token" ||
    fail "could not download $rel$(reason)"
  # each asset's API url comes before its name; the url of whatever else the release names is no asset's
  asset_url() {
    grep -o -e '"url": *"[^"]*/releases/assets/[0-9]*"' -e '"name": *"[^"]*"' "$tmp/release.json" |
      awk -v want="$1" '/^"url"/ { u = $0; sub(/^"url": *"/, "", u); sub(/"$/, "", u); next }
        { n = $0; sub(/^"name": *"/, "", n); sub(/"$/, "", n); if (n == want && u != "") { print u; exit } }'
  }
  sums_url=$(asset_url SHA256SUMS)
  [ -n "$sums_url" ] || fail "$rel has no asset SHA256SUMS"
  tar_url=$(asset_url "$asset")
  [ -n "$tar_url" ] || fail "$rel has no asset $asset"
  st=0
  get "$sums_url" "$tar_url" application/octet-stream "$token" || st=$?
  [ "$st" -eq 0 ] || fail "$why"
elif [ "$st" -ne 0 ]; then
  exit "$st"
fi
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
