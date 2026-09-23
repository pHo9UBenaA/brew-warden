package ports

import (
	"context"
	"errors"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

var ErrNothingToDo = errors.New("no installed formulae to upgrade")

type AgeOverride struct{ Name, Reason string }
type Planner interface {
	Prepare(context.Context, Request, domain.Policy, []AgeOverride, int64) (Prepared, ExecutionSession, error)
}
type Diagnostics interface{ Check(context.Context) error }

// PresentPlan explains the exact selected artifacts and individually requested
// age waivers before execution. A failed presentation prevents mutation.
type PresentPlan func(Prepared) error
