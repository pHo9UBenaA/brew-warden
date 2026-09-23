package tests

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/homebrew"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

func TestLivePublicCommandExecution(t *testing.T) {
	source := os.Getenv("BREWWARDEN_VM_PUBLIC_RUNTIME")
	names := strings.Fields(os.Getenv("BREWWARDEN_VM_PUBLIC_TARGETS"))
	if source == "" || len(names) == 0 {
		t.Skip("requires disposable VM, built runtime and explicit targets")
	}
	model, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.model").Output()
	if err != nil || !strings.HasPrefix(string(model), "VirtualMac") {
		t.Fatal("requires disposable VM")
	}
	directory, err := os.MkdirTemp(filepath.Dir(source), "public-execution-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("retained public execution", directory)
	collector := homebrew.Collector{Runtime: homebrew.Runtime{}, Directory: directory, BottleVerifier: installedBottleVerifier(t)}
	operation := os.Getenv("BREWWARDEN_VM_PUBLIC_OPERATION")
	if operation == "" {
		operation = "install"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	for pass := 0; pass < 2; pass++ {
		fault := os.Getenv("BREWWARDEN_VM_PUBLIC_FAULT")
		runContext, stopRun := context.WithCancel(ctx)
		defer stopRun()
		streams := homebrew.Streams{Out: os.Stdout, Err: os.Stderr}
		interrupt := &interruptOnPour{destination: os.Stdout, cancel: stopRun}
		if fault == "interrupt" {
			streams.Out, streams.Err = interrupt, interrupt
		}
		now := time.Now().Unix()
		collection, err := collector.Collect(ctx, ports.Request{Operation: operation, Targets: names}, now)
		if err != nil {
			t.Fatal(err)
		}
		prepared, session, err := collection.Prepare(ctx, domain.DefaultPolicy(), nil, time.Now().Unix(), streams)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		if pass == 0 {
			profiles, err := filepath.Glob(filepath.Join(directory, "collection-*", "public-execution.sb"))
			if err != nil || len(profiles) != 1 {
				t.Fatal("execution profile missing", err)
			}
			for _, protected := range []string{profiles[0], filepath.Join(filepath.Dir(profiles[0]), "plan.json")} {
				command := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", "-f", profiles[0], "/bin/sh", "-c", `printf changed >> "$1"`, "protection-test", protected)
				if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "Operation not permitted") {
					t.Fatal("installer could rewrite execution controls", string(output), err)
				}
			}
		}
		prepared.Assessment.Now = time.Now().Unix()
		if decision := domain.Evaluate(prepared.Assessment); decision.Outcome != domain.Allow {
			t.Fatal(decision)
		}
		if _, err := session.Revalidate(ctx); err != nil {
			t.Fatal(err)
		}
		unrelatedBefore := publicUnrelatedKegs(t, prepared.Assessment.Nodes)
		if fault == "interrupt" {
			if result, err := session.Run(runContext, prepared.Assessment.Binding); err == nil || !interrupt.triggered || !result.ExitKnown || result.ExitCode == 0 {
				t.Fatal("installer did not reach the interruption fixture", err)
			}
			if !maps.Equal(unrelatedBefore, publicUnrelatedKegs(t, prepared.Assessment.Nodes)) {
				t.Fatal("interrupted execution changed unrelated packages")
			}
			if err := session.Close(); err != nil && !strings.Contains(err.Error(), "owned Homebrew process") {
				t.Fatal(err)
			}
			// There is no saved-plan replay. An active descendant can outlive
			// the cancelled parent. Wait for exclusion to end, then re-collect.
			engine := homebrew.Engine{Collector: &collector}
			deadline := time.Now().Add(2 * time.Minute)
			for {
				newPlan, next, err := engine.Prepare(ctx, ports.Request{Operation: operation, Targets: names}, domain.DefaultPolicy(), nil, time.Now().Unix())
				if err == nil {
					defer next.Close()
					if newPlan.Assessment.Binding.Attempt == prepared.Assessment.Binding.Attempt {
						t.Fatal("interrupted plan was replayed")
					}
					break
				}
				if time.Now().After(deadline) || (!strings.Contains(err.Error(), "owned Homebrew process") && !strings.Contains(err.Error(), "another BrewWarden execution")) {
					t.Fatal("fresh collection after interruption failed", err)
				}
				time.Sleep(200 * time.Millisecond)
			}
			return
		}
		if fault != "" {
			before, err := kegSnapshot("/opt/homebrew/Cellar")
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "changed-input":
				files, err := filepath.Glob(filepath.Join(directory, "collection-*", "inputs", "*.tar.gz"))
				if err != nil || len(files) == 0 {
					t.Fatal("missing bottle fixture", err)
				}
				if err := os.WriteFile(files[0], []byte("changed after verification"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-cache":
				files, err := filepath.Glob(filepath.Join(directory, "collection-*", "fetch.json"))
				if err != nil || len(files) != 1 {
					t.Fatal("missing fetch fixture", err)
				}
				raw, err := os.ReadFile(files[0])
				if err != nil {
					t.Fatal(err)
				}
				var fetch struct{ Downloads []struct{ Path string } }
				if err := json.Unmarshal(raw, &fetch); err != nil || len(fetch.Downloads) == 0 {
					t.Fatal("invalid fetch fixture", err)
				}
				path := fetch.Downloads[0].Path
				if !strings.HasPrefix(path, filepath.Dir(files[0])+"/cache/downloads/") {
					t.Fatal("fixture path outside workspace")
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				cancelled, stop := context.WithCancel(ctx)
				stop()
				runContext = cancelled
			default:
				t.Fatal("unsupported public execution fault")
			}
			if _, err := session.Run(runContext, prepared.Assessment.Binding); err == nil {
				t.Fatal("fault did not stop execution")
			}
			if _, err := session.Run(ctx, prepared.Assessment.Binding); err == nil {
				t.Fatal("failed attempt was reusable")
			}
			after, err := kegSnapshot("/opt/homebrew/Cellar")
			if err != nil || after != before {
				t.Fatal("failed preflight changed installed payload", err)
			}
			return
		}
		result, err := session.Run(ctx, prepared.Assessment.Binding)
		if err != nil || !result.ExitKnown || result.ExitCode != 0 || !result.MatchesPlan || !result.AfterState.Valid() {
			t.Fatal(result, err)
		}
		if !maps.Equal(unrelatedBefore, publicUnrelatedKegs(t, prepared.Assessment.Nodes)) {
			t.Fatal("execution changed an unrelated installed package")
		}
		if pass == 1 && result.AfterState != prepared.BeforeState {
			t.Fatal("unchanged rerun changed selected state")
		}
		if _, err := session.Run(ctx, prepared.Assessment.Binding); err == nil {
			t.Fatal("attempt replay accepted")
		}
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

type interruptOnPour struct {
	mu          sync.Mutex
	destination io.Writer
	cancel      context.CancelFunc
	tail        string
	triggered   bool
}

func (w *interruptOnPour) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tail += string(data)
	if !w.triggered && strings.Contains(w.tail, "Fetching downloads for:") {
		w.triggered = true
		w.cancel()
	}
	if len(w.tail) > 4096 {
		w.tail = w.tail[len(w.tail)-4096:]
	}
	return w.destination.Write(data)
}

func publicUnrelatedKegs(t *testing.T, nodes []domain.Node) map[string]string {
	t.Helper()
	planned := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		planned[node.Artifact.Name] = true
	}
	entries, err := os.ReadDir("/opt/homebrew/Cellar")
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for _, entry := range entries {
		if planned[entry.Name()] {
			continue
		}
		digest, err := kegSnapshot(filepath.Join("/opt/homebrew/Cellar", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		result[entry.Name()] = digest
	}
	return result
}
