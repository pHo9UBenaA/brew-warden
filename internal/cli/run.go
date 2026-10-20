// Package cli parses wrapper invocations and presents execution evidence.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

const executionUnavailable = "runtime_unavailable: this build is diagnostic-only; use a verified distribution."

// Version is set by the distribution build; the default is not a release claim.
var Version = "development"

var errInvalidAgeException = errors.New("invalid or duplicate age exception")

func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithConfig(args, stdout, stderr, nil)
}

func RunWithConfig(args []string, stdout, stderr io.Writer, source ports.ConfigSource) int {
	return RunWithRuntime(context.Background(), args, stdout, stderr, source, nil)
}

type invocationOptions struct {
	location   string
	minimumAge *int64
	overrides  []ports.AgeOverride
	rest       []string
}

// parseOptions consumes only leading wrapper options. An age exception is valid
// only when the caller has an execution service to receive it.
func parseOptions(args []string, allowAgeExceptions bool) (invocationOptions, error) {
	options := invocationOptions{overrides: []ports.AgeOverride{}}
	configSeen := false
	ageExceptionNames := map[string]bool{}
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		option := args[0]
		if len(args) < 2 {
			if option == "--age-exception" && allowAgeExceptions {
				return invocationOptions{}, errInvalidAgeException
			}
			return invocationOptions{}, fmt.Errorf("wrapper option requires a value")
		}
		switch option {
		case "--config":
			if configSeen || args[1] == "" {
				return invocationOptions{}, fmt.Errorf("configuration path must occur once and be nonempty")
			}
			configSeen, options.location = true, args[1]
		case "--minimum-release-age":
			value, err := time.ParseDuration(args[1])
			if options.minimumAge != nil || err != nil || value < 0 || value%time.Second != 0 {
				return invocationOptions{}, fmt.Errorf("minimum release age must occur once and be a nonnegative duration in whole seconds")
			}
			seconds := int64(value / time.Second)
			options.minimumAge = &seconds
		case "--age-exception":
			if !allowAgeExceptions {
				return invocationOptions{}, fmt.Errorf("unsupported wrapper option")
			}
			name, reason, ok := strings.Cut(args[1], "=")
			if !ok || !domain.ValidFormulaName(name) || !domain.ValidAgeReason(reason) || ageExceptionNames[name] {
				return invocationOptions{}, errInvalidAgeException
			}
			ageExceptionNames[name] = true
			options.overrides = append(options.overrides, ports.AgeOverride{Name: name, Reason: reason})
		default:
			return invocationOptions{}, fmt.Errorf("unsupported wrapper option")
		}
		args = args[2:]
	}
	options.rest = args
	return options, nil
}
