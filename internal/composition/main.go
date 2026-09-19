package composition

import (
	"os"
	"path/filepath"

	"brewwarden/internal/adapters/localstate"
	"brewwarden/internal/cli"
)

// Main wires the diagnostic-only interface; no Homebrew executor is registered.
func Main() {
	configPath := ""
	if base, err := os.UserConfigDir(); err == nil {
		configPath = filepath.Join(base, "brewwarden", "config.json")
	}
	os.Exit(cli.RunWithConfig(os.Args[1:], os.Stdout, os.Stderr, localstate.Files{ConfigPath: configPath}))
}
