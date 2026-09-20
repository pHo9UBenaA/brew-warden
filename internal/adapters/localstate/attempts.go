package localstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// Journal stores append-only start/result records separately from refusals.
type Journal struct{ Path string }

type attemptDocument struct {
	Schema      int    `json:"schemaVersion" required:"true"`
	Kind        string `json:"kind" required:"true"`
	Previous    string `json:"previous" required:"true"`
	Attempt     string `json:"attempt" required:"true"`
	Plan        string `json:"plan" required:"true"`
	Policy      string `json:"policy" required:"true"`
	Graph       string `json:"graph" required:"true"`
	Environment string `json:"environment" required:"true"`
	Before      string `json:"beforeState" required:"true"`
	Exception   string `json:"exception" required:"true"`
	StartedAt   int64  `json:"startedAt" required:"true"`
	FinishedAt  int64  `json:"finishedAt" required:"true"`
	Outcome     string `json:"outcome" required:"true"`
	ExitKnown   bool   `json:"exitKnown" required:"true"`
	ExitCode    int    `json:"exitCode" required:"true"`
	After       string `json:"afterState" required:"true"`
}

func documentStart(s domain.AttemptStart) attemptDocument {
	return attemptDocument{Schema: 1, Kind: "start", Attempt: string(s.Binding.Attempt), Plan: string(s.Binding.Plan), Policy: string(s.Binding.Policy), Graph: string(s.Binding.Graph), Environment: string(s.Binding.Environment), Before: string(s.BeforeState), Exception: string(s.Exception), StartedAt: s.StartedAt}
}

func (d attemptDocument) start() domain.AttemptStart {
	return domain.AttemptStart{Binding: domain.Binding{Attempt: domain.Digest(d.Attempt), Plan: domain.Digest(d.Plan), Policy: domain.Digest(d.Policy), Graph: domain.Digest(d.Graph), Environment: domain.Digest(d.Environment)}, BeforeState: domain.Digest(d.Before), Exception: domain.Digest(d.Exception), StartedAt: d.StartedAt}
}

func (d attemptDocument) finish() domain.AttemptFinish {
	return domain.AttemptFinish{Attempt: domain.Digest(d.Attempt), FinishedAt: d.FinishedAt, Outcome: domain.AttemptOutcome(d.Outcome), ExitKnown: d.ExitKnown, ExitCode: d.ExitCode, AfterState: domain.Digest(d.After)}
}

func (d attemptDocument) valid() bool {
	if d.Schema != 1 || !d.start().Valid() {
		return false
	}
	if d.Kind == "start" {
		return d == documentStart(d.start())
	}
	return d.Kind == "finish" && domain.Digest(d.Previous).Valid() && d.finish().Valid() && d.FinishedAt >= d.StartedAt
}

type attemptHead struct {
	attempt domain.Attempt
	id      domain.Digest
}

