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
	statePath := ""
	if configPath != "" {
		statePath = filepath.Join(filepath.Dir(configPath), "history")
	}
	files := localstate.Files{ConfigPath: configPath, StatePath: statePath}
	os.Exit(cli.RunWithServices(os.Args[1:], os.Stdout, os.Stderr, files, files))
}
