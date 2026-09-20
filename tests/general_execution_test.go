package tests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/attestation"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/githubrelease"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/homebrew"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/localstate"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/osv"
	"github.com/pHo9UBenaA/brew-warden/internal/application"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// This acceptance case takes explicit formula names, never changes the host,
// and verifies both a new installation and a fully checked unchanged rerun.
func TestLiveGeneralBottleExecution(t *testing.T) {
	source := os.Getenv("BREWWARDEN_VM_RUNTIME")
	targets := strings.Fields(os.Getenv("BREWWARDEN_VM_GENERAL_TARGETS"))
	if source == "" || len(targets) == 0 {
		t.Skip("requires disposable VM and explicit targets")
	}
	model, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.model").Output()
	if err != nil || !strings.HasPrefix(string(model), "VirtualMac") {
		t.Fatal("requires disposable macOS VM")
	}
	if !domain.ValidRequest("install", targets) {
		t.Fatal("invalid acceptance targets")
	}
	raw, err := os.ReadFile(filepath.Join(source, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	directory, err := os.MkdirTemp(filepath.Dir(source), "general-execution-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("retained execution", directory)
	before := map[string]string{}
	racks, err := os.ReadDir("/opt/homebrew/Cellar")
	if err != nil {
		t.Fatal(err)
	}
	for _, rack := range racks {
		snapshot, err := kegSnapshot(filepath.Join("/opt/homebrew/Cellar", rack.Name()))
		if err != nil {
			t.Fatal(err)
		}
		before[rack.Name()] = snapshot
	}
	collector := &homebrew.Collector{Runtime: homebrew.Runtime{Root: source, ManifestSHA256: domain.Digest(hex.EncodeToString(sum[:]))}, Directory: directory, Publication: githubrelease.New(), Vulnerabilities: osv.New(), Verifier: func(path string, digest domain.Digest) ports.ProvenanceVerifier {
		return attestation.Verifier{Path: path, SHA256: digest}
	}}
	journal := localstate.Journal{Path: filepath.Join(directory, "attempts")}
	log, err := os.Create(filepath.Join(directory, "native.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	engine := homebrew.Engine{Collector: collector, Streams: homebrew.Streams{Out: io.MultiWriter(os.Stdout, log), Err: io.MultiWriter(os.Stderr, log)}}
	planned := map[string]bool{}
	service := application.Service{Planner: engine, Journal: journal, Clock: nativeClock{}, Present: func(p ports.Prepared) error {
		for _, node := range p.Assessment.Nodes {
			planned[node.Artifact.Name] = true
			t.Log("candidate", node.Artifact.Name, node.Artifact.Version, node.Artifact.SHA256)
		}
		return nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	result, err := service.Run(ctx, ports.Request{Operation: "install", Targets: targets}, domain.DefaultPolicy(), nil)
	if err != nil || result.Outcome != domain.AttemptSucceeded {
		t.Fatalf("installation: %+v %v", result, err)
	}
	racks, err = os.ReadDir("/opt/homebrew/Cellar")
	if err != nil {
		t.Fatal(err)
	}
	for _, rack := range racks {
		if planned[rack.Name()] {
			continue
		}
		after, err := kegSnapshot(filepath.Join("/opt/homebrew/Cellar", rack.Name()))
		if err != nil || before[rack.Name()] != after {
			t.Fatal("unplanned package changed", rack.Name(), err)
		}
	}
	for name := range before {
		if planned[name] {
			continue
		}
		if _, err := os.Stat(filepath.Join("/opt/homebrew/Cellar", name)); err != nil {
			t.Fatal("unplanned package removed", name, err)
		}
	}
	installed, err := kegSnapshot("/opt/homebrew/Cellar")
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.Run(ctx, ports.Request{Operation: "install", Targets: targets}, domain.DefaultPolicy(), nil)
	if err != nil || result.Outcome != domain.AttemptSucceeded {
		t.Fatalf("unchanged rerun: %+v %v", result, err)
	}
	after, err := kegSnapshot("/opt/homebrew/Cellar")
	if err != nil || installed != after {
		t.Fatal("unchanged rerun modified installed payload", err)
	}
	records, err := journal.Attempts()
	if err != nil || len(records) != 2 {
		t.Fatal(records, err)
	}
	for _, record := range records {
		if record.Finish.Outcome != domain.AttemptSucceeded || !record.Finish.AfterState.Valid() {
			t.Fatal(record)
		}
	}
	t.Log("verified complete closure, unchanged unrelated packages and idempotent rerun", targets)
}
