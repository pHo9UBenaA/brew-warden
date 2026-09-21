package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

type processRecord struct {
	PID     int `json:"pid" required:"true"`
	Session int `json:"session" required:"true"`
}

func processSessionActive(session int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for attempt := 0; attempt < 3; attempt++ {
		command := exec.CommandContext(ctx, "/bin/ps", "-U", strconv.Itoa(os.Getuid()), "-o", "pid=")
		command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
		output, diagnostics := &processOutput{}, &processOutput{}
		command.Stdout, command.Stderr = output, diagnostics
		if err := command.Run(); err != nil || output.overflow || diagnostics.overflow {
			return false, errors.New("owned process inventory unavailable")
		}
		fields := strings.Fields(output.String())
		if len(fields) == 0 || len(fields) > 100000 {
			return false, errors.New("invalid owned process inventory")
		}
		active, changed, err := sessionInProcesses(session, command.Process.Pid, fields)
		if err != nil || active {
			return active, err
		}
		if !changed {
			return false, nil
		}
	}
	return false, errors.New("process inventory changed during observation")
}

func sessionInProcesses(session, observer int, fields []string) (active, changed bool, err error) {
	for _, field := range fields {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 0 {
			return false, false, errors.New("invalid observed process ID")
		}
		if pid == observer {
			continue
		}
		sid, err := syscall.Getsid(pid)
		if errors.Is(err, syscall.ESRCH) {
			changed = true
			continue
		}
		if err != nil {
			return false, false, errors.New("owned process session unavailable")
		}
		if sid == session {
			return true, changed, nil
		}
	}
	return false, changed, nil
}

// Observe only sessions recorded by our startup gate. PID reuse conservatively holds
// recovery; no remembered process is signalled or presumed to have succeeded.
func stoppedProcessSessions(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) > 10000 {
		return errors.New("attempt process inventory unavailable")
	}
	count := 0
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "process-") {
			continue
		}
		index := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "process-"), ".json")
		n, err := strconv.Atoi(index)
		if err != nil || n < 0 || n >= 128 || entry.Name() != fmt.Sprintf("process-%d.json", n) || count >= 128 {
			return errors.New("invalid attempt process record")
		}
		count++
		raw, err := readRegular(filepath.Join(root, entry.Name()), 1024)
		if err != nil {
			return err
		}
		var record processRecord
		if err := decodeStrict(raw, &record); err != nil || record.PID <= 1 || record.PID > 1<<30 || record.Session != record.PID {
			return errors.New("invalid recorded process session")
		}
		if active, err := processSessionActive(record.Session); err != nil || active {
			return errors.New("recorded BrewWarden process session is still active or unavailable")
		}
	}
	return nil
}

type observedPath struct {
	Path   string        `json:"path"`
	Kind   string        `json:"kind"`
	Mode   uint32        `json:"mode"`
	Size   int64         `json:"size"`
	SHA256 domain.Digest `json:"sha256,omitempty"`
	Target string        `json:"target,omitempty"`
}

// Inventory selected racks and opt links without parsing possibly interrupted
// receipts or following links. This records facts, not an installation verdict.
func observeSelectedPaths(ctx context.Context, prefix string, nodes []domain.Node) ([]byte, error) {
	result := []observedPath{}
	var total int64
	for _, node := range nodes {
		if !node.Artifact.Valid() {
			return nil, errors.New("invalid recovery candidate")
		}
		for _, base := range []string{"Cellar", "opt"} {
			root := filepath.Join(prefix, base, node.Artifact.Name)
			err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if len(result) >= 100000 {
					return errors.New("recovery inventory exceeds limit")
				}
				relative, err := filepath.Rel(prefix, path)
				if err != nil || !safeRelative(filepath.ToSlash(relative)) {
					return errors.New("invalid recovery path")
				}
				item := observedPath{Path: filepath.ToSlash(relative)}
				if errors.Is(walkErr, os.ErrNotExist) && path == root {
					item.Kind = "absent"
				} else if walkErr != nil {
					return walkErr
				} else {
					info, err := entry.Info()
					if err != nil {
						return err
					}
					item.Mode, item.Size = uint32(info.Mode()), info.Size()
					switch {
					case info.IsDir():
						item.Kind = "directory"
						item.Size = 0
					case info.Mode()&os.ModeSymlink != 0:
						item.Kind = "symlink"
						item.Target, err = os.Readlink(path)
						if err != nil {
							return err
						}
					case info.Mode().IsRegular():
						item.Kind = "file"
						total += info.Size()
						if total > 2*1024*1024*1024 {
							return errors.New("recovery payload exceeds limit")
						}
						raw, err := readRegular(path, 128*1024*1024)
						if err != nil {
							return err
						}
						item.SHA256 = digestBytes(raw)
					default:
						return errors.New("unsupported recovery filesystem entry")
					}
				}
				result = append(result, item)
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > maxManifest {
		return nil, errors.New("recovery observation exceeds limit")
	}
	return raw, nil
}

func (e Engine) snapshotPublic(ctx context.Context, plan executionPlan, attemptRoot string) (domain.Digest, error) {
	lock, err := acquireOperationLock(e.Collector.Directory)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := stoppedProcessSessions(attemptRoot); err != nil {
		return "", err
	}
	raw, err := observeSelectedPaths(ctx, plan.Environment.Prefix, plan.Nodes)
	if err != nil {
		return "", err
	}
	root, err := os.MkdirTemp(e.Collector.Directory, "recovery-")
	if err != nil {
		return "", err
	}
	id := digestBytes(raw)
	if err := writeRecord(filepath.Join(root, string(id)+".json"), raw); err != nil {
		return "", err
	}
	return id, nil
}
