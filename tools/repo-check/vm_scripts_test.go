package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the VM driver without Tart, Homebrew, or the host's credentials.
func vmScriptFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".cache", "tart"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"env.sh", "macos-vm-acceptance.sh"} {
		raw, err := os.ReadFile(filepath.Join("../../scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "scripts", name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{
		"reviewed/bin/brew":                    "#!/bin/sh\n",
		"reviewed/Library/Homebrew/test.rb":    "# isolated fixture\n",
		"gh":                                   "#!/bin/sh\n",
		"archive.tar.gz":                       "not used before VM setup fails\n",
		".cache/tart/guest/credential-storage": "private-files-v1\n",
		".cache/tart/guest/home/.config/gh/hosts.yml":      "disposable-fake-token\n",
		".cache/tart/guest/home/.local/state/gh/state.yml": "disposable-state\n",
		".cache/tart/guest/home/device.log":                "old private login log\n",
		".cache/tart/guest/home/device.pid":                "99999999\n",
		".cache/tart/guest/login-output": "! First copy your one-time code: ABCD-1234\n" +
			"Open this URL to continue in your web browser: https://github.com/login/device\n" +
			"secret-marker must not be displayed\n",
		".cache/tart/guest/gh/gh": `#!/bin/sh
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
case "$1:$2" in
  auth:status) exit 1 ;;
  auth:logout)
    test ! -e "$root/../deny-logout" || exit 17
    exit 0 ;;
  auth:login)
    test "$7:$8" = --web:--insecure-storage || exit 64
    /bin/cat "$root/login-output" ;;
  *) exit 64 ;;
esac
`,
		"fake-tart": `#!/bin/sh
printf '%s:%s\n' "$1" "${2:-}" >> "$TART_HOME/actions"
case "$1" in
  clone) test ! -e "$TART_HOME/deny-clone" ;;
  set) exit 17 ;;
  exec)
    if test -e "$TART_HOME/guest-reachable"; then
      case "$3" in
        /usr/bin/id) exit 0 ;;
        /usr/sbin/sysctl) printf 'VirtualMac2,1\n'; exit 0 ;;
        /usr/bin/arch) printf 'arm64\n'; exit 0 ;;
        /usr/bin/grep)
          exec /usr/bin/grep -Fxq private-files-v1 "$TART_HOME/guest/credential-storage" ;;
        /usr/bin/env) exit 1 ;; # Reachable guest, failed gh auth-status boundary.
        /bin/sh)
          case "${5:-}" in
            *'rm -rf'*) test ! -e "$TART_HOME/deny-rm" || exit 23 ;;
          esac
          # Run the actual bounded guest command on disposable local files.
          # No command may reach the real /private/tmp guest path or host auth.
          program=$(printf '%s' "$5" | /usr/bin/sed "s|/private/tmp/bw-acceptance|$TART_HOME/guest|g")
          exec /bin/sh -c "$program" ;;
      esac
    fi
    exit 29 ;;
  stop) test ! -e "$TART_HOME/deny-stop" ;;
  *) exit 21 ;;
esac
`,
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("git", "init", "-q")
	command.Dir = root
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initialize isolated repository: %v: %s", err, out)
	}
	command = exec.Command("git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.org", "commit", "-q", "--allow-empty", "-m", "fixture")
	command.Dir = root
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("commit isolated fixture: %v: %s", err, out)
	}
	return root, filepath.Join(root, "fake-tart")
}

func runVMScriptFixture(t *testing.T, root, tart string, args ...string) (string, error) {
	t.Helper()
	command := exec.Command("/bin/sh", append([]string{filepath.Join(root, "scripts", "macos-vm-acceptance.sh")}, args...)...)
	command.Dir = root
	command.Env = append(os.Environ(), "BREWWARDEN_VM_TART="+tart)
	out, err := command.CombinedOutput()
	return string(out), err
}

