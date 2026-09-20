package application

import (
	"context"
	"errors"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// Service coordinates planning, durable execution and explicit reconciliation.
// It never translates a saved allow decision into an execution authorization.
type Service struct {
	Planner     ports.Planner
	Journal     ports.Attempts
	Recovery    ports.Recovery
	Diagnostics ports.Diagnostics
	Clock       ports.Clock
	Present     ports.PresentPlan
}

func (s Service) Run(ctx context.Context, request ports.Request, policy domain.Policy, overrides []ports.AgeOverride) (Execution, error) {
	if s.Planner == nil || s.Journal == nil || s.Clock == nil || !policy.Valid() || !domain.ValidRequest(request.Operation, request.Targets) {
		return Execution{}, errors.New("execution services or request unavailable")
	}
	records, err := s.Journal.Attempts()
	if err != nil {
		return Execution{}, errors.New("attempt journal unavailable")
	}
	for _, record := range records {
		if !record.Valid() || record.Unresolved() {
			return Execution{}, errors.New("unresolved execution; run bwd reconcile before starting another command")
		}
	}
	prepared, session, err := s.Planner.Prepare(ctx, request, policy, overrides, s.Clock.Now())
	if errors.Is(err, ports.ErrNothingToDo) {
		return Execution{NoChanges: true}, nil
	}
	if err != nil {
		return Execution{}, err
	}
	if session == nil {
		return Execution{}, errors.New("planner returned no execution session")
	}
	if s.Present != nil {
		if err := s.Present(prepared); err != nil {
			_ = session.Close()
			return Execution{}, errors.New("cannot present verified candidate plan")
		}
	}
	return Execute(ctx, prepared, session, s.Journal, s.Clock)
}

// An omitted ID selects the sole unresolved attempt. Selection never resumes
// execution: the existing recovery adapter must establish stopped native locks.
func (s Service) Reconcile(ctx context.Context, id domain.Digest) error {
	if (id != "" && !id.Valid()) || s.Journal == nil || s.Recovery == nil || s.Clock == nil {
		return errors.New("reconciliation services or identity unavailable")
	}
	records, err := s.Journal.Attempts()
	if err != nil {
		return errors.New("attempt journal unavailable")
	}
	var selected domain.Attempt
	for _, record := range records {
		if !record.Valid() {
			return errors.New("invalid attempt journal")
		}
		if id != "" && record.Start.Binding.Attempt != id || id == "" && !record.Unresolved() {
			continue
		}
		if selected.Start.Binding.Attempt != "" {
			return errors.New("multiple attempts require selection; use bwd status and reconcile ATTEMPT_ID")
		}
		selected = record
	}
	if selected.Start.Binding.Attempt == "" {
		return errors.New("no matching attempt requires reconciliation")
	}
	if !selected.Unresolved() {
		return errors.New("attempt does not require reconciliation")
	}
	observed, err := s.Recovery.Snapshot(ctx, selected.Start.Binding)
	if err != nil || !observed.Valid() {
		return errors.New("cannot establish stopped execution and observed state")
	}
	return s.Journal.FinishAttempt(domain.AttemptFinish{Attempt: selected.Start.Binding.Attempt, FinishedAt: max(s.Clock.Now(), selected.Start.StartedAt), Outcome: domain.AttemptReconciled, AfterState: observed})
}
