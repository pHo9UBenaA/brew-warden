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

// RunWithRuntime keeps wrapper options separate from literal Homebrew arguments.
// A missing trusted distribution continues to use the diagnostic-only interface.
func RunWithRuntime(ctx context.Context, args []string, out, errOut io.Writer, source ports.ConfigSource, history ports.History, service *application.Service) int {
	if service == nil {
		return RunWithServices(args, out, errOut, source, history)
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := fmt.Fprintln(out, "BrewWarden (bwd / brewwarden)\nUsage: bwd [--config PATH] [--minimum-release-age DURATION]\n           [--age-exception NAME=REASON] brew install|upgrade [FORMULA ...]\n       bwd doctor | history | status | reconcile ATTEMPT_ID\nSupported: official jq and oniguruma bottles on Apple Silicon macOS Tahoe, /opt/homebrew.\nAge exceptions apply only to named artifacts in this one attempt. Other required checks remain mandatory.\nNo casks, third-party taps, source builds or arbitrary Homebrew options.")
		if err != nil {
			return 1
		}
		return 0
	}
	if len(args) == 1 && args[0] == "--version" {
		return RunWithServices(args, out, errOut, source, history)
	}
	filtered, overrides, err := ageOptions(args)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "invocation_invalid: invalid or duplicate age exception.")
		return 1
	}
	location, age, rest, err := options(filtered)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "invocation_invalid: "+err.Error())
		return 1
	}
	if len(rest) == 1 && (rest[0] == "history" || rest[0] == "status") && len(overrides) == 0 {
		if rest[0] == "history" && history != nil {
			if showHistory(out, errOut, history) != 0 {
				return 1
			}
		}
		return showAttempts(out, errOut, service.Journal, rest[0] == "status")
	}
	if len(rest) == 2 && rest[0] == "reconcile" && len(overrides) == 0 {
		if err := service.Reconcile(ctx, domain.Digest(rest[1])); err != nil {
			_, _ = fmt.Fprintln(errOut, "reconciliation_failed: "+err.Error())
			return 1
		}
		_, err := fmt.Fprintln(out, "Reconciled observed state. The previous attempt and any age exception remain consumed; use a fresh command for a new plan.")
		if err != nil {
			return 1
		}
		return 0
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
	if len(rest) == 1 && rest[0] == "doctor" && len(overrides) == 0 {
		if service.Diagnostics == nil {
			_, _ = fmt.Fprintln(errOut, "runtime_unavailable")
			return 1
		}
		if err := service.Diagnostics.Check(ctx); err != nil {
			_, _ = fmt.Fprintln(errOut, "runtime_unavailable: "+err.Error())
			return 1
		}
		_, err := fmt.Fprintf(out, "Runtime integrity and supported platform verified.\nSupported candidates: jq, oniguruma; official arm64_tahoe bottles.\nMinimum release age: %d seconds.\nMetadata, provenance, publication, advisory coverage and installed state are freshly checked for each command.\n", policy.MinimumAgeSeconds())
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
		if _, err := fmt.Fprintf(errOut, "Plan %s\nAttempt %s\n", p.Assessment.Binding.Plan, p.Assessment.Binding.Attempt); err != nil {
			return err
		}
		for _, node := range p.Assessment.Nodes {
			a := node.Artifact
			if _, err := fmt.Fprintf(errOut, "  %s %s revision=%d rebuild=%d %s sha256:%s\n", a.Name, a.Version, a.Revision, a.Rebuild, a.BottleTag, a.SHA256); err != nil {
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
			_, _ = fmt.Fprintf(errOut, "policy: %s %s claim=%d\n", r.Code, r.Artifact.Name, r.Claim)
		}
		// Errors can contain local paths. Quoting prevents terminal control injection.
		_, _ = fmt.Fprintf(errOut, "execution_stopped: %q; outcome=%s\n", err.Error(), result.Outcome)
		if result.ExitKnown && result.ExitCode > 0 && result.ExitCode <= 255 {
			return result.ExitCode
		}
		return 1
	}
	if result.NoChanges {
		_, err = fmt.Fprintln(out, "No installed formulae to upgrade.")
	} else {
		_, err = fmt.Fprintln(out, "Installation verified; attempt recorded as succeeded.")
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
func showAttempts(out, errOut io.Writer, journal ports.Attempts, status bool) int {
	if journal == nil {
		_, _ = fmt.Fprintln(errOut, "attempt_history_unavailable")
		return 1
	}
	records, err := journal.Attempts()
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "attempt_history_invalid: cannot validate durable records.")
		return 1
	}
	unresolved := 0
	for _, a := range records {
		if !a.Valid() {
			_, _ = fmt.Fprintln(errOut, "attempt_history_invalid")
			return 1
		}
		if a.Unresolved() {
			unresolved++
		}
		outcome := string(a.Finish.Outcome)
		if outcome == "" {
			outcome = "unfinished"
		}
		if _, err := fmt.Fprintf(out, "%s %s plan=%s before=%s after=%s exit_known=%t exit_code=%d exception=%s\n", a.Start.Binding.Attempt, outcome, a.Start.Binding.Plan, a.Start.BeforeState, a.Finish.AfterState, a.Finish.ExitKnown, a.Finish.ExitCode, a.Start.Exception); err != nil {
			return 1
		}
	}
	if _, err := fmt.Fprintf(out, "%d attempts; %d require reconciliation.\n", len(records), unresolved); err != nil {
		return 1
	}
	if status && unresolved > 0 {
		return 1
	}
	return 0
}
