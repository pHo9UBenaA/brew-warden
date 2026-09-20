package ports

import (
	"context"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// SourceCandidate is derived from authenticated metadata and an authenticated
// recipe. UnmodifiedSource requires positive recipe inspection.
type SourceCandidate struct {
	Artifact         domain.Artifact
	SourceURL        string
	SourceSHA256     domain.Digest
	RecipeSHA256     domain.Digest
	UnmodifiedSource bool
}

type EvidenceCollector interface {
	Collect(context.Context, SourceCandidate, int64) (domain.Evidence, []byte, error)
}
type ProvenanceVerifier interface {
	Verify(context.Context, domain.Artifact, string, string, string, int64) (domain.Evidence, []byte, error)
}
