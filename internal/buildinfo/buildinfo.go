// Package buildinfo names the pm build: the release build (release/build.sh) sets Version from the release tag
// pm-v<X> with -ldflags "-X github.com/Yeeef/yeeef-agents/pm/internal/buildinfo.Version=<X>". No version is written
// in Go source: a release is a tag on a main commit.
package buildinfo

// Version is the pm version this binary is. An untagged build keeps "dev", which no repo pins, so the config check
// refuses every command in a repo rather than running as some version; pm version prints it.
var Version = "dev"
