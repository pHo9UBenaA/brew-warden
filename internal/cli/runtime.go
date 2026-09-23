package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/pHo9UBenaA/brew-warden/internal/application"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// Unsupported commands never fall through to an unchecked brew process.
func RunWithRuntime(ctx context.Context, args []string, out, errOut io.Writer, source ports.ConfigSource, service *application.Service) int {
	if len(args) == 1 && args[0] == "--version" {
		if _, err := fmt.Fprintln(out, "BrewWarden "+Version); err != nil {
			return 1
		}
		return 0
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		if service == nil {
			_, err := fmt.Fprintln(out, "BrewWarden (bwd / brewwarden)\nUsage: bwd [--config PATH] [--minimum-release-age DURATION] doctor\n       bwd brew install|upgrade ... (disabled)\nThis build cannot enable execution.")
			if err != nil {
				return 1
			}
		} else {
			_, err := fmt.Fprintln(out, "BrewWarden (bwd / brewwarden)\nUsage: bwd [--config PATH] [--minimum-release-age DURATION]\n           [--age-exception NAME=REASON] brew install|upgrade [FORMULA ...]\n       bwd doctor\nSupported: verified official core bottles on Apple Silicon macOS Tahoe, /opt/homebrew.\nAge exceptions apply only to named artifacts in this one attempt. Other required checks remain mandatory.\nNo casks, third-party taps, source builds or arbitrary Homebrew options.")
			if err != nil {
				return 1
			}
		}
		return 0
	}
	filtered := args
	var overrides []ports.AgeOverride
	var err error
	if service != nil {
		filtered, overrides, err = ageOptions(args)
		if err != nil {
			_, _ = fmt.Fprintln(errOut, "invocation_invalid: invalid or duplicate age exception.")
			return 1
		}
	}
	location, age, rest, err := options(filtered)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "invocation_invalid: "+err.Error())
		return 1
	}
	policy := domain.DefaultPolicy()
	if source != nil {
		policy, err = source.LoadConfig(location)
	} else if location != "" {
		err = fmt.Errorf("configuration source unavailable")
	}
	if err == nil && age != nil {
		policy, err = domain.NewPolicy(*age)
	}
	if err != nil || !policy.Valid() {
		_, _ = fmt.Fprintln(errOut, "configuration_invalid: cannot load a valid policy.")
		return 1
	}
	if service == nil {
		if len(rest) == 1 && rest[0] == "doctor" {
			_, _ = fmt.Fprintf(errOut, "minimum_release_age_seconds: %d\n%s\nNo live Homebrew checks were run. This build cannot enable execution.\n", policy.MinimumAgeSeconds(), executionUnavailable)
			return 1
		}
		_, _ = fmt.Fprintln(errOut, executionUnavailable)
		return 1
	}
	if len(rest) == 1 && rest[0] == "doctor" && len(overrides) == 0 {
		if service.Diagnostics == nil {
			_, _ = fmt.Fprintln(errOut, "runtime_unavailable")
			return 1
		}
		if err := service.Diagnostics.Check(ctx); err != nil {
			_, _ = fmt.Fprintln(errOut, "runtime_unavailable: "+err.Error())
			return 1
		}
		_, err := fmt.Fprintf(out, "Runtime integrity and supported platform verified.\nCandidate eligibility: verified official bottles for arm64_tahoe with complete required evidence.\nMinimum release age: %d seconds.\nMetadata, provenance, publication, advisory coverage and installed state are freshly checked for each command.\n", policy.MinimumAgeSeconds())
		if err != nil {
			return 1
		}
		return 0
	}
	if len(rest) < 2 || rest[0] != "brew" || !domain.ValidRequest(rest[1], rest[2:]) {
		_, _ = fmt.Fprintln(errOut, "invocation_invalid: unsupported operation or Homebrew options.")
		return 1
	}
	run := *service
	run.Present = func(p ports.Prepared) error {
		if _, err := fmt.Fprintln(errOut, "Checking requested formulae and dependencies:"); err != nil {
			return err
		}
		for _, node := range p.Assessment.Nodes {
			a := node.Artifact
			if _, err := fmt.Fprintf(errOut, "  %s %s\n", a.Name, a.Version); err != nil {
				return err
			}
		}
		if p.Assessment.Exception != nil {
			for _, w := range p.Assessment.Exception.Waivers {
				if _, err := fmt.Fprintf(errOut, "  Age exception: %s sha256:%s reason=%q\n", w.Artifact.Name, w.Artifact.SHA256, w.Reason); err != nil {
					return err
				}
			}
		}
		return nil
	}
	result, err := run.Run(ctx, ports.Request{Operation: rest[1], Targets: rest[2:]}, policy, overrides)
	if err != nil {
		for _, r := range result.Decision.Reasons {
			_, _ = fmt.Fprintf(errOut, "%s: %s (%s)\n", r.Artifact.Name, claimName(r.Claim), r.Code)
		}
		_, _ = fmt.Fprintf(errOut, "execution_stopped: %q; outcome=%s\n", err.Error(), result.Outcome)
		if result.ExitKnown && result.ExitCode > 0 && result.ExitCode <= 255 {
			return result.ExitCode
		}
		return 1
	}
	if result.NoChanges {
		_, err = fmt.Fprintln(out, "No installed formulae to upgrade.")
	} else {
		_, err = fmt.Fprintln(out, "Installation verified.")
	}
	if err != nil {
		return 1
	}
	return 0
}

func ageOptions(args []string) ([]string, []ports.AgeOverride, error) {
	filtered := []string{}
	overrides := []ports.AgeOverride{}
	seen := map[string]bool{}
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		if len(args) < 2 {
			return nil, nil, fmt.Errorf("missing option value")
		}
		if args[0] == "--age-exception" {
			name, reason, ok := strings.Cut(args[1], "=")
			if !ok || !domain.ValidRequest("install", []string{name}) || !domain.ValidAgeReason(reason) || seen[name] {
				return nil, nil, fmt.Errorf("invalid age exception")
			}
			seen[name] = true
			overrides = append(overrides, ports.AgeOverride{Name: name, Reason: reason})
		} else {
			filtered = append(filtered, args[:2]...)
		}
		args = args[2:]
	}
	return append(filtered, args...), overrides, nil
}

func claimName(claim domain.Claim) string {
	switch claim {
	case domain.Metadata:
		return "metadata verification"
	case domain.Checksum:
		return "checksum verification"
	case domain.Provenance:
		return "provenance verification"
	case domain.Publication:
		return "release age verification"
	case domain.Vulnerabilities:
		return "vulnerability verification"
	default:
		return "execution verification"
	}
}
