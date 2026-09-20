// Package application coordinates eligibility, durable attempts and execution.
package application

import (
	"context"
	"errors"

	"brewwarden/internal/domain"
	"brewwarden/internal/ports"
)

type Execution struct {
	NoChanges bool
	Decision  domain.Decision
	Outcome   domain.AttemptOutcome
	ExitKnown bool
	ExitCode  int
}

// Execute owns the supplied session. The adapter must keep verified inputs bound
// to execution until Close; this workflow cannot manufacture that guarantee.
func Execute(ctx context.Context, original ports.Prepared, session ports.ExecutionSession, journal ports.Attempts, clock ports.Clock) (result Execution, err error) {
	if session == nil {
		return result, errors.New("execution services unavailable")
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			err = errors.Join(err, errors.New("execution session release failed"))
		}
	}()
	if journal == nil || clock == nil || ctx == nil {
		return result, errors.New("execution services unavailable")
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	fresh, err := session.Revalidate(ctx)
	if err != nil {
		return result, errors.New("execution revalidation failed")
	}
	now := clock.Now()
	if fresh.Assessment.Policy != original.Assessment.Policy || fresh.Assessment.Binding != original.Assessment.Binding || fresh.BeforeState != original.BeforeState || fresh.ExceptionID != original.ExceptionID ||
		!fresh.BeforeState.Valid() || fresh.ExpiresAt <= now || original.ExpiresAt <= now || fresh.ExpiresAt > original.ExpiresAt {
		return result, errors.New("execution plan changed or expired")
	}
	if (fresh.Assessment.Exception == nil) != (fresh.ExceptionID == "") || (fresh.ExceptionID != "" && !fresh.ExceptionID.Valid()) {
		return result, errors.New("execution exception identity invalid")
	}
	// Never accept a caller's serialized replay assertion or observation clock.
	fresh.Assessment.Now = now
	prior, err := journal.Attempts()
	if err != nil {
		return result, errors.New("attempt journal unavailable")
	}
	for _, a := range prior {
		if !a.Valid() || a.Unresolved() || a.Start.Binding.Attempt == fresh.Assessment.Binding.Attempt ||
			(fresh.ExceptionID != "" && a.Start.Exception == fresh.ExceptionID) {
			return result, errors.New("attempt replay or unresolved execution requires reconciliation")
		}
	}
	fresh.Assessment.AttemptAlreadyStarted = false
	result.Decision = domain.Evaluate(fresh.Assessment)
	if result.Decision.Outcome != domain.Allow {
		return result, errors.New("execution held by policy")
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	start := domain.AttemptStart{Binding: fresh.Assessment.Binding, BeforeState: fresh.BeforeState, Exception: fresh.ExceptionID, StartedAt: now}
	if err := journal.StartAttempt(start); err != nil {
		return result, errors.New("execution start was not durably recorded")
	}
	// From here on the attempt is consumed, including cancellation before Run.
	launchTime := clock.Now()
	fresh.Assessment.Now = launchTime
	result.Decision = domain.Evaluate(fresh.Assessment)
	if ctx.Err() != nil || launchTime < now || launchTime >= fresh.ExpiresAt || result.Decision.Outcome != domain.Allow {
		finish := domain.AttemptFinish{Attempt: start.Binding.Attempt, FinishedAt: max(now, clock.Now()), Outcome: domain.AttemptNotStarted, AfterState: start.BeforeState}
		result.Outcome = finish.Outcome
		return result, errors.Join(errors.New("execution cancelled or expired before launch"), journal.FinishAttempt(finish))
	}
	actual, runErr := session.Run(ctx, fresh.Assessment.Binding)
	result.ExitKnown, result.ExitCode = actual.ExitKnown, actual.ExitCode
	result.Outcome = domain.AttemptUnknown
	if actual.ExitKnown && actual.AfterState.Valid() {
		if runErr == nil && actual.ExitCode == 0 && actual.MatchesPlan {
			result.Outcome = domain.AttemptSucceeded
		} else if actual.ExitCode != 0 {
			result.Outcome = domain.AttemptFailed
			if actual.AfterState != start.BeforeState {
				result.Outcome = domain.AttemptPartial
			}
		}
	}
	finish := domain.AttemptFinish{Attempt: start.Binding.Attempt, FinishedAt: max(now, clock.Now()), Outcome: result.Outcome, ExitKnown: actual.ExitKnown, ExitCode: actual.ExitCode, AfterState: actual.AfterState}
	if err := journal.FinishAttempt(finish); err != nil {
		result.Outcome = domain.AttemptUnknown
		return result, errors.Join(runErr, errors.New("execution outcome durability unknown; reconcile before retrying"))
	}
	if result.Outcome != domain.AttemptSucceeded {
		return result, errors.Join(runErr, errors.New("execution did not complete with a verified state"))
	}
	return result, nil
}
