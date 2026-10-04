package homebrew

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

func inFlightFixture(pid int) inFlight {
	return inFlight{Schema: 1, Plan: domain.Digest(strings.Repeat("a", 64)), Attempt: domain.Digest(strings.Repeat("b", 64)), PID: pid, Session: pid, Collection: "collection-12345"}
}

func FuzzInFlightRecord(f *testing.F) {
	valid := marshalFixture(f, inFlightFixture(500))
	for _, seed := range [][]byte{valid, []byte(`{"schema":1,"collection":"../outside"}`), []byte(`{"schema":1,"pid":500,"session":500}`), []byte("{")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 2048 {
			return
		}
		var record inFlight
		if err := decodeStrict(raw, &record); err == nil && record.valid() && filepath.Base(record.Collection) != record.Collection {
			t.Fatalf("accepted collection path outside private root: record=%+v", record)
		}
	})
}

func TestLegacyJournalCannotBeSilentlyMigrated(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "attempts")
	if err := os.Mkdir(journal, 0o700); err != nil {
		t.Fatal(err)
	}
	engine := Engine{Collector: &Collector{LegacyState: journal}}
	request := ports.Request{Operation: "install", Targets: []string{"jq"}}
	_, _, err := engine.Prepare(context.Background(), request, domain.DefaultPolicy(), nil, time.Now().Unix())
	if err == nil || !strings.Contains(err.Error(), "matching older build") {
		t.Fatal("legacy execution state was bypassed", err)
	}
}

func TestInFlightRejectsCorruptAndUnsafeRecords(t *testing.T) {
	fixture := inFlightFixture(500)
	if !fixture.valid() {
		t.Fatal("invalid baseline in-flight fixture", fixture)
	}
	valid := string(marshalFixture(t, fixture))
	for _, tc := range []struct {
		name, data string
	}{
		{"malformed", "{"},
		{"missing identity", replaceFixtureText(t, valid, `"plan":"`+strings.Repeat("a", 64)+`"`, `"plan":""`)},
		{"unbound session", replaceFixtureText(t, valid, `"session":500`, `"session":501`)},
		{"unknown schema", replaceFixtureText(t, valid, `"schema":1`, `"schema":99`)},
		{"workspace traversal", replaceFixtureText(t, valid, `"collection":"collection-12345"`, `"collection":"../outside"`)},
		{"workspace substitution", replaceFixtureText(t, valid, `"collection":"collection-12345"`, `"collection":"collection-trap"`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "inflight.json"), []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := clearStoppedInFlight(directory); err == nil || !strings.Contains(err.Error(), "invalid in-flight execution record") {
				t.Fatal("want invalid-record refusal before process inspection or cleanup", err)
			}
		})
	}
	directory := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(directory, "inflight.json")); err != nil {
		t.Fatal(err)
	}
	if err := clearStoppedInFlight(directory); err == nil {
		t.Fatal("symlink in-flight record accepted")
	}
}

// Run the writer in a separate process and exit it without waiting for the
// owned child. This tests a real parent death, not an in-memory fake port.
func TestInFlightParentExitHelper(t *testing.T) {
	if os.Getenv("BREWWARDEN_INFLIGHT_HELPER") != "1" {
		return
	}
	directory := os.Getenv("BREWWARDEN_INFLIGHT_DIR")
	child := exec.Command("/bin/sh", "-c", "exec /bin/sleep 10")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	if err := os.Mkdir(filepath.Join(directory, "collection-12345"), 0o700); err != nil {
		os.Exit(4)
	}
	record := inFlightFixture(child.Process.Pid)
	if os.Getenv("BREWWARDEN_INFLIGHT_PENDING") == "1" {
		raw := marshalFixture(t, record)
		if err := writeNew(filepath.Join(directory, "inflight.pending"), raw, 0o600); err != nil {
			_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
			os.Exit(3)
		}
	} else if err := saveInFlight(directory, record); err != nil {
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
		os.Exit(3)
	}
	// No child.Wait and no outcome record: the child continues after us.
	os.Exit(0)
}

func TestInFlightParentDeathAndFreshRetry(t *testing.T) {
	for _, pending := range []bool{false, true} {
		name := "committed"
		if pending {
			name = "pending"
		}
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			writer := exec.Command(os.Args[0], "-test.run=^TestInFlightParentExitHelper$")
			writer.Env = append(os.Environ(), "BREWWARDEN_INFLIGHT_HELPER=1", "BREWWARDEN_INFLIGHT_DIR="+directory)
			name := "inflight.json"
			if pending {
				writer.Env = append(writer.Env, "BREWWARDEN_INFLIGHT_PENDING=1")
				name = "inflight.pending"
			}
			if err := writer.Run(); err != nil {
				t.Fatal("writer did not record owned child before exiting", err)
			}
			record, err := readInFlight(filepath.Join(directory, name))
			if err != nil || record == nil {
				t.Fatalf("missing durable %s record: record=%+v error=%v", name, record, err)
			}
			defer func() { _ = syscall.Kill(-record.PID, syscall.SIGKILL) }()
			if err := clearStoppedInFlight(directory); err == nil {
				t.Fatal("another mutation was allowed while child continued")
			}
			if err := syscall.Kill(-record.PID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				t.Fatal(err)
			}
			// The system may retain a dead orphan briefly while it is reaped,
			// and a busy process table can force conservative retry snapshots.
			deadline := time.Now().Add(30 * time.Second)
			for {
				if err := clearStoppedInFlight(directory); err == nil {
					break
				} else if time.Now().After(deadline) {
					t.Fatal("stopped execution did not permit a complete fresh retry", err)
				}
				time.Sleep(50 * time.Millisecond)
			}
			if record, err := readInFlight(filepath.Join(directory, name)); err != nil || record != nil {
				t.Fatalf("stale %s record was not removed: record=%+v error=%v", name, record, err)
			}
			if _, err := os.Lstat(filepath.Join(directory, "collection-12345")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("stale workspace was not removed", err)
			}
		})
	}
}

func TestInFlightDoesNotDeleteReplacedWorkspace(t *testing.T) {
	directory := t.TempDir()
	outside := t.TempDir()
	marker := filepath.Join(outside, "marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "collection-12345")); err != nil {
		t.Fatal(err)
	}
	if err := saveInFlight(directory, inFlightFixture(1<<28)); err != nil {
		t.Fatal(err)
	}
	if err := clearStoppedInFlight(directory); err == nil {
		t.Fatal("replaced workspace was followed or silently removed")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("escaped the private collection root", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "inflight.json")); err != nil {
		t.Fatal("unsafe state was discarded", err)
	}
}
