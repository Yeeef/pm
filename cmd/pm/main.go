// Command pm is Go pm: the project-management CLI, hooks and (later) site and service in one binary.
package main

import (
	"os"

	"github.com/Yeeef/yeeef-agents/pm/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
