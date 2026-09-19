package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"brewwarden/internal/domain"
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
			if stdout.Len() != 0 || stderr.Len() == 0 || strings.ContainsRune(stderr.String(), '\x1b') {
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
	stdout.Reset()
	if Run([]string{"--version"}, &stdout, &stderr) != 0 || !strings.HasPrefix(stdout.String(), "BrewWarden ") {
		t.Fatal("missing local version")
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

type configSource struct{}

func (configSource) LoadConfig(string) (domain.Policy, error) { return domain.NewPolicy(3600) }

func TestWrapperOptions(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		seconds string
	}{
		{[]string{"doctor"}, "3600"},
		{[]string{"--minimum-release-age", "168h", "doctor"}, "604800"},
		{[]string{"--minimum-release-age", "0h", "doctor"}, "0"},
	} {
		var out, diagnostics bytes.Buffer
		if RunWithConfig(tc.args, &out, &diagnostics, configSource{}) != 1 || !strings.Contains(diagnostics.String(), "minimum_release_age_seconds: "+tc.seconds+"\n") {
			t.Fatalf("unexpected diagnostic: %s", &diagnostics)
		}
	}
	for _, args := range [][]string{{"--minimum-release-age", "-1h", "doctor"}, {"--minimum-release-age", "0.5s", "doctor"}, {"--minimum-release-age", "1h", "--minimum-release-age", "2h", "doctor"}, {"--config", "", "doctor"}} {
		var out, diagnostics bytes.Buffer
		if RunWithConfig(args, &out, &diagnostics, configSource{}) == 0 || !strings.HasPrefix(diagnostics.String(), "invocation_invalid:") {
			t.Fatal("invalid wrapper options accepted")
		}
	}
	_, age, rest, err := options([]string{"brew", "install", "--minimum-release-age", "0h"})
	if err != nil || age != nil || len(rest) != 4 {
		t.Fatal("consumed child arguments")
	}
}
