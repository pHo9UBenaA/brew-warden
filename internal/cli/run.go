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

// Run never launches Homebrew. In particular, child flags cannot select wrapper
// diagnostics, and unknown commands cannot fall through to an unchecked process.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithConfig(args, stdout, stderr, nil)
}

func RunWithConfig(args []string, stdout, stderr io.Writer, source ports.ConfigSource) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		if _, err := fmt.Fprintln(stdout, "BrewWarden (bwd / brewwarden)\nUsage: bwd [--config PATH] [--minimum-release-age DURATION] doctor\n       bwd brew install|upgrade ... (disabled)\nHomebrew commands are unavailable pending execution-binding verification."); err != nil {
			return 1
		}
		return 0
	}
	location, override, rest, err := options(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "invocation_invalid: "+err.Error())
		return 1
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
	return 1
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
