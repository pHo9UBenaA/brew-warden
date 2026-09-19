package composition

import (
	"os"

	"brewwarden/internal/cli"
)

// Main wires the diagnostic-only interface; no Homebrew executor is registered.
func Main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