func vmActions(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".cache", "tart", "actions"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestVMPrepareStopsOwnedCloneAfterSetupFailure(t *testing.T) {
	root, tart := vmScriptFixture(t)
	out, err := runVMScriptFixture(t, root, tart, "prepare", "base", "new-vm", filepath.Join(root, "reviewed"), filepath.Join(root, "gh"), filepath.Join(root, "archive.tar.gz"))
	if err == nil || !strings.Contains(out, "VM setup failed") {
		t.Fatalf("expected setup failure and cleanup diagnostic: %v: %s", err, out)
	}
	actions := vmActions(t, root)
	if !strings.Contains(actions, "clone:base\nset:new-vm\n") || !strings.Contains(actions, "stop:new-vm\n") || strings.Contains(actions, "stop:base\n") {
		t.Fatalf("failed setup did not stop only its new clone: %s", actions)
	}
}

func TestVMPrepareNeverStopsCloneItDidNotCreate(t *testing.T) {
	root, tart := vmScriptFixture(t)
	if err := os.MkdirAll(filepath.Join(root, ".cache", "tart"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cache", "tart", "deny-clone"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := runVMScriptFixture(t, root, tart, "prepare", "base", "preexisting", filepath.Join(root, "reviewed"), filepath.Join(root, "gh"), filepath.Join(root, "archive.tar.gz"))
	if err == nil {
		t.Fatal("failed clone was accepted")
	}
	if actions := vmActions(t, root); strings.Contains(actions, "stop:") {
		t.Fatalf("attempted to stop unowned VM: %s", actions)
	}
}

func TestVMPrepareReportsUnconfirmedCleanup(t *testing.T) {
	root, tart := vmScriptFixture(t)
	if err := os.MkdirAll(filepath.Join(root, ".cache", "tart"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cache", "tart", "deny-stop"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runVMScriptFixture(t, root, tart, "prepare", "base", "new-vm", filepath.Join(root, "reviewed"), filepath.Join(root, "gh"), filepath.Join(root, "archive.tar.gz"))
	if err == nil || !strings.Contains(out, "cleanup unconfirmed") {
		t.Fatalf("lost setup failure or concealed incomplete cleanup: %v: %s", err, out)
	}
	if actions := vmActions(t, root); !strings.Contains(actions, "stop:new-vm\n") {
		t.Fatalf("did not attempt to stop owned clone: %s", actions)
	}
}

func TestProductReadyRefusesMissingApprovalAndCancelsOnlyItsPendingVM(t *testing.T) {
	root, tart := vmScriptFixture(t)
	raw, err := os.ReadFile("../../scripts/product-ready.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "product-ready.sh"), raw, 0700); err != nil {
		t.Fatal(err)
	}
	trustedTart := filepath.Join(root, ".cache", "vm-tools", "tart-2.37.0", "tart.app", "Contents", "MacOS", "tart")
	if err := os.MkdirAll(filepath.Dir(trustedTart), 0700); err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(tart)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trustedTart, binary, 0700); err != nil {
		t.Fatal(err)
	}
	run := func(operation, vm string) (string, error) {
		t.Helper()
		command := exec.Command("/bin/sh", filepath.Join(root, "scripts", "product-ready.sh"), operation, vm)
		command.Dir = root
		out, err := command.CombinedOutput()
		return string(out), err
	}
	out, err := run("cancel", "unowned")
	if err == nil || !strings.Contains(out, "No pending readiness run") {
		t.Fatalf("cancel accepted an unowned VM: %v: %s", err, out)
	}
	state := filepath.Join(root, ".cache", "product-ready.pending", "state")
	if err := os.MkdirAll(filepath.Dir(state), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, []byte("awaiting-device-approval\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cache", "tart", "guest-reachable"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	out, err = run("complete", "pending")
	if err == nil || !strings.Contains(out, "Guest gh is not authenticated") {
		t.Fatalf("unapproved guest passed the readiness gate: %v: %s", err, out)
	}
	if actions := vmActions(t, root); strings.Contains(actions, "stop:") {
		t.Fatalf("pending approval stopped unrelated VM unexpectedly: %s", actions)
	}
	out, err = run("cancel", "pending")
	if err == nil || !strings.Contains(out, "NOT product-ready") || !strings.Contains(vmActions(t, root), "stop:pending\n") {
		t.Fatalf("cancellation did not stop the pending clone: %v: %s", err, out)
	}
	final, err := os.ReadFile(state)
	if err != nil || string(final) != "failed\n" {
		t.Fatalf("cancellation left a resumable readiness state: %q: %v", final, err)
	}
}

func TestVMFinishRemovesGuestCredentialsAndStops(t *testing.T) {
	root, tart := vmScriptFixture(t)
	if err := os.WriteFile(filepath.Join(root, ".cache", "tart", "guest-reachable"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runVMScriptFixture(t, root, tart, "finish", "new-vm")
	if err != nil || !strings.Contains(out, "Guest credentials removed and VM stopped") {
		t.Fatalf("normal cleanup failed: %v: %s", err, out)
	}
	if actions := vmActions(t, root); !strings.Contains(actions, "stop:new-vm\n") {
		t.Fatalf("normal cleanup left guest running: %s", actions)
	}
	for _, name := range []string{".config/gh/hosts.yml", ".local/state/gh/state.yml", "device.log", "device.pid"} {
		if _, err := os.Lstat(filepath.Join(root, ".cache/tart/guest/home", name)); !os.IsNotExist(err) {
			t.Fatalf("guest credential file %s remains: %v", name, err)
		}
	}
}

func TestVMFinishStopsAfterGuestCredentialCleanupFails(t *testing.T) {
	for _, failure := range []string{"deny-rm", "deny-logout", "legacy-storage"} {
		t.Run(failure, func(t *testing.T) {
			root, tart := vmScriptFixture(t)
			for _, marker := range []string{"guest-reachable", failure} {
				if err := os.WriteFile(filepath.Join(root, ".cache/tart", marker), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "legacy-storage" {
				if err := os.Remove(filepath.Join(root, ".cache/tart/guest/credential-storage")); err != nil {
					t.Fatal(err)
				}
			}
			out, err := runVMScriptFixture(t, root, tart, "finish", "new-vm")
			if err == nil || !strings.Contains(out, "credential cleanup unconfirmed") || strings.Contains(out, "Guest credentials removed") {
				t.Fatalf("cleanup failure was certified: %v: %s", err, out)
			}
			if !strings.Contains(vmActions(t, root), "stop:new-vm\n") {
				t.Fatal("failed cleanup did not stop guest")
			}
			if failure == "deny-logout" {
				if _, err := os.Stat(filepath.Join(root, ".cache/tart/guest/home/.config/gh/hosts.yml")); !os.IsNotExist(err) {
					t.Fatal("logout failure skipped file cleanup", err)
				}
			}
		})
	}
}

func TestVMFinishAcceptsUnauthenticatedGuest(t *testing.T) {
	for _, state := range []string{"new guest", "empty hosts file"} {
		t.Run(state, func(t *testing.T) {
			root, tart := vmScriptFixture(t)
			if state == "new guest" {
				if err := os.RemoveAll(filepath.Join(root, ".cache/tart/guest")); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(filepath.Join(root, ".cache/tart/guest/home/.config/gh/hosts.yml"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				// No credential exists to log out; that command would fail.
				if err := os.WriteFile(filepath.Join(root, ".cache/tart/deny-logout"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, ".cache/tart/guest-reachable"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := runVMScriptFixture(t, root, tart, "finish", "new-vm"); err != nil {
				t.Fatalf("unauthenticated guest cleanup: %s: %v", out, err)
			}
		})
	}
}

func TestVMFinishStopsWhenGuestCredentialsCannotBeRemoved(t *testing.T) {
	root, tart := vmScriptFixture(t)
	out, err := runVMScriptFixture(t, root, tart, "finish", "new-vm")
	if err == nil || !strings.Contains(out, "credential cleanup unconfirmed") {
		t.Fatalf("unreachable guest cleanup was reported as success: %v: %s", err, out)
	}
	if actions := vmActions(t, root); !strings.Contains(actions, "stop:new-vm\n") {
		t.Fatalf("unreachable guest was left running: %s", actions)
	}
}

func TestVMAuthDisplaysEarlierGHDeviceCodeWording(t *testing.T) {
	root, tart := vmScriptFixture(t)
	if err := os.WriteFile(filepath.Join(root, ".cache", "tart", "guest-reachable"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runVMScriptFixture(t, root, tart, "auth", "new-vm")
	if err != nil || !strings.Contains(out, "ABCD-1234") || !strings.Contains(out, "https://github.com/login/device") || strings.Contains(out, "secret-marker") {
		t.Fatalf("older gh device login code was hidden: %v: %s", err, out)
	}
}

func TestVMAuthStatusRefusesUnauthenticatedGuest(t *testing.T) {
	root, tart := vmScriptFixture(t)
	if err := os.WriteFile(filepath.Join(root, ".cache/tart/guest-reachable"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runVMScriptFixture(t, root, tart, "auth-status", "new-vm")
	if err == nil || !strings.Contains(out, "Guest gh is not authenticated") {
		t.Fatalf("unauthenticated guest was accepted: %v: %s", err, out)
	}
}
