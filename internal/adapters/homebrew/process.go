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
	"slices"
	"time"
)

type workspace struct{ root string }

func (w workspace) environment() []string {
	return []string{"HOME=" + filepath.Join(w.root, "home"), "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "TMPDIR=" + filepath.Join(w.root, "tmp"), "XDG_CONFIG_HOME=" + filepath.Join(w.root, "home/config"), "HOMEBREW_CACHE=" + filepath.Join(w.root, "cache"), "HOMEBREW_LOGS=" + filepath.Join(w.root, "logs"), "HOMEBREW_TEMP=" + filepath.Join(w.root, "tmp"), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_ENV_HINTS=1", "HOMEBREW_NO_COLOR=1", "HOMEBREW_NO_INSTALL_CLEANUP=1", "HOMEBREW_NO_AUTOREMOVE=1", "HOMEBREW_NO_BOOTSNAP=1", "TZ=UTC"}
}
func (w workspace) initialize() error {
	for _, name := range []string{"home", "tmp", "cache", "logs", "inputs", "observations", "states"} {
		if err := os.Mkdir(filepath.Join(w.root, name), 0700); err != nil {
			return err
		}
	}
	return nil
}
func (w workspace) sandbox(name string, network bool, mutablePrefix bool, immutable []string) (string, error) {
	quote := func(value string) string { raw, _ := json.Marshal(value); return string(raw) }
	profile := "(version 1)\n(allow default)\n(deny file-write*)\n(allow file-write* (subpath " + quote(w.root) + ") (literal \"/dev/null\"))\n"
	if !network {
		profile += "(deny network*)\n"
	}
	if mutablePrefix {
		profile += "(allow file-write* (subpath \"/opt/homebrew\"))\n(deny file-write* (subpath \"/opt/homebrew/Library\") (subpath \"/opt/homebrew/.git\") (literal \"/opt/homebrew/bin/brew\"))\n"
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

// invoke also supports ordinary Homebrew commands, without loading a Ruby bridge.
func (w workspace) invoke(ctx context.Context, label, profile string, args ...string) ([]byte, error) {
	return w.invokeMode(ctx, label, profile, false, args...)
}

func (w workspace) invokeAPI(ctx context.Context, label, profile string, args ...string) ([]byte, error) {
	return w.invokeMode(ctx, label, profile, true, args...)
}

func (w workspace) invokeMode(ctx context.Context, label, profile string, api bool, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	command := w.command(ctx, profile, args...)
	if api {
		command.Env = slices.DeleteFunc(command.Env, func(value string) bool { return value == "HOMEBREW_NO_INSTALL_FROM_API=1" })
	}
	stdout, stderr := &processOutput{}, &processOutput{}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	// Retain bounded diagnostics even on failure; callers emit fixed messages.
	if writeErr := writeNew(filepath.Join(w.root, label+".stderr"), stderr.Bytes(), 0600); writeErr != nil {
		return nil, writeErr
	}
	if err != nil || stdout.overflow || stderr.overflow {
		return nil, fmt.Errorf("homebrew %s failed", label)
	}
	return stdout.Bytes(), nil
}

type processOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *processOutput) Write(p []byte) (int, error) {
	if len(p) > maxManifest-b.Len() {
		b.overflow = true
		return 0, errors.New("native output exceeds limit")
	}
	return b.Buffer.Write(p)
}
