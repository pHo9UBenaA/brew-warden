package localstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"brewwarden/internal/domain"
)

func attemptFixture(letter string) domain.AttemptStart {
	d := domain.Digest(strings.Repeat(letter, 64))
	return domain.AttemptStart{Binding: domain.Binding{Attempt: d, Plan: d, Policy: d, Graph: d, Environment: d}, BeforeState: d, Exception: domain.Digest(strings.Repeat("e", 64)), StartedAt: 1000}
}

func TestAttemptConsumptionAndRecovery(t *testing.T) {
	j := Journal{Path: filepath.Join(t.TempDir(), "attempts")}
	if records, err := j.Attempts(); err != nil || len(records) != 0 {
		t.Fatal(records, err)
	}
	if _, err := os.Stat(j.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read created state", err)
	}
	s := attemptFixture("a")
	if err := j.StartAttempt(s); err != nil {
		t.Fatal(err)
	}
	if err := j.StartAttempt(s); err == nil {
		t.Fatal("duplicate attempt accepted")
	}
	next := attemptFixture("b")
	next.Exception = ""
	if err := j.StartAttempt(next); err == nil {
		t.Fatal("unresolved state ignored")
	}
	f := domain.AttemptFinish{Attempt: s.Binding.Attempt, FinishedAt: 1001, Outcome: domain.AttemptUnknown}
	if err := j.FinishAttempt(f); err != nil {
		t.Fatal(err)
	}
	if err := j.FinishAttempt(f); err == nil {
		t.Fatal("duplicate outcome accepted")
	}
	f.Outcome = domain.AttemptReconciled
	f.AfterState = s.BeforeState
	f.FinishedAt++
	if err := j.FinishAttempt(f); err != nil {
		t.Fatal(err)
	}
	if err := j.StartAttempt(s); err == nil {
		t.Fatal("reconciliation unconsumed old attempt")
	}
	next.Exception = s.Exception
	if err := j.StartAttempt(next); err == nil {
		t.Fatal("age exception replayed under new attempt")
	}
	next.Exception = ""
	if err := j.StartAttempt(next); err != nil {
		t.Fatal(err)
	}
	got, err := j.Attempts()
	if err != nil || len(got) != 2 || got[0].Finish.Outcome != domain.AttemptReconciled || !got[1].Unresolved() {
		t.Fatal(got, err)
	}
}

