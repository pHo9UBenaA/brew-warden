// Package application coordinates eligibility and bound execution.
package application

import (
	"context"
	"errors"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

type Execution struct {
	NoChanges bool
	Decision  domain.Decision
	Outcome   domain.AttemptOutcome
	ExitKnown bool
	ExitCode  int
}

// Execute owns the supplied one-use session. The concrete session must record
// the owned process before the startup gate can launch Homebrew. An assessment
// or an exception by itself is never an authorization to execute.
func Execute(ctx context.Context, original ports.Prepared, session ports.ExecutionSession, clock ports.Clock) (result Execution, err error) {
	if session == nil {
		return result, errors.New("execution session unavailable")
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			result.Outcome = domain.AttemptUnknown
			err = errors.Join(err, errors.New("execution session release failed; owned process state may be unresolved"))
		}
	}()
	if clock == nil || ctx == nil {
		return result, errors.New("execution clock unavailable")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	fresh, err := session.Revalidate(ctx)
	if err != nil {
		return result, errors.New("execution revalidation failed")
	}
	now := clock.Now()
	samePlan := fresh.Assessment.Policy == original.Assessment.Policy &&
		fresh.Assessment.Binding == original.Assessment.Binding &&
		fresh.BeforeState == original.BeforeState && fresh.ExceptionID == original.ExceptionID
	if !samePlan || !fresh.BeforeState.Valid() || fresh.ExpiresAt <= now || original.ExpiresAt <= now || fresh.ExpiresAt > original.ExpiresAt {
		return result, errors.New("execution plan changed or expired")
	}
	hasException := fresh.Assessment.Exception != nil
	hasExceptionID := fresh.ExceptionID != ""
	if hasException != hasExceptionID || (hasExceptionID && !fresh.ExceptionID.Valid()) {
		return result, errors.New("execution exception identity invalid")
	}
	fresh.Assessment.Now = now
	result.Decision = domain.Evaluate(fresh.Assessment)
	if result.Decision.Outcome != domain.Allow {
		return result, errors.New("execution held by policy")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	actual, runErr := session.Run(ctx, fresh.Assessment.Binding)
	result.ExitKnown, result.ExitCode = actual.ExitKnown, actual.ExitCode
	result.Outcome = executionOutcome(actual, original.BeforeState, runErr)
	if result.Outcome != domain.AttemptSucceeded {
		return result, errors.Join(runErr, errors.New("execution did not complete with a verified state; retry with fresh checks"))
	}
	return result, nil
}

func executionOutcome(actual ports.ExecutionResult, before domain.Digest, runErr error) domain.AttemptOutcome {
	if !actual.ExitKnown || !actual.AfterState.Valid() {
		return domain.AttemptUnknown
	}
	if actual.ExitCode == 0 {
		if runErr == nil && actual.MatchesPlan {
			return domain.AttemptSucceeded
		}
		return domain.AttemptUnknown
	}
	if actual.AfterState != before {
		return domain.AttemptPartial
	}
	return domain.AttemptFailed
}
