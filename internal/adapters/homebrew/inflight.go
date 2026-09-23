package homebrew

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// inFlight is the only durable fact needed to distinguish an owned Homebrew
// process still executing after parent death from an interrupted operation.
// It is never an authorization, saved result or replayable plan.
type inFlight struct {
	Schema     int           `json:"schema" required:"true"`
	Plan       domain.Digest `json:"plan" required:"true"`
	Attempt    domain.Digest `json:"attempt" required:"true"`
	PID        int           `json:"pid" required:"true"`
	Session    int           `json:"session" required:"true"`
	Collection string        `json:"collection" required:"true"`
}

func validCollection(name string) bool {
	suffix := strings.TrimPrefix(name, "collection-")
	return strings.HasPrefix(name, "collection-") && len(suffix) > 0 && len(suffix) <= 32 && strings.Trim(suffix, "0123456789") == ""
}
func (r inFlight) valid() bool {
	return r.Schema == 1 && r.Plan.Valid() && r.Attempt.Valid() && r.PID > 1 && r.PID < 1<<30 && r.PID == r.Session && validCollection(r.Collection)
}

func readInFlight(file string) (*inFlight, error) {
	raw, err := readRegular(file, 2048)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("unsafe in-flight execution record")
	}
	var record inFlight
	if err := decodeStrict(raw, &record); err != nil || !record.valid() {
		return nil, errors.New("invalid in-flight execution record")
	}
	return &record, nil
}

func syncParent(file string) error {
	dir, err := os.Open(filepath.Dir(file))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// Called only while holding the private BrewWarden operation lock. A record
// from a lost wrapper is removed only after the whole owned session is absent.
// The pending record is considered too: parent death between sync and rename
// must not convert an unknown child into a successful or reusable attempt.
func clearStoppedInFlight(directory string) error {
	for _, name := range []string{"inflight.json", "inflight.pending"} {
		file := filepath.Join(directory, name)
		record, err := readInFlight(file)
		if err != nil {
			return err
		}
		if record == nil {
			continue
		}
		active, err := processSessionActive(record.Session)
		if err != nil || active {
			return errors.New("an owned Homebrew process may still be running")
		}
		if err := removeCollection(directory, record.Collection); err != nil {
			return err
		}
		if err := os.Remove(file); err != nil {
			return err
		}
		if err := syncParent(file); err != nil {
			return err
		}
	}
	return nil
}

func saveInFlight(directory string, record inFlight) error {
	if !record.valid() {
		return errors.New("invalid owned Homebrew process")
	}
	file := filepath.Join(directory, "inflight.json")
	if previous, err := readInFlight(file); err != nil || previous != nil {
		return errors.New("another Homebrew process is recorded")
	}
	pending := filepath.Join(directory, "inflight.pending")
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := writeNew(pending, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(pending, file); err != nil {
		return err
	}
	return syncParent(file)
}

func removeCollection(directory, name string) error {
	if !validCollection(name) {
		return errors.New("unsafe collection identity")
	}
	root := filepath.Join(directory, name)
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() {
		return errors.New("collection workspace is unsafe")
	}
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	return syncParent(root)
}

func finishInFlight(directory string, expected inFlight) error {
	file := filepath.Join(directory, "inflight.json")
	actual, err := readInFlight(file)
	if err != nil || actual == nil || *actual != expected {
		return errors.New("owned execution record changed or unavailable")
	}
	active, err := processSessionActive(expected.Session)
	if err != nil || active {
		return errors.New("owned Homebrew process may still be running")
	}
	if err := os.Remove(file); err != nil {
		return err
	}
	return syncParent(file)
}
