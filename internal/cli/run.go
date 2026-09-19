// Package cli exposes only local diagnostics until execution binding is proven.
package cli

import (
	"fmt"
	"io"
)

const executionUnavailable = "execution_binding_unverified: Homebrew execution is disabled; verified artifacts and the complete dependency plan are not bound to installation."

// Run never launches Homebrew. In particular, child flags cannot select wrapper
// diagnostics, and unknown commands cannot fall through to an unchecked process.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		if _, err := fmt.Fprintln(stdout, "BrewWarden (bwd / brewwarden)\nUsage: bwd --help | doctor\nHomebrew commands are unavailable pending execution-binding verification."); err != nil {
			return 1
		}
		return 0
	}
	if len(args) == 1 && args[0] == "doctor" {
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
