package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/localstate"
	"github.com/pHo9UBenaA/brew-warden/internal/cli"
)

func TestDiagnosticOnlyBuildNeverCreatesExecutionHistory(t *testing.T) {
	dir := t.TempDir()
	f := localstate.Files{ConfigPath: filepath.Join(dir, "config.json")}
	var out, diagnostics bytes.Buffer
	if code := cli.RunWithConfig([]string{"brew", "install", "wget"}, &out, &diagnostics, f); code == 0 || !strings.Contains(diagnostics.String(), "runtime_unavailable") {
		t.Fatal("unsupported build executed a mutation", code, diagnostics.String())
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatal("diagnostic-only refusal created product state", entries, err)
	}
	for _, command := range []string{"history", "status", "reconcile"} {
		diagnostics.Reset()
		if code := cli.RunWithConfig([]string{command}, &out, &diagnostics, f); code == 0 {
			t.Fatal("obsolete saved-state command accepted", command)
		}
	}
}
