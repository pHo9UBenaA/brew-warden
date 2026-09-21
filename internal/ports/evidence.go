package ports

import (
	"context"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

type ProvenanceVerifier interface {
	Verify(context.Context, domain.Artifact, string, string, string, int64) (domain.Evidence, []byte, error)
}
