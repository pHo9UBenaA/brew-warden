package localstate

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"brewwarden/internal/domain"
)

const maxHistoryRecords = 10000

type refusalDocument struct {
	SchemaVersion     int      `json:"schemaVersion" required:"true"`
	Kind              string   `json:"kind" required:"true"`
	EventID           string   `json:"eventID" required:"true"`
	OccurredAt        int64    `json:"occurredAt" required:"true"`
	Operation         string   `json:"operation" required:"true"`
	Targets           []string `json:"targets" required:"true"`
	ReasonCode        string   `json:"reasonCode" required:"true"`
	MinimumAgeSeconds int64    `json:"minimumAgeSeconds" required:"true"`
}

func (f Files) RecordRefusal(operation string, targets []string, policy domain.Policy) (domain.Digest, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	r := domain.Refusal{EventID: domain.Digest(hex.EncodeToString(nonce[:])), OccurredAt: time.Now().Unix(), Operation: operation, Targets: append([]string{}, targets...), ReasonCode: "execution_binding_unverified", MinimumAgeSeconds: policy.MinimumAgeSeconds()}
	if !policy.Valid() || !r.Valid() {
		return "", errors.New("invalid refusal record")
	}
	doc := refusalDocument{1, "refusal", string(r.EventID), r.OccurredAt, r.Operation, r.Targets, r.ReasonCode, r.MinimumAgeSeconds}
	data, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	root, err := f.openHistory(true)
	if err != nil {
		return "", err
	}
	defer root.Close()
	lock, err := openPrivate(root, ".writer.lock", os.O_CREATE|os.O_RDWR)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return "", errors.New("history writer is busy")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	prior, err := readHistory(root)
	if err != nil {
		return "", err
	}
	if len(prior) >= maxHistoryRecords-2 {
		return "", errors.New("history inventory is full")
	}
	return writeImmutable(root, data, string(r.EventID))
}

func (f Files) History() ([]domain.HistoryEntry, error) {
	root, err := f.openHistory(false)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.HistoryEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return readHistory(root)
}

func (f Files) openHistory(create bool) (*os.Root, error) {
	if !filepath.IsAbs(f.StatePath) {
		return nil, errors.New("history requires an absolute user state directory")
	}
	if create {
		if err := makeStateDir(f.StatePath); err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(f.StatePath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return nil, errors.New("history directory must be private and must not be a symlink")
	}
	return os.OpenRoot(f.StatePath)
}

func makeStateDir(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return errors.New("cannot create state root")
	}
	if err := makeStateDir(parent); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	dir, err := os.Open(parent)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func openPrivate(root *os.Root, name string, flags int) (*os.File, error) {
	info, err := root.Lstat(name)
	if err != nil && !(errors.Is(err, os.ErrNotExist) && flags&os.O_CREATE != 0) {
		return nil, err
	}
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm() != 0o600) {
		return nil, errors.New("history file must be private and regular")
	}
	file, err := root.OpenFile(name, flags, 0o600)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0o600 || (info != nil && !os.SameFile(info, opened)) {
		file.Close()
		return nil, errors.New("history file changed or has unsafe permissions")
	}
	return file, nil
}

func writeImmutable(root *os.Root, data []byte, nonce string) (domain.Digest, error) {
	sum := sha256.Sum256(data)
	id := domain.Digest(hex.EncodeToString(sum[:]))
	name := string(id) + ".json"
	if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("refusing to replace an existing history record")
	}
	temp := ".pending-" + nonce
	file, err := openPrivate(root, temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if err != nil {
		return "", err
	}
	defer root.Remove(temp)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	// The private directory and writer lock exclude other cooperative writers.
	// A same-user process replacing this directory is outside the threat model.
	if err := os.Rename(filepath.Join(root.Name(), temp), filepath.Join(root.Name(), name)); err != nil {
		return "", err
	}
	dir, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return "", errors.New("history durability is unknown after rename")
	}
	return id, nil
}

func readHistory(root *os.Root) ([]domain.HistoryEntry, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(maxHistoryRecords + 2)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > maxHistoryRecords {
		return nil, errors.New("history inventory exceeds limit")
	}
	result := make([]domain.HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == ".writer.lock" || strings.HasPrefix(name, ".pending-") {
			continue
		}
		id := domain.Digest(strings.TrimSuffix(name, ".json"))
		if !strings.HasSuffix(name, ".json") || !id.Valid() {
			return nil, errors.New("unrecognized history entry")
		}
		file, err := openPrivate(root, name, os.O_RDONLY)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return nil, errors.New("cannot read history record")
		}
		sum := sha256.Sum256(data)
		if string(id) != hex.EncodeToString(sum[:]) {
			return nil, errors.New("history content digest mismatch")
		}
		var doc refusalDocument
		if err := decodeStrict(data, &doc); err != nil {
			return nil, err
		}
		r := domain.Refusal{EventID: domain.Digest(doc.EventID), OccurredAt: doc.OccurredAt, Operation: doc.Operation, Targets: doc.Targets, ReasonCode: doc.ReasonCode, MinimumAgeSeconds: doc.MinimumAgeSeconds}
		if doc.SchemaVersion != 1 || doc.Kind != "refusal" || !r.Valid() {
			return nil, errors.New("invalid history record schema")
		}
		result = append(result, domain.HistoryEntry{ID: id, Refusal: r})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Refusal.OccurredAt != result[j].Refusal.OccurredAt {
			return result[i].Refusal.OccurredAt > result[j].Refusal.OccurredAt
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}
