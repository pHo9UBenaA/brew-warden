package homebrew

import (
	"os"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// One representative of equivalent public responses captured in disposable
// Tahoe guests. JSON replay does not establish upstream signature verification
// or execute the labeled Homebrew release.
func TestCapturedHomebrewInfoAndScanner(t *testing.T) {
	info, err := os.ReadFile("testdata/brew-6.0.19-jq-info.json")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := parseInfo(info, []string{"jq"})
	if err != nil || len(selected) != 1 {
		t.Fatalf("want one selected public bottle: candidates=%+v error=%v", selected, err)
	}
	jq := selected[0]
	if jq.artifact().SHA256 != domain.Digest("ca67c64d0aaf1e5472790ec2cc081ff7972316f27095d8a8aab81b3321247036") ||
		jq.Version != "1.8.2" || jq.BottleTag != "arm64_tahoe" || jq.Rebuild != 1 ||
		len(jq.Dependencies) != 1 || jq.Dependencies[0] != "oniguruma" || !jq.artifact().Valid() {
		t.Fatalf("captured jq 1.8.2 arm64_tahoe rebuild 1 with oniguruma dependency changed: %+v", jq)
	}
	scan, err := os.ReadFile("testdata/brew-6.0.19-jq-vulns.json")
	if err != nil {
		t.Fatal(err)
	}
	report, err := parsePublicVulns(scan, 0, selected)
	if err != nil || len(report.Skipped) != 0 || len(report.Findings) != 0 {
		t.Fatalf("want captured scan without findings or skipped subjects: report=%+v error=%v", report, err)
	}
}
