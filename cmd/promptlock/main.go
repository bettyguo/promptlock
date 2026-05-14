package main

import (
	"os"

	"github.com/promptlock/promptlock/internal/cli"
)

// Version is set at build time via -ldflags "-X main.Version=...".
var Version = "0.1.0-dev"

func main() {
	os.Exit(cli.Run(Version, os.Args[1:], os.Stdout, os.Stderr))
}
