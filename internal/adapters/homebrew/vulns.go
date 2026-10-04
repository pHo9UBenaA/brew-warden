package homebrew

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// Public advisory output intentionally omits prose/severity: no severity filter
// changes eligibility, and Homebrew may emit null summaries. Retain raw output
// separately for attribution and later reconciliation with Homebrew advisories.
type publicAdvisory struct {
	ID      string   `json:"id" required:"true"`
	Aliases []string `json:"aliases" required:"true"`
}

type publicFinding struct {
	Formula string           `json:"formula" required:"true"`
	Version string           `json:"version" required:"true"`
	Open    []publicAdvisory `json:"vulnerabilities" required:"true"`
	Patched []publicAdvisory `json:"patched" required:"true"`
}

type publicVulnsReport struct {
	Findings []publicFinding `json:"findings" required:"true"`
	Skipped  []string        `json:"skipped_formulae" required:"true"`
}

// Valid only with a reviewed scanner, explicit authenticated candidates and an
// empty inspection prefix. JSON has no clean-subject list; it is not portable
// standalone proof that an arbitrary invocation checked the requested plan.
func parsePublicVulns(raw []byte, exitCode int, candidates []formulaMetadata) (publicVulnsReport, error) {
	var report publicVulnsReport
	if len(candidates) == 0 || len(candidates) > 128 || (exitCode != 0 && exitCode != 1) {
		return report, errors.New("unsupported advisory invocation")
	}
	expectedVersions := map[string]string{}
	for _, candidate := range candidates {
		if !candidate.artifact().Valid() || expectedVersions[candidate.Name] != "" {
			return report, errors.New("invalid advisory candidate")
		}
		expectedVersions[candidate.Name] = candidate.Version
	}
	if err := decodeSchema(raw, &report, true); err != nil {
		return publicVulnsReport{}, err
	}
	if len(report.Skipped) != 0 {
		if len(report.Skipped) > len(candidates) {
			return publicVulnsReport{}, errors.New("invalid skipped advisory inventory")
		}
		for _, name := range report.Skipped {
			if expectedVersions[name] == "" {
				return publicVulnsReport{}, errors.New("unknown skipped advisory subject")
			}
		}
		return publicVulnsReport{}, fmt.Errorf("homebrew skipped required advisory subjects: %s", strings.Join(report.Skipped, ", "))
	}
	seenFormulae := map[string]bool{}
	hasOpen := false
	for _, finding := range report.Findings {
		expectedVersion := expectedVersions[finding.Formula]
		hasFindings := len(finding.Open)+len(finding.Patched) > 0
		if expectedVersion == "" || expectedVersion != finding.Version || seenFormulae[finding.Formula] || !hasFindings {
			return publicVulnsReport{}, errors.New("homebrew advisory subject mismatch")
		}
		seenFormulae[finding.Formula] = true
		seenAdvisories := map[string]bool{}
		for _, group := range [][]publicAdvisory{finding.Open, finding.Patched} {
			for _, advisory := range group {
				if !validAdvisoryID(advisory.ID) || seenAdvisories[advisory.ID] {
					return publicVulnsReport{}, errors.New("invalid or contradictory advisory identifier")
				}
				seenAdvisories[advisory.ID] = true
				for _, alias := range advisory.Aliases {
					if !validAdvisoryID(alias) {
						return publicVulnsReport{}, errors.New("invalid advisory alias")
					}
				}
			}
		}
		hasOpen = hasOpen || len(finding.Open) > 0
	}
	if (exitCode == 1) != hasOpen {
		return publicVulnsReport{}, errors.New("homebrew advisory exit and findings disagree")
	}
	return report, nil
}

func validAdvisoryID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		isLetter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		isSeparator := r == '-' || r == '_' || r == '.' || r == ':'
		if !isLetter && !isDigit && !isSeparator {
			return false
		}
	}
	return true
}

// scanCandidateVulnerabilities checks the explicit candidate closure before
// collection combines it with the Homebrew advisory status.
func (w workspace) scanCandidateVulnerabilities(ctx context.Context, candidates []formulaMetadata) (publicVulnsReport, []byte, error) {
	fail := func(err error) (publicVulnsReport, []byte, error) { return publicVulnsReport{}, nil, err }
	if ctx == nil || len(candidates) == 0 || len(candidates) > 128 {
		return fail(errors.New("invalid advisory scan"))
	}
	for _, name := range []string{"Cellar", "opt"} {
		path := filepath.Join(w.root, "runtime/brew", name)
		if st, err := os.Lstat(path); err == nil && (st.Mode()&os.ModeSymlink != 0 || !st.IsDir()) {
			return fail(errors.New("invalid inspection prefix directory"))
		}
		entries, err := os.ReadDir(path)
		if err != nil && !os.IsNotExist(err) {
			return fail(err)
		}
		if len(entries) != 0 {
			return fail(errors.New("candidate advisory scan requires an empty inspection prefix"))
		}
	}
	names := make([]string, 0, len(candidates))
	args := []string{"vulns", "--json"}
	for _, candidate := range candidates {
		if !candidate.artifact().Valid() || !domain.ValidRequest("install", []string{candidate.Name}) || slices.Contains(names, candidate.Name) {
			return fail(errors.New("invalid advisory candidate"))
		}
		names = append(names, candidate.Name)
		args = append(args, "homebrew/core/"+candidate.Name)
	}
	for _, candidate := range candidates {
		for _, dependency := range candidate.Dependencies {
			if !slices.Contains(names, dependency) {
				return fail(errors.New("incomplete advisory dependency closure"))
			}
		}
	}
	profile, err := w.sandbox("advisory", sandboxPermissions{AllowNetwork: true}, []string{
		filepath.Join(w.root, "runtime/brew/Library"), filepath.Join(w.root, metadataCachePath),
		filepath.Join(w.root, "runtime/brew/Cellar"), filepath.Join(w.root, "runtime/brew/opt"),
	})
	if err != nil {
		return fail(err)
	}
	// Reconcile public metadata in the same inspection context, before invoking
	// the scanner. No installed SBOM can replace the selected current version.
	infoArgs := append([]string{"info", "--json=v2", "--formula"}, args[2:]...)
	info, err := w.invoke(ctx, "advisory-candidate-info", profile, infoArgs...)
	if err != nil {
		return fail(err)
	}
	actual, err := parseInfo(info, names)
	if err != nil {
		return fail(err)
	}
	for _, formula := range actual {
		candidate := candidates[slices.Index(names, formula.Name)]
		if formula.artifact() != candidate.artifact() || !slices.Equal(formula.Dependencies, candidate.Dependencies) {
			return fail(errors.New("advisory candidate metadata changed"))
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := w.command(ctx, profile, args...)
	out, stderr := &processOutput{}, &processOutput{}
	cmd.Stdout, cmd.Stderr = out, stderr
	err = cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || ctx.Err() != nil {
			return fail(errors.New("homebrew advisory command failed"))
		}
		exitCode = exitErr.ExitCode()
	}
	if out.overflow || stderr.overflow {
		return fail(errors.New("homebrew advisory output exceeded limit"))
	}
	if err := writeNew(filepath.Join(w.root, "advisory-scan.stderr"), stderr.Bytes(), 0600); err != nil {
		return fail(err)
	}
	if err := writeNew(filepath.Join(w.root, "advisory-scan.stdout"), out.Bytes(), 0600); err != nil {
		return fail(err)
	}
	report, err := parsePublicVulns(out.Bytes(), exitCode, candidates)
	if err != nil {
		return fail(err)
	}
	return report, out.Bytes(), nil
}
