package homebrew

import (
	"context"
	"encoding/json"
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
	valid, _ := json.Marshal(inFlightFixture(500))
	for _, seed := range [][]byte{valid, []byte(`{"schema":1,"collection":"../outside"}`), []byte(`{"schema":1,"pid":500,"session":500}`), []byte("{")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 2048 {
			return
		}
		var record inFlight
		if err := decodeStrict(raw, &record); err == nil && record.valid() && filepath.Base(record.Collection) != record.Collection {
			t.Fatal("record accepted a workspace outside the private collection root")
		}
	})
}

func TestLegacyJournalCannotBeSilentlyMigrated(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "attempts")
	if err := os.Mkdir(journal, 0700); err != nil {
		t.Fatal(err)
	}
	engine := Engine{Collector: &Collector{LegacyState: journal}}
	if _, _, err := engine.Prepare(context.Background(), ports.Request{Operation: "install", Targets: []string{"jq"}}, domain.DefaultPolicy(), nil, time.Now().Unix()); err == nil || !strings.Contains(err.Error(), "matching older build") {
		t.Fatal("legacy execution state was bypassed", err)
	}
}

func TestInFlightRejectsCorruptAndUnsafeRecords(t *testing.T) {
	for _, tc := range []struct {
		name, data string
	}{
		{"malformed", "{"},
		{"missing identity", `{"schema":1,"plan":"","attempt":"","pid":500,"session":500}`},
		{"unbound session", `{"schema":1,"plan":"` + strings.Repeat("a", 64) + `","attempt":"` + strings.Repeat("b", 64) + `","pid":500,"session":501}`},
		{"unknown schema", `{"schema":99,"plan":"` + strings.Repeat("a", 64) + `","attempt":"` + strings.Repeat("b", 64) + `","pid":500,"session":500}`},
		{"workspace traversal", `{"schema":1,"plan":"` + strings.Repeat("a", 64) + `","attempt":"` + strings.Repeat("b", 64) + `","pid":500,"session":500,"collection":"../outside"}`},
		{"workspace substitution", `{"schema":1,"plan":"` + strings.Repeat("a", 64) + `","attempt":"` + strings.Repeat("b", 64) + `","pid":500,"session":500,"collection":"collection-trap"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "inflight.json"), []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			if err := clearStoppedInFlight(directory); err == nil {
				t.Fatal("unsafe in-flight state permitted a new mutation")
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
	if err := os.Mkdir(filepath.Join(directory, "collection-12345"), 0700); err != nil {
		os.Exit(4)
	}
	record := inFlightFixture(child.Process.Pid)
	if os.Getenv("BREWWARDEN_INFLIGHT_PENDING") == "1" {
		raw, _ := json.Marshal(record)
		if err := writeNew(filepath.Join(directory, "inflight.pending"), raw, 0600); err != nil {
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
		t.Run(map[bool]string{false: "committed", true: "pending"}[pending], func(t *testing.T) {
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
				t.Fatal("missing durable in-flight record", err)
			}
			defer func() { _ = syscall.Kill(-record.PID, syscall.SIGKILL) }()
			if err := clearStoppedInFlight(directory); err == nil {
				t.Fatal("another mutation was allowed while child continued")
			}
			if err := syscall.Kill(-record.PID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				if err := clearStoppedInFlight(directory); err == nil {
					break
				} else if time.Now().After(deadline) {
					t.Fatal("stopped execution did not permit a complete fresh retry", err)
				}
				time.Sleep(50 * time.Millisecond)
			}
			if record, err := readInFlight(filepath.Join(directory, name)); err != nil || record != nil {
				t.Fatal("stale record was not removed", err)
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
	if err := os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
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
