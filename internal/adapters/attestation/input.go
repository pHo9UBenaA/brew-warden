package attestation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strconv"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

const maxResponse = 8 * 1024 * 1024
const identityPrefix = "https://github.com/Homebrew/homebrew-core/.github/workflows/"
const issuer = "https://token.actions.githubusercontent.com"
const repository = "https://github.com/Homebrew/homebrew-core"

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
