package attestation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// PublicGH is an isolated public-command capability. The product must not use
// its age evidence until the collection, execution and distribution boundaries
// have migrated together. Only the inspected CLI version is accepted.
type PublicGH struct {
	Path string
}

const ghLimit = 100
const ghVersion = "gh version 2.101.0 "

// VerifyBottle returns the oldest verified timestamp for the exact bytes, as
// well as the raw CLI response. The caller must bind both to its pending plan.
func (v PublicGH) VerifyBottle(ctx context.Context, a domain.Artifact, bottle string, now int64) (int64, []byte, error) {
	if ctx == nil || !a.Valid() || !filepath.IsAbs(v.Path) || !filepath.IsAbs(bottle) || filepath.Base(bottle) != bottleName(a) || now <= 0 {
		return 0, nil, errors.New("invalid public attestation inputs")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	tool, err := hashFile(v.Path, 128*1024*1024)
	if err != nil {
		return 0, nil, errors.New("installed gh unavailable")
	}
	actual, err := hashFile(bottle, 2*1024*1024*1024)
	if err != nil || actual != a.SHA256 {
		return 0, nil, errors.New("bottle integrity mismatch")
	}
	version, err := runGH(ctx, v.Path, []string{"version"})
	if err != nil || !strings.HasPrefix(string(version), ghVersion) {
		return 0, nil, errors.New("unsupported installed gh version")
	}
	raw, err := runGH(ctx, v.Path, []string{"attestation", "verify", bottle,
		"--repo", "Homebrew/homebrew-core",
		"--predicate-type", "https://slsa.dev/provenance/v1",
		"--format", "json", "--limit", "100"})
	if err != nil {
		return 0, nil, errors.New("public attestation verification failed or unavailable")
	}
	for _, input := range []struct {
		path   string
		digest domain.Digest
		limit  int64
	}{{v.Path, tool, 128 * 1024 * 1024}, {bottle, actual, 2 * 1024 * 1024 * 1024}} {
		digest, err := hashFile(input.path, input.limit)
		if err != nil || digest != input.digest {
			return 0, nil, errors.New("attestation input changed during verification")
		}
	}
	oldest, err := oldestVerifiedTimestamp(raw, a, now)
	if err != nil {
		return 0, nil, err
	}
	return oldest, raw, nil
}

func runGH(ctx context.Context, path string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.WaitDelay = 2 * time.Second
	// The installed CLI uses the user's normal authentication and trust roots.
	// Do not let an ambient enterprise host override the fixed github.com repo.
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "GH_HOST=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	stdout, stderr := &boundedOutput{}, &boundedOutput{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil || stdout.overflow || stderr.overflow || ctx.Err() != nil {
		return nil, errors.New("gh command failed or output exceeded limit")
	}
	return stdout.Bytes(), nil
}

// Parse all returned results, not just the first or the newest. Every accepted
// timestamp must be tied to a verified matching statement. A saturated result
// set cannot establish the oldest timestamp, even when all results are valid.
func oldestVerifiedTimestamp(data []byte, a domain.Artifact, now int64) (int64, error) {
	if len(data) == 0 || len(data) > maxResponse {
		return 0, errors.New("invalid public attestation output size")
	}
	var results []json.RawMessage
	if err := decodeJSONArray(data, &results); err != nil || len(results) == 0 || len(results) >= ghLimit {
		return 0, errors.New("missing or saturated public attestations")
	}
	oldest := now
	for _, raw := range results {
		var result struct {
			Verification json.RawMessage `json:"verificationResult"`
		}
		if err := decodeObject(raw, &result, "verificationResult"); err != nil {
			return 0, err
		}
		var verification struct {
			Signature  json.RawMessage   `json:"signature"`
			Statement  json.RawMessage   `json:"statement"`
			Timestamps []json.RawMessage `json:"verifiedTimestamps"`
		}
		if err := decodeObject(result.Verification, &verification, "signature", "statement", "verifiedTimestamps"); err != nil || len(verification.Timestamps) == 0 {
			return 0, errors.New("missing verified attestation timestamps")
		}
		// Validate all signer and subject fields using the same contract as the
		// existing verifier; do not borrow time from an unrelated result.
		wrapped, err := json.Marshal([]json.RawMessage{raw})
		if err != nil || verifiedSubject(wrapped, a) != nil {
			return 0, errors.New("public attestation subject or signer mismatch")
		}
		for _, value := range verification.Timestamps {
			var timestamp struct {
				Type string `json:"type"`
				URI  string `json:"uri"`
				Time string `json:"timestamp"`
			}
			if err := decodeObject(value, &timestamp, "type", "uri", "timestamp"); err != nil || timestamp.Type != "Tlog" || timestamp.URI != "https://rekor.sigstore.dev" {
				return 0, errors.New("unsupported verified timestamp")
			}
			t, err := time.Parse(time.RFC3339Nano, timestamp.Time)
			if err != nil || t.Unix() <= 0 || t.After(time.Unix(now, 0)) {
				return 0, errors.New("invalid verified attestation time")
			}
			if t.Unix() < oldest {
				oldest = t.Unix()
			}
		}
	}
	return oldest, nil
}

func decodeJSONArray(data []byte, out *[]json.RawMessage) error {
	if !utf8.Valid(data) {
		return errors.New("invalid public attestation UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := validateJSON(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing public attestation JSON")
	}
	if err := json.Unmarshal(data, out); err != nil || *out == nil {
		return errors.New("invalid public attestation results")
	}
	return nil
}

// EvidenceDigest is the raw response identity for pending-operation diagnostics.
func EvidenceDigest(raw []byte) domain.Digest {
	sum := sha256.Sum256(raw)
	return domain.Digest(hex.EncodeToString(sum[:]))
}
