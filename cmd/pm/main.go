// Command pm is Go pm: the project-management CLI, hooks and (later) site and service in one binary.
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
