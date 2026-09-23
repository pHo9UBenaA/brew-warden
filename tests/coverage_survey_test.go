package tests

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/homebrew"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// A completed survey reports eligibility and holds separately; it is not a
// successful-install test and never turns unavailable evidence into eligibility.
func TestLivePublicCoverageSurvey(t *testing.T) {
	source := os.Getenv("BREWWARDEN_VM_PUBLIC_RUNTIME")
	names := strings.Fields(os.Getenv("BREWWARDEN_VM_SURVEY_TARGETS"))
	if source == "" || len(names) == 0 {
		t.Skip("requires explicit VM coverage survey")
	}
	model, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.model").Output()
	if err != nil || !strings.HasPrefix(string(model), "VirtualMac") {
		t.Fatal("requires disposable VM")
	}
	directory, err := os.MkdirTemp(filepath.Dir(source), "coverage-survey-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("retained survey", directory)
	collector := homebrew.Collector{Runtime: homebrew.Runtime{}, Directory: directory, BottleVerifier: installedBottleVerifier(t)}
	type observation struct {
		Name, Result string
		Candidates   []domain.Node
	}
	results := []observation{}
	for _, name := range names {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		collection, err := collector.Collect(ctx, ports.Request{Operation: "install", Targets: []string{name}}, time.Now().Unix())
		cancel()
		record := observation{Name: name}
		if err != nil {
			record.Result = "held: " + err.Error()
		} else {
			record.Candidates = collection.Evidence()
			record.Result = "evidence collected"
			for _, node := range record.Candidates {
				for _, evidence := range node.Evidence {
					if evidence.Status != domain.Verified || evidence.Claim == domain.Vulnerabilities && evidence.Applicability != domain.NoKnownApplicableFindings || evidence.Claim == domain.Publication && time.Now().Unix()-evidence.PublishedAt < domain.DefaultPolicy().MinimumAgeSeconds() {
						record.Result = "held by evidence or age policy"
					}
				}
			}
		}
		t.Log(name, record.Result)
		results = append(results, record)
		raw, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "survey.json"), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
