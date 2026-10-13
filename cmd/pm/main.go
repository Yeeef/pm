// Command pm is Go pm: the project-management CLI, its hooks, site and service in one binary. main runs the launcher
// (internal/launch) before anything else, so the installed pm runs each repo's pinned version; the module root's
// package pm (assets.go) embeds prime.md, style.css, prompts/ and the release notes.
package main

import (
	"fmt"
	"os"

	"github.com/Yeeef/pm/internal/cli"
	"github.com/Yeeef/pm/internal/launch"
)

func main() {
	// the launcher runs the repo's pinned version: exec'd, unless it is this one
	if err := launch.Launch(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
	os.Exit(cli.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
