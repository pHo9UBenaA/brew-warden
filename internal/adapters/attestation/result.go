package attestation

import (
	"brewwarden/internal/domain"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

func verifiedSubject(data []byte, a domain.Artifact) error {
	if len(data) > maxResponse {
		return errors.New("provenance output exceeds limit")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := validateJSON(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing provenance result")
	}
	var results []json.RawMessage
	if err := json.Unmarshal(data, &results); err != nil || len(results) == 0 || len(results) > 32 {
		return errors.New("missing provenance subjects")
	}
	matched := false
	for _, raw := range results {
		var result struct {
			Verification json.RawMessage `json:"verificationResult"`
		}
		if err := decodeObject(raw, &result, "verificationResult"); err != nil {
			return err
		}
		var verification struct {
			Signature json.RawMessage `json:"signature"`
			Statement json.RawMessage `json:"statement"`
		}
		if err := decodeObject(result.Verification, &verification, "signature", "statement"); err != nil {
			return err
		}
		var signature struct {
			Certificate json.RawMessage `json:"certificate"`
		}
		if err := decodeObject(verification.Signature, &signature, "certificate"); err != nil {
			return err
		}
		var cert struct {
			Identity   string `json:"subjectAlternativeName"`
			Issuer     string `json:"issuer"`
			Repository string `json:"sourceRepositoryURI"`
			Runner     string `json:"runnerEnvironment"`
		}
		if err := decodeObject(signature.Certificate, &cert, "subjectAlternativeName", "issuer", "sourceRepositoryURI", "runnerEnvironment"); err != nil {
			return err
		}
		if (cert.Identity != identityPrefix+"publish-commit-bottles.yml@refs/heads/main" && cert.Identity != identityPrefix+"dispatch-build-bottle.yml@refs/heads/main") || cert.Issuer != issuer || cert.Repository != repository || cert.Runner != "github-hosted" {
			return errors.New("provenance signer identity mismatch")
		}
		var statement struct {
			Type      string            `json:"_type"`
			Predicate string            `json:"predicateType"`
			Subjects  []json.RawMessage `json:"subject"`
		}
		if err := decodeObject(verification.Statement, &statement, "_type", "predicateType", "subject"); err != nil {
			return err
		}
		if statement.Type != "https://in-toto.io/Statement/v1" || statement.Predicate != "https://slsa.dev/provenance/v1" || len(statement.Subjects) == 0 {
			return errors.New("unsupported provenance statement")
		}
		for _, rawSubject := range statement.Subjects {
			var subject struct {
				Name   string          `json:"name"`
				Digest json.RawMessage `json:"digest"`
			}
			if err := decodeObject(rawSubject, &subject, "name", "digest"); err != nil {
				return err
			}
			var digest struct {
				SHA256 domain.Digest `json:"sha256"`
			}
			if err := decodeObject(subject.Digest, &digest, "sha256"); err != nil {
				return err
			}
			if subject.Name == bottleName(a) {
				if digest.SHA256 != a.SHA256 {
					return errors.New("provenance digest mismatch")
				}
				matched = true
			}
		}
	}
	if !matched {
		return errors.New("verified candidate subject missing")
	}
	return nil
}
