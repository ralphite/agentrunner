// Command agentrunner is the CLI entry point for the AgentRunner harness.
package main

import (
	"os"

	"github.com/ralphite/agentrunner/internal/cli"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], version, os.Stdout, os.Stderr))
}
