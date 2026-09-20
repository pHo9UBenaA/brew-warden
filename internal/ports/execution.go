package ports

import (
	"context"

	"brewwarden/internal/domain"
)

type Attempts interface {
	StartAttempt(domain.AttemptStart) error
	FinishAttempt(domain.AttemptFinish) error
	Attempts() ([]domain.Attempt, error)
}

type Clock interface{ Now() int64 }

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

// A session owns the lock/snapshot lifetime through final state inspection.
// Revalidate must recompute identity from actual inputs, not return cached flags.
type ExecutionSession interface {
	Revalidate(context.Context) (Prepared, error)
	Run(context.Context, domain.Binding) (ExecutionResult, error)
	Close() error
}
