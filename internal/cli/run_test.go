package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRejectUnverifiedOperations(t *testing.T) {
	for _, args := range [][]string{
		nil, {"brew", "install", "wget"}, {"brew", "upgrade"},
		{"brew", "install", "--help"}, {"brew", "install", "--dry-run", "wget"},
		{"brew", "reinstall", "wget"}, {"brew", "bundle", "exec", "--install", "sh"},
		{"--minimum-release-age", "0h", "brew", "install", "wget"},
		{"--force", "brew", "upgrade"}, {"--help", "brew", "install", "wget"},
		{"brew", "info", "wget"}, {"/usr/local/bin/brew", "install", "wget"},
		{"brew", "install", "\x1b[2J"}, {"doctor", "brew", "upgrade"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(args, &stdout, &stderr); code == 0 {
				t.Fatal("unverified invocation succeeded")
			}
			if stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), "execution_binding_unverified:") || strings.ContainsRune(stderr.String(), '\x1b') {
				t.Fatalf("unexpected output: stdout=%q stderr=%q", &stdout, &stderr)
			}
		})
	}
}

func TestLocalDiagnostics(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if Run([]string{"--help"}, &stdout, &stderr) != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
		t.Fatal("help must succeed locally")
	}
	stdout.Reset()
	if Run([]string{"doctor"}, &stdout, &stderr) == 0 || !strings.Contains(stderr.String(), "No live Homebrew checks were run") || stdout.Len() != 0 {
		t.Fatal("doctor must report an unavailable execution capability")
	}
	if Run([]string{"--help"}, failedWriter{}, &stderr) == 0 {
		t.Fatal("failed output must not succeed")
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }
