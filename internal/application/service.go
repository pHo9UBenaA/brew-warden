package application

import (
	"context"
	"errors"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// Service creates a new bound plan for every request; it never replays a saved
// assessment. The planner's operation lock and startup gate own crash safety.
type Service struct {
	Planner     ports.Planner
	Diagnostics ports.Diagnostics
	Clock       ports.Clock
	Present     ports.PresentPlan
}

func (s Service) Run(ctx context.Context, request ports.Request, policy domain.Policy, overrides []ports.AgeOverride) (Execution, error) {
	if s.Planner == nil || s.Clock == nil || ctx == nil || !policy.Valid() || !domain.ValidRequest(request.Operation, request.Targets) {
		return Execution{}, errors.New("execution services or request unavailable")
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
	return Execute(ctx, prepared, session, s.Clock)
}
