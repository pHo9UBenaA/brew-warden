package homebrew

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// Store validated, immutable evidence with a content digest in private files.
// Callers still bind cached facts to the current candidate and verify signatures.
func evidenceCachePath(directory, kind string, digest domain.Digest) (string, error) {
	if !digest.Valid() || kind != "attestations" && kind != "registrations" {
		return "", errors.New("invalid evidence cache subject")
	}
	root := filepath.Join(directory, kind)
	if err := os.Mkdir(root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("evidence cache must be private")
	}
	return filepath.Join(root, string(digest)+".json"), nil
}

func cachedEvidence(directory, kind string, digest domain.Digest) ([]byte, error) {
	path, err := evidenceCachePath(directory, kind, digest)
	if err != nil {
		return nil, err
	}
	raw, err := readRegular(path, maxManifest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entry struct {
		SHA256 domain.Digest
		Data   string
	}
	if err := decodeStrict(raw, &entry); err != nil || !entry.SHA256.Valid() || digestBytes([]byte(entry.Data)) != entry.SHA256 {
		return nil, errors.New("cached evidence integrity mismatch")
	}
	return []byte(entry.Data), nil
}

func retainEvidence(directory, kind string, digest domain.Digest, raw []byte) error {
	path, err := evidenceCachePath(directory, kind, digest)
	if err != nil {
		return err
	}
	if previous, err := cachedEvidence(directory, kind, digest); err == nil && previous != nil {
		if digestBytes(previous) != digestBytes(raw) {
			return errors.New("evidence cache changed during verification")
		}
		return nil
	} else if err != nil {
		return err
	}
	entry, err := json.Marshal(struct {
		SHA256 domain.Digest
		Data   string
	}{digestBytes(raw), string(raw)})
	if err != nil || len(entry) > maxManifest {
		return errors.New("cached evidence exceeds limit")
	}
	return writeRecord(path, entry)
}
