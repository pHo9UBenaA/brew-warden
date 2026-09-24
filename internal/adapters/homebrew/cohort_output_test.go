package homebrew

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// Unmodified public CLI responses captured in disposable native Tahoe guests
// with official release checkouts. The signed snapshot and explicit complete
// scanner invocation were separately exercised by the live adapter test; raw
// JSON alone is not proof of either upstream verification or a safe package.
func TestCapturedHomebrewInfoAndScannerCohorts(t *testing.T) {
	for _, version := range []string{"6.0.19", "7.0.2", "7.0.3", "7.0.5", "7.0.6"} {
		t.Run(version, func(t *testing.T) {
			info, err := os.ReadFile(filepath.Join("testdata", "brew-"+version+"-jq-info.json"))
			if err != nil {
				t.Fatal(err)
			}
			selected, err := parseInfo(info, []string{"jq"})
			if err != nil || len(selected) != 1 {
				t.Fatalf("selected public bottle not interpretable: %v", err)
			}
			jq := selected[0]
			if jq.artifact().SHA256 != domain.Digest("ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036") ||
				jq.Version != "1.8.2" || jq.BottleTag != "arm64_tahoe" || jq.Rebuild != 1 ||
				len(jq.Dependencies) != 1 || jq.Dependencies[0] != "oniguruma" || !jq.artifact().Valid() {
				t.Fatal("captured Homebrew bottle or closure changed")
			}
			scan, err := os.ReadFile(filepath.Join("testdata", "brew-"+version+"-jq-vulns.json"))
			if err != nil {
				t.Fatal(err)
			}
			report, err := parsePublicVulns(scan, 0, selected)
			if err != nil || len(report.Skipped) != 0 || len(report.Findings) != 0 {
				t.Fatalf("captured scanner output incompatible with selected candidate: %v", err)
			}
			// Exit zero without the explicit skipped-subject inventory cannot
			// justify a clean result. Derive the failure from the real response.
			var document map[string]json.RawMessage
			if err := json.Unmarshal(scan, &document); err != nil {
				t.Fatal(err)
			}
			delete(document, "skipped_formulae")
			missing, _ := json.Marshal(document)
			if _, err := parsePublicVulns(missing, 0, selected); err == nil {
				t.Fatal("incomplete public scanner response accepted")
			}
			document["skipped_formulae"] = json.RawMessage(`["jq"]`)
			skipped, _ := json.Marshal(document)
			if _, err := parsePublicVulns(skipped, 0, selected); err == nil {
				t.Fatal("skipped requested subject accepted")
			}
			// An info response without the dependency cannot authorize a
			// different execution closure, even when all bottle fields remain.
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(info, &envelope); err != nil {
				t.Fatal(err)
			}
			var formulae []map[string]json.RawMessage
			if err := json.Unmarshal(envelope["formulae"], &formulae); err != nil || len(formulae) != 1 {
				t.Fatal("invalid captured formulae", err)
			}
			delete(formulae[0], "dependencies")
			envelope["formulae"], _ = json.Marshal(formulae)
			broken, _ := json.Marshal(envelope)
			if bytes.Equal(broken, info) {
				t.Fatal("capture was not mutated")
			}
			if _, err := parseInfo(broken, []string{"jq"}); err == nil {
				t.Fatal("missing dependency inventory accepted")
			}
		})
	}
}
