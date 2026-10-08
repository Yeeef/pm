// Package buildinfo names the pm build: the release build sets Version with
// -ldflags "-X github.com/Yeeef/yeeef-agents/pm/internal/buildinfo.Version=<X>".
package buildinfo

// Version is the pm version this binary is. A build without -X keeps "unset", which no repo pins, so the config check
// refuses every command rather than running as some version.
var Version = "unset"
