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

func TestRefusalHistoryAndBrokenConfigRecovery(t *testing.T) {
	dir := t.TempDir()
	f := localstate.Files{ConfigPath: filepath.Join(dir, "config.json"), StatePath: filepath.Join(dir, "history")}
	var out, diagnostics bytes.Buffer
	if code := cli.RunWithServices([]string{"brew", "install", "wget"}, &out, &diagnostics, f, f); code != 1 || !strings.Contains(diagnostics.String(), "history_record:") {
		t.Fatal(code, &diagnostics)
	}
	if err := os.WriteFile(f.ConfigPath, []byte("invalid configuration"), 0o600); err != nil {
		t.Fatal(err)
	}
	diagnostics.Reset()
	if code := cli.RunWithServices([]string{"history"}, &out, &diagnostics, f, f); code != 0 || !strings.Contains(out.String(), "refused install") {
		t.Fatal(code, &out, &diagnostics)
	}
	if strings.Contains(out.String(), "succeeded") {
		t.Fatal("refusal reported as an installation")
	}
}
