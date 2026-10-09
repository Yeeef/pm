#!/bin/sh
# Build Go pm's release tarball for this machine: pm/release/build.sh OUT_DIR [pm-v<X> | <X>]
#
# A release is a tag pm-v<X> on a main commit (pm/AGENTS.md, Releasing pm): the version comes from the tag given, or
# else from the tag on HEAD (git describe --tags --exact-match --match 'pm-v*': lightweight tags too); an untagged build is "dev". No version is
# written in any file. The binary is built as the release workflow and make go-build build it (cgo for gozstd,
# -tags gms_pure_go for Dolt, stripped) with buildinfo.Version stamped, and packed as OUT_DIR/pm-<X>-<os>-<arch>.tar.gz:
# a gzip tar holding the one regular file pm, mode 0755. It prints the tarball's path.
set -eu

[ $# -ge 1 ] && [ $# -le 2 ] || { echo "usage: $0 OUT_DIR [pm-v<X> | <X>]" >&2; exit 2; }
out=$1
root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
if [ $# -eq 2 ]; then
  version=${2#pm-v}
elif tag=$(git -C "$root" describe --tags --exact-match --match 'pm-v*' HEAD 2>/dev/null); then
  version=${tag#pm-v}
else
  version=dev
fi
case $version in
  '' | *[!0-9A-Za-z.+-]*) echo "$0: '$version' is not a pm version" >&2; exit 2 ;;
esac

mkdir -p "$out"
out=$(cd "$out" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
(cd "$root/pm" && CGO_ENABLED=1 go build -trimpath -tags gms_pure_go \
  -ldflags "-s -w -X github.com/Yeeef/yeeef-agents/pm/internal/buildinfo.Version=$version" -o "$work/pm" ./cmd/pm)
chmod 0755 "$work/pm"
tarball="$out/pm-$version-$(cd "$root/pm" && go env GOOS)-$(cd "$root/pm" && go env GOARCH).tar.gz"
# COPYFILE_DISABLE: macOS tar would add an AppleDouble ._pm member beside pm
(cd "$work" && COPYFILE_DISABLE=1 tar -czf "$tarball" pm)
[ "$(tar -tzf "$tarball")" = pm ] || { echo "$0: $tarball holds more than pm" >&2; exit 1; }
echo "$tarball"
