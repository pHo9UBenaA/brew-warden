// Package cli parses wrapper invocations and presents execution evidence.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

const executionUnavailable = "runtime_unavailable: this build is diagnostic-only; use a verified distribution."

// Version is set by the reproducible development build; it is not a release claim.
var Version = "development"

func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithConfig(args, stdout, stderr, nil)
}

func RunWithConfig(args []string, stdout, stderr io.Writer, source ports.ConfigSource) int {
	return RunWithRuntime(context.Background(), args, stdout, stderr, source, nil)
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
