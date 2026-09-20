// Package cli parses wrapper invocations and presents execution evidence.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

const executionUnavailable = "runtime_unavailable: this build has no trusted bundled execution runtime; use a verified distribution."

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
	return RunWithRuntime(context.Background(), args, stdout, stderr, source, journal, nil)
}

// A missing runtime is a capability failure within the common CLI, never a
// fallback to a native process. Keep legacy refusal records readable/writable.
func refuseUnavailable(rest []string, policy domain.Policy, stderr io.Writer, journal ports.History) int {
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
