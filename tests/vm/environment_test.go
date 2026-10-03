//go:build vmacceptance

package vm

import (
	"os/exec"
	"strings"
	"testing"
)

// Keep this guard before any acceptance fixture can modify a Homebrew prefix.
func requireDisposableMac(t *testing.T) {
	t.Helper()
	model, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.model").Output()
	if err != nil || !strings.HasPrefix(string(model), "VirtualMac") {
		t.Fatalf("requires disposable VirtualMac: model=%q, error=%v", model, err)
	}
}
