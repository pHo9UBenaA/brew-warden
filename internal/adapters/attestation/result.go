package attestation

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// Keep signer, subject and timestamps from one gh result together. A timestamp
// cannot borrow a matching subject or a trusted signer from another result.
type ghVerificationResult struct {
	Signature  json.RawMessage   `json:"signature"`
	Statement  json.RawMessage   `json:"statement"`
	Timestamps []json.RawMessage `json:"verifiedTimestamps"`
}

func verifiedResultMatchesSubjectAndSigner(verification ghVerificationResult, artifact domain.Artifact) (bool, error) {
	var signature struct {
		Certificate json.RawMessage `json:"certificate"`
	}
	if err := decodeObject(verification.Signature, &signature, "certificate"); err != nil {
		return false, err
	}
	var cert struct {
		Identity   string `json:"subjectAlternativeName"`
		Issuer     string `json:"issuer"`
		Repository string `json:"sourceRepositoryURI"`
		Runner     string `json:"runnerEnvironment"`
	}
	if err := decodeObject(signature.Certificate, &cert, "subjectAlternativeName", "issuer", "sourceRepositoryURI", "runnerEnvironment"); err != nil {
		return false, err
	}
	trustedWorkflow := cert.Identity == identityPrefix+"publish-commit-bottles.yml@refs/heads/main" ||
		cert.Identity == identityPrefix+"dispatch-build-bottle.yml@refs/heads/main"
	if !trustedWorkflow || cert.Issuer != issuer || cert.Repository != repository || cert.Runner != "github-hosted" {
		return false, errors.New("provenance signer identity mismatch")
	}

	var statement struct {
		Type      string            `json:"_type"`
		Predicate string            `json:"predicateType"`
		Subjects  []json.RawMessage `json:"subject"`
	}
	if err := decodeObject(verification.Statement, &statement, "_type", "predicateType", "subject"); err != nil {
		return false, err
	}
	if statement.Type != "https://in-toto.io/Statement/v1" || statement.Predicate != "https://slsa.dev/provenance/v1" || len(statement.Subjects) == 0 {
		return false, errors.New("unsupported provenance statement")
	}
	// Homebrew merges byte-identical platform bottles into an `all` bottle after
	// attestation. The candidate's exact name and the merged platform name are the
	// only accepted subjects; every subject using either name must match the digest.
	acceptedNames := acceptedSubjectNames(artifact)
	matched := false
	for _, rawSubject := range statement.Subjects {
		var subject struct {
			Name   string          `json:"name"`
			Digest json.RawMessage `json:"digest"`
		}
		if err := decodeObject(rawSubject, &subject, "name", "digest"); err != nil {
			return false, err
		}
		var digest struct {
			SHA256 domain.Digest `json:"sha256"`
		}
		if err := decodeObject(subject.Digest, &digest, "sha256"); err != nil {
			return false, err
		}
		if !slices.Contains(acceptedNames, subject.Name) {
			continue
		}
		if digest.SHA256 != artifact.SHA256 {
			return false, errors.New("provenance digest mismatch")
		}
		matched = true
	}
	return matched, nil
}

func acceptedSubjectNames(artifact domain.Artifact) []string {
	names := []string{bottleName(artifact)}
	if artifact.BottleTag == "all" && artifact.OS == "macos" && artifact.Arch == "arm64" {
		platform := artifact
		platform.BottleTag = "arm64_tahoe"
		names = append(names, bottleName(platform))
	}
	return names
}