func TestAttemptCorruptionAndUnsafePaths(t *testing.T) {
	for _, kind := range []string{"digest", "duplicate field", "missing start", "symlink", "permissions", "incomplete temp"} {
		t.Run(kind, func(t *testing.T) {
			j := Journal{Path: filepath.Join(t.TempDir(), "attempts")}
			s := attemptFixture("a")
			if err := j.StartAttempt(s); err != nil {
				t.Fatal(err)
			}
			files, err := filepath.Glob(filepath.Join(j.Path, "*.json"))
			if err != nil || len(files) != 1 {
				t.Fatal(files, err)
			}
			path := files[0]
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "digest":
				err = os.WriteFile(path, append(data, ' '), 0o600)
			case "duplicate field":
				data = []byte(strings.Replace(string(data), `"kind":"start"`, `"kind":"start","kind":"start"`, 1))
				sum := sha256.Sum256(data)
				err = os.Remove(path)
				if err == nil {
					err = os.WriteFile(filepath.Join(j.Path, hex.EncodeToString(sum[:])+".json"), data, 0o600)
				}
			case "missing start":
				err = j.FinishAttempt(domain.AttemptFinish{Attempt: s.Binding.Attempt, FinishedAt: 1001, Outcome: domain.AttemptSucceeded, ExitKnown: true, AfterState: s.BeforeState})
				if err == nil {
					err = os.Remove(path)
				}
			case "symlink":
				outside := filepath.Join(t.TempDir(), "record")
				err = os.WriteFile(outside, data, 0o600)
				if err == nil {
					err = os.Remove(path)
				}
				if err == nil {
					err = os.Symlink(outside, path)
				}
			case "permissions":
				err = os.Chmod(path, 0o644)
			case "incomplete temp":
				err = os.WriteFile(filepath.Join(j.Path, ".pending-interrupted"), []byte("{"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := j.Attempts()
			if kind == "incomplete temp" {
				if err != nil || len(got) != 1 || !got[0].Unresolved() {
					t.Fatal(got, err)
				}
			} else if err == nil {
				t.Fatal("corruption accepted", got)
			}
			if err := j.StartAttempt(attemptFixture("b")); err == nil {
				t.Fatal("continued damaged or unresolved journal")
			}
		})
	}
}

func TestAttemptKilledWriter(t *testing.T) {
	if dir := os.Getenv("BREWWARDEN_TEST_ATTEMPT_CHILD"); dir != "" {
		if err := (Journal{Path: dir}).StartAttempt(attemptFixture("a")); err != nil {
			os.Exit(10)
		}
		// The parent sees this only after file and directory synchronization.
		_, _ = os.Stdout.WriteString("durable\n")
		for {
			time.Sleep(time.Second)
		}
	}
	dir := filepath.Join(t.TempDir(), "attempts")
	cmd := exec.Command(os.Args[0], "-test.run=^TestAttemptKilledWriter$")
	cmd.Env = append(os.Environ(), "BREWWARDEN_TEST_ATTEMPT_CHILD="+dir)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ready := make(chan error, 1)
	go func() {
		var buf [8]byte
		_, err := io.ReadFull(pipe, buf[:])
		if err == nil && string(buf[:]) != "durable\n" {
			err = errors.New("unexpected child marker")
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not persist start")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("writer was not interrupted")
	}
	j := Journal{Path: dir}
	records, err := j.Attempts()
	if err != nil || len(records) != 1 || !records[0].Unresolved() {
		t.Fatal(records, err)
	}
	if err := j.StartAttempt(attemptFixture("b")); err == nil {
		t.Fatal("lost attempt was replayable")
	}
	if err := j.FinishAttempt(domain.AttemptFinish{Attempt: records[0].Start.Binding.Attempt, FinishedAt: 1001, Outcome: domain.AttemptReconciled, AfterState: records[0].Start.BeforeState}); err != nil {
		t.Fatal(err)
	}
}

func TestAttemptOSLock(t *testing.T) {
	j := Journal{Path: filepath.Join(t.TempDir(), "attempts")}
	root, err := (Files{StatePath: j.Path}).openHistory(true)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	lock, err := openPrivate(root, ".writer.lock", os.O_CREATE|os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if err := j.StartAttempt(attemptFixture("a")); err == nil {
		t.Fatal("concurrent writer accepted")
	}
	if _, err := j.Attempts(); err == nil {
		t.Fatal("reader accepted an unstable inventory")
	}
}

func TestAttemptFullFilesystem(t *testing.T) {
	dir := os.Getenv("BREWWARDEN_TEST_FULL_DISK")
	if dir == "" {
		t.Skip("requires the isolated container's bounded tmpfs")
	}
	// Never fill a host filesystem. The container driver supplies a dedicated
	// 64 KiB tmpfs; verify its capacity before deliberately exhausting it.
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		t.Fatal(err)
	}
	if uint64(stat.Blocks)*uint64(stat.Bsize) > 128*1024 {
		t.Fatal("refusing to fill an unbounded filesystem")
	}
	root, err := os.MkdirTemp(dir, "journal-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	j := Journal{Path: filepath.Join(root, "attempts")}
	filler, err := os.Create(filepath.Join(root, "filler"))
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err = filler.Write(make([]byte, 4096))
		if err != nil {
			break
		}
	}
	_ = filler.Close()
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatal("expected a real full filesystem", err)
	}
	if err := j.StartAttempt(attemptFixture("a")); err == nil {
		t.Fatal("start persisted on full filesystem")
	}
	records, err := j.Attempts()
	if err != nil || len(records) != 0 {
		t.Fatal("failed start committed", records, err)
	}
	if err := os.Remove(filepath.Join(root, "filler")); err != nil {
		t.Fatal(err)
	}
	if err := j.StartAttempt(attemptFixture("a")); err != nil {
		t.Fatal("failed precommit write consumed attempt", err)
	}
}

func FuzzAttemptRecord(f *testing.F) {
	seed, err := json.Marshal(documentStart(attemptFixture("a")))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{"schemaVersion":1,"kind":"start","kind":"finish"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var doc attemptDocument
		if decodeStrict(data, &doc) != nil || !doc.valid() {
			return
		}
		encoded, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		var again attemptDocument
		if err := decodeStrict(encoded, &again); err != nil || again != doc || !again.valid() {
			t.Fatal("accepted record did not round trip", err)
		}
	})
}
