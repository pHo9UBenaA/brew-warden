package attestation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// PublicGH verifies the exact bottle bytes with an installed, reviewed gh CLI.
// Unknown or incompatible versions hold before collecting execution evidence.
type PublicGH struct {
	Path string
}

// InstalledGH resolves the user's installed command without installing or
// upgrading it. Check and VerifyEvidence validate the supported version cohort.
func InstalledGH() (PublicGH, error) {
	path, err := exec.LookPath("gh")
	if err != nil {
		return PublicGH{}, errors.New("installed gh is required")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil || !filepath.IsAbs(path) {
		return PublicGH{}, errors.New("installed gh path is unavailable")
	}
	return PublicGH{Path: path}, nil
}

// Reaching the requested result limit cannot establish the earliest timestamp.
const attestationResultLimit = 100

var errUnsupportedGHVersion = errors.New("unsupported installed gh version (requires 2.66.0 through 2.101.0)")

// The lower bound is the first inspected gh using sigstore-go 0.7.0, which
// exposes the verified transparency-log URI instead of the literal "TODO".
// The upper bound is the last upstream release reviewed for this contract.
func supportedGHVersion(raw []byte) (string, error) {
	line := strings.SplitN(string(raw), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "gh" || fields[1] != "version" {
		return "", errUnsupportedGHVersion
	}
	parts := strings.Split(fields[2], ".")
	if len(parts) != 3 {
		return "", errUnsupportedGHVersion
	}
	values := [3]int{}
	for i, part := range parts {
		if len(part) == 0 || len(part) > 3 || len(part) > 1 && part[0] == '0' {
			return "", errUnsupportedGHVersion
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return "", errUnsupportedGHVersion
			}
		}
		var err error
		values[i], err = strconv.Atoi(part)
		if err != nil {
			return "", errUnsupportedGHVersion
		}
	}
	major, minor, patch := values[0], values[1], values[2]
	if major != 2 || minor < 66 || minor > 101 || (minor == 101 && patch != 0) {
		return "", errUnsupportedGHVersion
	}
	return fields[2], nil
}

func (v PublicGH) checkedVersion(ctx context.Context) (string, error) {
	if ctx == nil || !filepath.IsAbs(v.Path) {
		return "", errors.New("installed gh unavailable")
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	raw, err := runGH(bounded, v.Path, []string{"version"})
	if err != nil {
		return "", errors.New("installed gh version check unavailable or timed out")
	}
	return supportedGHVersion(raw)
}

func (v PublicGH) Check(ctx context.Context) error {
	_, err := v.checkedVersion(ctx)
	return err
}

// verifyBottle retains the checked version alongside the verified response;
// attributing evidence must not launch a second, independent version probe.
func (v PublicGH) verifyBottle(ctx context.Context, a domain.Artifact, bottle string, now int64) (int64, []byte, string, error) {
	if ctx == nil || !a.Valid() || !filepath.IsAbs(v.Path) || !filepath.IsAbs(bottle) || filepath.Base(bottle) != bottleName(a) || now <= 0 || now > 1<<62 {
		return 0, nil, "", errors.New("invalid public attestation inputs")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	verifierDigest, err := hashFile(v.Path, 128*1024*1024)
	if err != nil {
		return 0, nil, "", errors.New("installed gh unavailable")
	}
	bottleDigest, err := hashFile(bottle, 2*1024*1024*1024)
	if err != nil || bottleDigest != a.SHA256 {
		return 0, nil, "", errors.New("bottle integrity mismatch")
	}
	version, err := v.checkedVersion(ctx)
	if err != nil {
		return 0, nil, "", err
	}
	raw, err := runGH(ctx, v.Path, []string{"attestation", "verify", bottle,
		"--repo", "Homebrew/homebrew-core",
		"--predicate-type", "https://slsa.dev/provenance/v1",
		"--format", "json", "--limit", strconv.Itoa(attestationResultLimit)})
	if err != nil {
		return 0, nil, "", fmt.Errorf("public attestation verification unavailable: %w", err)
	}
	for _, input := range []struct {
		path     string
		digest   domain.Digest
		maxBytes int64
	}{{v.Path, verifierDigest, 128 * 1024 * 1024}, {bottle, bottleDigest, 2 * 1024 * 1024 * 1024}} {
		digest, err := hashFile(input.path, input.maxBytes)
		if err != nil || digest != input.digest {
			return 0, nil, "", errors.New("attestation input changed during verification")
		}
	}
	oldest, err := oldestVerifiedTimestamp(raw, a, now)
	if err != nil {
		return 0, nil, "", err
	}
	return oldest, raw, version, nil
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
		// Expose only fixed, actionable reasons; gh diagnostics can contain user
		// paths or credentials and must never be copied into wrapper output.
		if ctx.Err() != nil {
			return nil, errors.New("gh command timed out or was cancelled")
		}
		if stdout.overflow || stderr.overflow {
			return nil, errors.New("gh output exceeded limit")
		}
		message := strings.ToLower(stderr.String())
		switch {
		case strings.Contains(message, "gh auth login"):
			return nil, errors.New("gh authentication required; run gh auth login")
		case strings.Contains(message, "rate limit"):
			return nil, errors.New("GitHub attestation rate limit reached; retry later")
		}
		return nil, errors.New("gh command failed")
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
	if err := decodeJSONArray(data, &results); err != nil || len(results) == 0 || len(results) >= attestationResultLimit {
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
		var verification ghVerificationResult
		if err := decodeObject(result.Verification, &verification, "signature", "statement", "verifiedTimestamps"); err != nil || len(verification.Timestamps) == 0 {
			return 0, errors.New("missing verified attestation timestamps")
		}
		matched, err := verifiedResultSubject(verification, a)
		if err != nil || !matched {
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

// VerifyEvidence supplies two attributed claims from the same verified bytes;
// neither process success nor an unsigned publication date can satisfy age.
func (v PublicGH) VerifyEvidence(ctx context.Context, a domain.Artifact, bottle string, now int64) (domain.Evidence, domain.Evidence, []byte, error) {
	oldest, raw, version, err := v.verifyBottle(ctx, a, bottle, now)
	if err != nil {
		return domain.Evidence{}, domain.Evidence{}, nil, err
	}
	base := domain.Evidence{
		Subject: a, Status: domain.Verified, Provider: domain.Supplement,
		Source: repository, ProviderVersion: "gh/" + version,
		RawSHA256: EvidenceDigest(raw), ObservedAt: now, ExpiresAt: now + 3600,
	}
	provenance := base
	provenance.Claim = domain.Provenance
	age := base
	age.Claim, age.Publication, age.PublishedAt = domain.Publication, domain.VerifiedAttestation, oldest
	provenance, err = domain.NewEvidence(provenance)
	if err != nil {
		return domain.Evidence{}, domain.Evidence{}, nil, err
	}
	age, err = domain.NewEvidence(age)
	if err != nil {
		return domain.Evidence{}, domain.Evidence{}, nil, err
	}
	return provenance, age, raw, nil
}

// EvidenceDigest is the raw response identity for pending-operation diagnostics.
func EvidenceDigest(raw []byte) domain.Digest {
	sum := sha256.Sum256(raw)
	return domain.Digest(hex.EncodeToString(sum[:]))
}
