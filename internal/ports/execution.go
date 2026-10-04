package ports

import (
	"context"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// Clock supplies the explicit time required by domain assessments.
type Clock interface {
	// Now returns Unix time in whole seconds.
	Now() int64
}

type Request struct {
	Operation string
	Targets   []string
}

// Prepared is derived from stored input bytes while the session retains its
// execution constraints. Neither an assessment nor its digests authorize Run.
type Prepared struct {
	Assessment  domain.Assessment
	BeforeState domain.Digest
	ExceptionID domain.Digest
	ExpiresAt   int64
}

type ExecutionResult struct {
	ExitKnown   bool
	ExitCode    int
	AfterState  domain.Digest
	MatchesPlan bool
}

// ExecutionSession owns the lock/snapshot lifetime through final state inspection.
// Revalidate must recompute identity from actual inputs, not return cached flags.
type ExecutionSession interface {
	Revalidate(ctx context.Context) (Prepared, error)
	Run(ctx context.Context, binding domain.Binding) (ExecutionResult, error)
	Close() error
}
