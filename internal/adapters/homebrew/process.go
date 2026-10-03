package homebrew

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type workspace struct{ root string }

func (w workspace) environment() []string {
	return []string{
		"HOME=" + filepath.Join(w.root, "home"),
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"TMPDIR=" + filepath.Join(w.root, "tmp"),
		"XDG_CONFIG_HOME=" + filepath.Join(w.root, "home/config"),
		"HOMEBREW_CACHE=" + filepath.Join(w.root, "cache"),
		"HOMEBREW_LOGS=" + filepath.Join(w.root, "logs"),
		"HOMEBREW_TEMP=" + filepath.Join(w.root, "tmp"),
		"HOMEBREW_NO_AUTO_UPDATE=1",
		"HOMEBREW_NO_ANALYTICS=1",
		"HOMEBREW_NO_ENV_HINTS=1",
		"HOMEBREW_NO_COLOR=1",
		"HOMEBREW_NO_INSTALL_CLEANUP=1",
		"HOMEBREW_NO_AUTOREMOVE=1",
		"HOMEBREW_NO_BOOTSNAP=1",
		"TZ=UTC",
	}
}
func (w workspace) initialize() error {
	for _, name := range []string{"home", "tmp", "cache", "logs", "inputs", "observations", "states"} {
		if err := os.Mkdir(filepath.Join(w.root, name), 0700); err != nil {
			return err
		}
	}
	return nil
}

type sandboxPermissions struct {
	AllowNetwork      bool
	AllowPrefixWrites bool
}

func (w workspace) sandbox(name string, permissions sandboxPermissions, immutable []string) (string, error) {
	quote := func(value string) string {
		raw, _ := json.Marshal(value)
		return string(raw)
	}
	profile := "(version 1)\n(allow default)\n(deny file-write*)\n(allow file-write* (subpath " + quote(w.root) + ") (literal \"/dev/null\"))\n"
	if !permissions.AllowNetwork {
		profile += "(deny network*)\n"
	}
	if permissions.AllowPrefixWrites {
		profile += "(allow file-write* (subpath \"/opt/homebrew\"))\n" +
			"(deny file-write* (subpath \"/opt/homebrew/Library\") " +
			"(subpath \"/opt/homebrew/.git\") (literal \"/opt/homebrew/bin/brew\"))\n"
	}
	for _, file := range immutable {
		profile += "(deny file-write* (subpath " + quote(file) + "))\n"
	}
	destination := filepath.Join(w.root, name+".sb")
	if err := writeNew(destination, []byte(profile), 0600); err != nil {
		return "", err
	}
	return destination, nil
}
func (w workspace) command(ctx context.Context, profile string, args ...string) *exec.Cmd {
	brew := filepath.Join(w.root, "runtime/brew/bin/brew")
	command := exec.CommandContext(ctx, "/usr/bin/arch", append([]string{"-arm64", "/usr/bin/sandbox-exec", "-f", profile, brew}, args...)...)
	command.Env = w.environment()
	command.Dir = w.root
	command.WaitDelay = 2 * time.Second
	return command
}

// Invoke public Homebrew commands without loading a Ruby bridge.
func (w workspace) invoke(ctx context.Context, label, profile string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return w.invokeCommand(ctx, label, w.command(ctx, profile, args...))
}

// Keep safe classifications after collection cleanup; never expose raw stderr,
// which can contain credentials, terminal controls or user-owned paths.
func (w workspace) invokeCommand(ctx context.Context, label string, command *exec.Cmd) ([]byte, error) {
	stdout, stderr := &processOutput{}, &processOutput{}
	command.Stdout = stdout
	command.Stderr = stderr
	runErr := command.Run()
	var failure error
	switch {
	case ctx.Err() != nil:
		failure = ctx.Err()
	case stdout.overflow || stderr.overflow:
		failure = errors.New("output exceeded limit")
	case runErr != nil:
		var exit *exec.ExitError
		if errors.As(runErr, &exit) {
			failure = fmt.Errorf("command exited with status %d", exit.ExitCode())
		} else {
			failure = errors.New("command could not start or complete; check Homebrew prerequisites")
		}
	}
	// Diagnostics exist only for this pending workspace, not execution history.
	writeErr := writeNew(filepath.Join(w.root, label+".stderr"), stderr.Bytes(), 0600)
	if writeErr != nil {
		failure = errors.Join(failure, errors.New("could not save private command diagnostics"))
	}
	if failure != nil {
		return nil, fmt.Errorf("homebrew %s: %w", label, failure)
	}
	return stdout.Bytes(), nil
}

type processOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

// Do not embed bytes.Buffer: its ReadFrom would bypass Write's bound in io.Copy.
func (b *processOutput) Bytes() []byte  { return b.buffer.Bytes() }
func (b *processOutput) String() string { return b.buffer.String() }

func (b *processOutput) Write(p []byte) (int, error) {
	if len(p) > maxManifest-b.buffer.Len() {
		b.overflow = true
		return 0, errors.New("native output exceeds limit")
	}
	return b.buffer.Write(p)
}
