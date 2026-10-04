package ports

import (
	"context"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// BottleVerifier binds both provenance and age to one verified CLI response.
type BottleVerifier interface {
	Check(ctx context.Context) error
	VerifyEvidence(ctx context.Context, artifact domain.Artifact, bottlePath string, observedAt int64) (provenance, publication domain.Evidence, rawResponse []byte, err error)
}
