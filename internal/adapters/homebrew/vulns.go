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

// Valid only with the pinned scanner, explicit authenticated candidates and an
// empty inspection prefix. JSON has no clean-subject list; it is not portable
// standalone proof that an arbitrary invocation checked the requested plan.
func parsePublicVulns(raw []byte, status int, candidates []formulaMetadata) (publicVulnsReport, error) {
	var report publicVulnsReport
	if len(candidates) == 0 || len(candidates) > 128 || (status != 0 && status != 1) {
		return report, errors.New("unsupported advisory invocation")
	}
	expected := map[string]string{}
	for _, c := range candidates {
		if !c.artifact().Valid() || expected[c.Name] != "" {
			return report, errors.New("invalid advisory candidate")
		}
		expected[c.Name] = c.Version
	}
	if err := decodeSchema(raw, &report, true); err != nil {
		return publicVulnsReport{}, err
	}
	if len(report.Skipped) != 0 {
		if len(report.Skipped) > len(candidates) {
			return publicVulnsReport{}, errors.New("invalid skipped advisory inventory")
		}
		for _, name := range report.Skipped {
			if expected[name] == "" {
				return publicVulnsReport{}, errors.New("unknown skipped advisory subject")
			}
		}
		return publicVulnsReport{}, fmt.Errorf("homebrew skipped required advisory subjects: %s", strings.Join(report.Skipped, ", "))
	}
	seen := map[string]bool{}
	hasOpen := false
	for _, f := range report.Findings {
		if expected[f.Formula] == "" || expected[f.Formula] != f.Version || seen[f.Formula] || len(f.Open)+len(f.Patched) == 0 {
			return publicVulnsReport{}, errors.New("homebrew advisory subject mismatch")
		}
		seen[f.Formula] = true
		ids := map[string]bool{}
		for _, list := range [][]publicAdvisory{f.Open, f.Patched} {
			for _, a := range list {
				if !validAdvisoryID(a.ID) || ids[a.ID] {
					return publicVulnsReport{}, errors.New("invalid or contradictory advisory identifier")
				}
				ids[a.ID] = true
				for _, alias := range a.Aliases {
					if !validAdvisoryID(alias) {
						return publicVulnsReport{}, errors.New("invalid advisory alias")
					}
				}
			}
		}
		hasOpen = hasOpen || len(f.Open) > 0
	}
	if (status == 1) != hasOpen {
		return publicVulnsReport{}, errors.New("homebrew advisory exit and findings disagree")
	}
	return report, nil
}
func validAdvisoryID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == ':') {
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
	for _, c := range candidates {
		if !c.artifact().Valid() || !domain.ValidRequest("install", []string{c.Name}) || slices.Contains(names, c.Name) {
			return fail(errors.New("invalid advisory candidate"))
		}
		names = append(names, c.Name)
		args = append(args, "homebrew/core/"+c.Name)
	}
	for _, c := range candidates {
		for _, dependency := range c.Dependencies {
			if !slices.Contains(names, dependency) {
				return fail(errors.New("incomplete advisory dependency closure"))
			}
		}
	}
	profile, err := w.sandbox("advisory", true, false, []string{
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
	for _, f := range actual {
		i := slices.Index(names, f.Name)
		c := candidates[i]
		if f.artifact() != c.artifact() || f.SourceSHA256 != c.SourceSHA256 || f.SourceURL != c.SourceURL || f.RecipeSHA256 != c.RecipeSHA256 || !slices.Equal(f.Dependencies, c.Dependencies) {
			return fail(errors.New("advisory candidate metadata changed"))
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := w.command(ctx, profile, args...)
	cmd.Env = slices.DeleteFunc(cmd.Env, func(s string) bool { return s == "HOMEBREW_NO_INSTALL_FROM_API=1" || s == "HOMEBREW_DEVELOPER=1" })
	out, stderr := &processOutput{}, &processOutput{}
	cmd.Stdout, cmd.Stderr = out, stderr
	err = cmd.Run()
	status := 0
	if err != nil {
		var ex *exec.ExitError
		if !errors.As(err, &ex) || ctx.Err() != nil {
			return fail(errors.New("homebrew advisory command failed"))
		}
		status = ex.ExitCode()
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
	report, err := parsePublicVulns(out.Bytes(), status, candidates)
	if err != nil {
		return fail(err)
	}
	return report, out.Bytes(), nil
}