// withJournal holds a cross-process lock across validation and append. A result
// whose directory sync failed may already exist: callers must treat that error
// as uncertain durability, never as permission to rerun the child process.
func (j Journal) withJournal(write bool, fn func(*os.Root, map[domain.Digest]attemptHead) error) error {
	root, err := (Files{StatePath: j.Path}).openHistory(write)
	if !write && errors.Is(err, os.ErrNotExist) {
		return fn(nil, map[domain.Digest]attemptHead{})
	}
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := openPrivate(root, ".writer.lock", os.O_CREATE|os.O_RDWR)
	if err != nil {
		return err
	}
	defer lock.Close()
	mode := syscall.LOCK_SH
	if write {
		mode = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(lock.Fd()), mode|syscall.LOCK_NB); err != nil {
		return errors.New("attempt journal is busy")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	heads, err := readAttempts(root)
	if err != nil {
		return err
	}
	return fn(root, heads)
}

func (j Journal) Attempts() ([]domain.Attempt, error) {
	result := []domain.Attempt{}
	err := j.withJournal(false, func(_ *os.Root, heads map[domain.Digest]attemptHead) error {
		for _, head := range heads {
			result = append(result, head.attempt)
		}
		return nil
	})
	sort.Slice(result, func(i, k int) bool { return result[i].Start.Binding.Attempt < result[k].Start.Binding.Attempt })
	return result, err
}

func (j Journal) StartAttempt(start domain.AttemptStart) error {
	if !start.Valid() {
		return errors.New("invalid attempt start")
	}
	return j.withJournal(true, func(root *os.Root, heads map[domain.Digest]attemptHead) error {
		for id, head := range heads {
			if id == start.Binding.Attempt || head.attempt.Unresolved() || (start.Exception != "" && head.attempt.Start.Exception == start.Exception) {
				return errors.New("attempt consumed or unresolved; reconciliation is required")
			}
		}
		return appendAttempt(root, documentStart(start))
	})
}

func (j Journal) FinishAttempt(finish domain.AttemptFinish) error {
	if !finish.Valid() {
		return errors.New("invalid attempt outcome")
	}
	return j.withJournal(true, func(root *os.Root, heads map[domain.Digest]attemptHead) error {
		head, exists := heads[finish.Attempt]
		if !exists || !head.attempt.Unresolved() || finish.FinishedAt < head.attempt.Start.StartedAt ||
			(head.attempt.Finish.Outcome != "" && finish.Outcome != domain.AttemptReconciled) ||
			finish.FinishedAt < head.attempt.Finish.FinishedAt {
			return errors.New("invalid attempt transition")
		}
		doc := documentStart(head.attempt.Start)
		doc.Kind, doc.Previous = "finish", string(head.id)
		doc.FinishedAt, doc.Outcome = finish.FinishedAt, string(finish.Outcome)
		doc.ExitKnown, doc.ExitCode, doc.After = finish.ExitKnown, finish.ExitCode, string(finish.AfterState)
		return appendAttempt(root, doc)
	})
}

func appendAttempt(root *os.Root, doc attemptDocument) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := dir.ReadDir(maxHistoryRecords + 1)
	closeErr := dir.Close()
	if (readErr != nil && readErr != io.EOF) || closeErr != nil {
		return errors.New("cannot inspect journal capacity")
	}
	reserve := 1
	if doc.Kind == "start" {
		reserve = 3
	}
	if len(entries) > maxHistoryRecords-reserve {
		return errors.New("attempt journal inventory is full")
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = writeImmutable(root, data, doc.Attempt+"-"+doc.Kind+"-"+doc.Outcome)
	return err
}

func readAttempts(root *os.Root) (map[domain.Digest]attemptHead, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(maxHistoryRecords + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	// Reserve room for a final outcome and reconciliation before accepting starts.
	if len(entries) > maxHistoryRecords {
		return nil, errors.New("attempt journal inventory exceeds limit")
	}
	docs := map[domain.Digest]attemptDocument{}
	roots := map[domain.Digest]domain.Digest{}
	next := map[domain.Digest]domain.Digest{}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".writer.lock" || strings.HasPrefix(name, ".pending-") {
			continue
		}
		id := domain.Digest(strings.TrimSuffix(name, ".json"))
		if !strings.HasSuffix(name, ".json") || !id.Valid() {
			return nil, errors.New("unrecognized attempt record")
		}
		file, err := openPrivate(root, name, os.O_RDONLY)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return nil, errors.New("cannot read attempt record")
		}
		sum := sha256.Sum256(data)
		if string(id) != hex.EncodeToString(sum[:]) {
			return nil, errors.New("attempt record digest mismatch")
		}
		var doc attemptDocument
		if err := decodeStrict(data, &doc); err != nil {
			return nil, err
		}
		if !doc.valid() {
			return nil, errors.New("invalid attempt record")
		}
		docs[id] = doc
		if doc.Kind == "start" {
			attempt := domain.Digest(doc.Attempt)
			if roots[attempt] != "" {
				return nil, errors.New("duplicate attempt start")
			}
			roots[attempt] = id
		} else {
			previous := domain.Digest(doc.Previous)
			if next[previous] != "" {
				return nil, errors.New("forked attempt history")
			}
			next[previous] = id
		}
	}
	heads := map[domain.Digest]attemptHead{}
	seen := map[domain.Digest]bool{}
	exceptions := map[domain.Digest]bool{}
	for attempt, id := range roots {
		start := docs[id].start()
		if start.Exception != "" {
			if exceptions[start.Exception] {
				return nil, errors.New("replayed age exception")
			}
			exceptions[start.Exception] = true
		}
		head := attemptHead{attempt: domain.Attempt{Start: start}, id: id}
		for {
			if seen[id] {
				return nil, errors.New("cyclic attempt history")
			}
			seen[id] = true
			child := next[id]
			if child == "" {
				break
			}
			doc := docs[child]
			finish := doc.finish()
			if doc.start() != start || !head.attempt.Unresolved() || finish.FinishedAt < head.attempt.Finish.FinishedAt ||
				(head.attempt.Finish.Outcome != "" && finish.Outcome != domain.AttemptReconciled) {
				return nil, errors.New("inconsistent attempt chain")
			}
			head.attempt.Finish, head.id, id = finish, child, child
		}
		heads[attempt] = head
	}
	if len(seen) != len(docs) {
		return nil, errors.New("orphaned attempt result")
	}
	return heads, nil
}
