// Package attestation delegates cryptography to the pinned upstream verifier.
package attestation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"brewwarden/internal/domain"
)

const maxResponse = 8 * 1024 * 1024
const identityPrefix = "https://github.com/Homebrew/homebrew-core/.github/workflows/"
const issuer = "https://token.actions.githubusercontent.com"
const repository = "https://github.com/Homebrew/homebrew-core"
const verifierVersion = "github-cli-v2.101.0/attestation-only-v1"

// The path and digest come from the trusted distribution manifest, never from
// package metadata, policy configuration or a caller-controlled executable name.
type Verifier struct {
	Path   string
	SHA256 domain.Digest
}

func bottleName(a domain.Artifact) string {
	version := a.Version
	if a.Revision > 0 {
		version += "_" + strconv.Itoa(a.Revision)
	}
	rebuild := ""
	if a.Rebuild > 0 {
		rebuild = "." + strconv.Itoa(a.Rebuild)
	}
	return a.Name + "--" + version + "." + a.BottleTag + ".bottle" + rebuild + ".tar.gz"
}

func (v Verifier) Verify(ctx context.Context, a domain.Artifact, bottle, bundle, home string, now int64) (domain.Evidence, []byte, error) {
	if ctx == nil || !a.Valid() || !v.SHA256.Valid() || now <= 0 || now > 1<<62 || !filepath.IsAbs(v.Path) || !filepath.IsAbs(bottle) || !filepath.IsAbs(bundle) || !filepath.IsAbs(home) || filepath.Base(bottle) != bottleName(a) {
		return domain.Evidence{}, nil, errors.New("invalid provenance inputs")
	}
	info, err := os.Lstat(home)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return domain.Evidence{}, nil, errors.New("verifier home must be private")
	}
	helper, err := hashFile(v.Path, 128*1024*1024)
	if err != nil || helper != v.SHA256 {
		return domain.Evidence{}, nil, errors.New("verifier integrity mismatch")
	}
	actual, err := hashFile(bottle, 2*1024*1024*1024)
	if err != nil || actual != a.SHA256 {
		return domain.Evidence{}, nil, errors.New("bottle integrity mismatch")
	}
	bundleHash, err := hashFile(bundle, maxResponse)
	if err != nil {
		return domain.Evidence{}, nil, errors.New("invalid attestation bundle")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	args := []string{bottle, "--bundle", bundle, "--repo", "Homebrew/homebrew-core", "--cert-identity-regex", `^https://github\.com/Homebrew/homebrew-core/\.github/workflows/(publish-commit-bottles|dispatch-build-bottle)\.yml@refs/heads/main$`, "--cert-oidc-issuer", issuer, "--deny-self-hosted-runners", "--predicate-type", "https://slsa.dev/provenance/v1", "--format", "json"}
	command := exec.CommandContext(ctx, v.Path, args...)
	command.Env = []string{"HOME=" + home, "GH_CONFIG_DIR=" + home, "PATH=/usr/bin:/bin", "NO_COLOR=1", "TZ=UTC"}
	command.Dir = home
	command.WaitDelay = 2 * time.Second
	stdout, stderr := &boundedOutput{}, &boundedOutput{}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil || stdout.overflow || stderr.overflow {
		return domain.Evidence{}, nil, errors.New("provenance verifier failed or timed out")
	}
	for _, input := range []struct {
		path   string
		digest domain.Digest
		limit  int64
	}{{v.Path, helper, 128 * 1024 * 1024}, {bottle, actual, 2 * 1024 * 1024 * 1024}, {bundle, bundleHash, maxResponse}} {
		digest, err := hashFile(input.path, input.limit)
		if err != nil || digest != input.digest {
			return domain.Evidence{}, nil, errors.New("provenance input changed during verification")
		}
	}
	raw := stdout.Bytes()
	if err := verifiedSubject(raw, a); err != nil {
		return domain.Evidence{}, nil, err
	}
	sum := sha256.Sum256(raw)
	evidence, err := domain.NewEvidence(domain.Evidence{Claim: domain.Provenance, Subject: a, Status: domain.Verified, Provider: domain.Supplement, Source: repository, ProviderVersion: verifierVersion, RawSHA256: domain.Digest(hex.EncodeToString(sum[:])), ObservedAt: now, ExpiresAt: now + 3600})
	return evidence, raw, err
}

type boundedOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > maxResponse-b.Len() {
		b.overflow = true
		return 0, errors.New("verifier output exceeds limit")
	}
	return b.Buffer.Write(p)
}

func hashFile(path string, limit int64) (domain.Digest, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return "", errors.New("invalid verifier input file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", errors.New("verifier input changed")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, limit+1))
	if err != nil || n > limit {
		return "", errors.New("cannot hash verifier input")
	}
	return domain.Digest(hex.EncodeToString(h.Sum(nil))), nil
}
