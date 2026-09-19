// Package cli exposes only local diagnostics until execution binding is proven.
package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"brewwarden/internal/domain"
	"brewwarden/internal/ports"
)

const executionUnavailable = "execution_binding_unverified: Homebrew execution is disabled; verified artifacts and the complete dependency plan are not bound to installation."

// Version is set by the reproducible development build; it is not a release claim.
var Version = "development"

// Run never launches Homebrew. In particular, child flags cannot select wrapper
// diagnostics, and unknown commands cannot fall through to an unchecked process.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithConfig(args, stdout, stderr, nil)
}

func RunWithConfig(args []string, stdout, stderr io.Writer, source ports.ConfigSource) int {
	return RunWithServices(args, stdout, stderr, source, nil)
}

func RunWithServices(args []string, stdout, stderr io.Writer, source ports.ConfigSource, journal ports.History) int {
	if len(args) == 1 && args[0] == "--version" {
		if _, err := fmt.Fprintln(stdout, "BrewWarden "+Version); err != nil {
			return 1
		}
		return 0
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		if _, err := fmt.Fprintln(stdout, "BrewWarden (bwd / brewwarden)\nUsage: bwd [--config PATH] [--minimum-release-age DURATION] doctor\n       bwd history\n       bwd brew install|upgrade ... (disabled)\nHomebrew commands are unavailable pending execution-binding verification."); err != nil {
			return 1
		}
		return 0
	}
	location, override, rest, err := options(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "invocation_invalid: "+err.Error())
		return 1
	}
	// History must remain available even when policy configuration is invalid.
	if len(rest) == 1 && rest[0] == "history" {
		return showHistory(stdout, stderr, journal)
	}
	policy := domain.DefaultPolicy()
	if source != nil {
		policy, err = source.LoadConfig(location)
	} else if location != "" {
		err = fmt.Errorf("configuration source is unavailable")
	}
	if err == nil && override != nil {
		policy, err = domain.NewPolicy(*override)
	}
	if err != nil || !policy.Valid() {
		_, _ = fmt.Fprintln(stderr, "configuration_invalid: cannot load a valid policy; check the configuration schema, permissions and values.")
		return 1
	}
	if len(rest) == 1 && rest[0] == "doctor" {
		_, _ = fmt.Fprintln(stderr, "minimum_release_age_seconds: "+strconv.FormatInt(policy.MinimumAgeSeconds(), 10))
		_, _ = fmt.Fprintln(stderr, executionUnavailable)
		_, _ = fmt.Fprintln(stderr, "No live Homebrew checks were run. No brew version or installation path is supported for execution yet.")
		return 1
	}
	// Reject the entire invocation, including unsupported options, aliases,
	// nested commands and apparently read-only brew operations. Do not echo
	// untrusted tokens into the terminal or consume a child's --help.
	_, _ = fmt.Fprintln(stderr, executionUnavailable)
	if journal != nil && len(rest) >= 2 && rest[0] == "brew" && domain.ValidRequest(rest[1], rest[2:]) {
		if id, err := journal.RecordRefusal(rest[1], rest[2:], policy); err != nil {
			_, _ = fmt.Fprintln(stderr, "history_unavailable: the refused request could not be durably recorded; no Homebrew process was started.")
		} else {
			_, _ = fmt.Fprintln(stderr, "history_record: "+string(id))
		}
	}
	return 1
}

func showHistory(stdout, stderr io.Writer, journal ports.History) int {
	if journal == nil {
		_, _ = fmt.Fprintln(stderr, "history_unavailable: no history source is configured.")
		return 1
	}
	entries, err := journal.History()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "history_unavailable: cannot validate the history records.")
		return 1
	}
	if len(entries) == 0 {
		if _, err := fmt.Fprintln(stdout, "No recorded requests."); err != nil {
			return 1
		}
	}
	for _, entry := range entries {
		if !entry.ID.Valid() || !entry.Refusal.Valid() {
			_, _ = fmt.Fprintln(stderr, "history_invalid: invalid record returned by history source.")
			return 1
		}
		r := entry.Refusal
		if _, err := fmt.Fprintf(stdout, "%s %s refused %s %q (%s)\n", entry.ID, time.Unix(r.OccurredAt, 0).UTC().Format(time.RFC3339), r.Operation, r.Targets, r.ReasonCode); err != nil {
			return 1
		}
	}
	return 0
}

func options(args []string) (location string, age *int64, rest []string, err error) {
	configSeen := false
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		if len(args) < 2 {
			return "", nil, nil, fmt.Errorf("wrapper option requires a value")
		}
		switch args[0] {
		case "--config":
			if configSeen || args[1] == "" {
				return "", nil, nil, fmt.Errorf("configuration path must occur once and be nonempty")
			}
			configSeen, location = true, args[1]
		case "--minimum-release-age":
			value, parseErr := time.ParseDuration(args[1])
			if age != nil || parseErr != nil || value < 0 || value%time.Second != 0 {
				return "", nil, nil, fmt.Errorf("minimum release age must occur once and be a nonnegative duration in whole seconds")
			}
			seconds := int64(value / time.Second)
			age = &seconds
		default:
			return "", nil, nil, fmt.Errorf("unsupported wrapper option")
		}
		args = args[2:]
	}
	return location, age, args, nil
}
