package ports

import (
	"context"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// BottleVerifier binds both provenance and age to one verified CLI response.
type BottleVerifier interface {
	Check(context.Context) error
	VerifyEvidence(context.Context, domain.Artifact, string, int64) (domain.Evidence, domain.Evidence, []byte, error)
}
