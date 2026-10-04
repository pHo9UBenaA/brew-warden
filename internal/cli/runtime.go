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

const diagnosticHelp = `BrewWarden (bwd / brewwarden)
Usage: bwd [--config PATH] [--minimum-release-age DURATION] doctor
       bwd brew install|upgrade ... (disabled)
This build cannot enable execution.`

const executionHelp = `BrewWarden (bwd / brewwarden)
Usage: bwd [--config PATH] [--minimum-release-age DURATION]
           [--age-exception NAME=REASON] brew install|upgrade [FORMULA ...]
       bwd doctor
Supported: verified official core bottles on Apple Silicon macOS Tahoe, /opt/homebrew.
Age exceptions apply only to named artifacts in this one attempt. Other required checks remain mandatory.
No casks, third-party taps, source builds or arbitrary Homebrew options.`

// RunWithRuntime routes mutation requests through the supplied service.
// Unsupported commands never fall through to an unchecked brew process.
func RunWithRuntime(ctx context.Context, args []string, out, errOut io.Writer, source ports.ConfigSource, service *application.Service) int {
	if len(args) == 1 && args[0] == "--version" {
		if _, err := fmt.Fprintln(out, "BrewWarden "+Version); err != nil {
			return 1
		}
		return 0
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		help := diagnosticHelp
		if service != nil {
			help = executionHelp
		}
		if _, err := fmt.Fprintln(out, help); err != nil {
			return 1
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
			_, _ = fmt.Fprintf(errOut,
				"minimum_release_age_seconds: %d\n%s\n"+
					"No live Homebrew checks were run. This build cannot enable execution.\n",
				policy.MinimumAgeSeconds(), executionUnavailable)
			return 1
		}
		_, _ = fmt.Fprintln(errOut, executionUnavailable)
		return 1
	}
	if len(rest) == 1 && rest[0] == "doctor" && len(overrides) == 0 {
		return runDoctor(ctx, out, errOut, service.Diagnostics, policy)
	}
	if len(rest) < 2 || rest[0] != "brew" || !domain.ValidRequest(rest[1], rest[2:]) {
		_, _ = fmt.Fprintln(errOut, "invocation_invalid: unsupported operation or Homebrew options.")
		return 1
	}
	invocationService := *service
	invocationService.Present = func(prepared ports.Prepared) error {
		return presentPlan(errOut, prepared)
	}
	result, err := invocationService.Run(ctx, ports.Request{Operation: rest[1], Targets: rest[2:]}, policy, overrides)
	if err != nil {
		for _, reason := range result.Decision.Reasons {
			_, _ = fmt.Fprintf(errOut, "%s: %s (%s)\n", reason.Artifact.Name, claimName(reason.Claim), reason.Code)
		}
		outcome := string(result.Outcome)
		if outcome == "" {
			outcome = "not_started"
		}
		_, _ = fmt.Fprintf(errOut, "execution_stopped: %q; outcome=%s\n", err.Error(), outcome)
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

func runDoctor(ctx context.Context, out, errOut io.Writer, diagnostics ports.Diagnostics, policy domain.Policy) int {
	if diagnostics == nil {
		_, _ = fmt.Fprintln(errOut, "runtime_unavailable")
		return 1
	}
	if err := diagnostics.Check(ctx); err != nil {
		_, _ = fmt.Fprintln(errOut, "runtime_unavailable: "+err.Error())
		return 1
	}
	_, err := fmt.Fprintf(out,
		"Runtime integrity and supported platform verified.\n"+
			"Candidate eligibility: verified official bottles for arm64_tahoe with complete required evidence.\n"+
			"Minimum release age: %d seconds.\n"+
			"Metadata, provenance, publication, advisory coverage and installed state are freshly checked for each command.\n",
		policy.MinimumAgeSeconds())
	if err != nil {
		return 1
	}
	return 0
}

func presentPlan(out io.Writer, prepared ports.Prepared) error {
	if _, err := fmt.Fprintln(out, "Checking requested formulae and dependencies:"); err != nil {
		return err
	}
	for _, node := range prepared.Assessment.Nodes {
		artifact := node.Artifact
		if _, err := fmt.Fprintf(out, "  %s %s\n", artifact.Name, artifact.Version); err != nil {
			return err
		}
	}
	if prepared.Assessment.Exception != nil {
		for _, waiver := range prepared.Assessment.Exception.Waivers {
			if _, err := fmt.Fprintf(out, "  Age exception: %s sha256:%s reason=%q\n", waiver.Artifact.Name, waiver.Artifact.SHA256, waiver.Reason); err != nil {
				return err
			}
		}
	}
	return nil
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
