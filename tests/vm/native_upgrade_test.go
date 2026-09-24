//go:build vmacceptance

package vm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/homebrew"
	"github.com/pHo9UBenaA/brew-warden/internal/application"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// The caller provisions a genuinely older official bottle in a disposable VM.
// No fixture is installed or removed implicitly by this test.
func TestLiveExplicitUpgradeChangesSelectedVersion(t *testing.T) {
	source := os.Getenv("BREWWARDEN_VM_RUNTIME")
	target := os.Getenv("BREWWARDEN_VM_UPGRADE_TARGET")
	if source == "" || target == "" {
		t.Skip("requires a disposable VM, private workspace and older installed target")
	}
	model, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.model").Output()
	if err != nil || !strings.HasPrefix(string(model), "VirtualMac") || !domain.ValidRequest("install", []string{target}) {
		t.Fatal("upgrade requires a disposable VirtualMac and one valid formula name")
	}
	prior, err := filepath.EvalSymlinks(filepath.Join("/opt/homebrew/opt", target))
	if err != nil || filepath.Dir(prior) != filepath.Join("/opt/homebrew/Cellar", target) {
		t.Fatal("requires a linked older bottle", err)
	}
	unrelated := map[string]string{}
	racks, err := os.ReadDir("/opt/homebrew/Cellar")
	if err != nil {
		t.Fatal(err)
	}
	for _, rack := range racks {
		if rack.Name() == target {
			continue
		}
		unrelated[rack.Name()], err = kegSnapshot(filepath.Join("/opt/homebrew/Cellar", rack.Name()))
		if err != nil {
			t.Fatal(err)
		}
	}
	directory, err := os.MkdirTemp(filepath.Dir(source), "explicit-upgrade-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("retained upgrade workspace", directory)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	collector := homebrew.Collector{Runtime: homebrew.Runtime{}, Directory: directory, BottleVerifier: installedBottleVerifier(t)}
	collection, err := collector.Collect(ctx, ports.Request{Operation: "upgrade", Targets: []string{target}}, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	wanted := ""
	for _, node := range collection.Evidence() {
		if node.Artifact.Name != target {
			continue
		}
		wanted = node.Artifact.Version
		if node.Artifact.Revision > 0 {
			wanted += "_" + strconv.Itoa(node.Artifact.Revision)
		}
	}
	if wanted == "" || filepath.Base(prior) == wanted {
		t.Fatal("fixture is not an older installed version", prior, wanted)
	}
	prepared, session, err := collection.Prepare(ctx, domain.DefaultPolicy(), nil, time.Now().Unix(), homebrew.Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	result, err := application.Execute(ctx, prepared, session, nativeClock{})
	if err != nil || result.Outcome != domain.AttemptSucceeded {
		t.Fatal("bound explicit upgrade failed", result, err)
	}
	after, err := filepath.EvalSymlinks(filepath.Join("/opt/homebrew/opt", target))
	if err != nil || after != filepath.Join("/opt/homebrew/Cellar", target, wanted) {
		t.Fatal("upgrade did not activate the verified candidate", prior, after, wanted, err)
	}
	for name, before := range unrelated {
		after, err := kegSnapshot(filepath.Join("/opt/homebrew/Cellar", name))
		if err != nil || after != before {
			t.Fatal("upgrade changed an unrelated keg", name, err)
		}
	}
	t.Log("verified explicit upgrade", target, filepath.Base(prior), "->", wanted)
}
