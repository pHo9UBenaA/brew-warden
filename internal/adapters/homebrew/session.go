package homebrew

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

type Streams struct {
	In       io.Reader
	Out, Err io.Writer
}
type plannedAction struct {
	Name      string `json:"name" required:"true"`
	Operation string `json:"operation" required:"true"`
}
type frozenInput struct {
	Path   string        `json:"path" required:"true"`
	SHA256 domain.Digest `json:"sha256" required:"true"`
}
type executionEnvironment struct {
	Runtime   domain.Digest `json:"runtime" required:"true"`
	OSVersion string        `json:"osVersion" required:"true"`
	Prefix    string        `json:"prefix" required:"true"`
}
type executionPlan struct {
	Schema      int                  `json:"schema" required:"true"`
	MinimumAge  int64                `json:"minimumAge" required:"true"`
	Targets     []domain.Artifact    `json:"targets" required:"true"`
	Nodes       []domain.Node        `json:"nodes" required:"true"`
	Actions     []plannedAction      `json:"actions" required:"true"`
	BeforeState domain.Digest        `json:"beforeState" required:"true"`
	Environment executionEnvironment `json:"environment" required:"true"`
	Inputs      []frozenInput        `json:"inputs" required:"true"`
	Attempt     domain.Digest        `json:"attempt" required:"true"`
	IssuedAt    int64                `json:"issuedAt" required:"true"`
	ExpiresAt   int64                `json:"expiresAt" required:"true"`
	Waivers     []domain.AgeWaiver   `json:"waivers" required:"true"`
}

func (p executionPlan) prepared(id domain.Digest) (ports.Prepared, error) {
	policy, err := domain.NewPolicy(p.MinimumAge)
	if err != nil || (p.Schema != 1 && p.Schema != 2 && p.Schema != 3) || !p.Attempt.Valid() || !p.BeforeState.Valid() || p.IssuedAt <= 0 || p.ExpiresAt <= p.IssuedAt || p.ExpiresAt > p.IssuedAt+600 {
		return ports.Prepared{}, errors.New("invalid persisted execution plan")
	}
	if !p.Environment.Runtime.Valid() || p.Environment.Prefix != "/opt/homebrew" || !strings.HasPrefix(p.Environment.OSVersion, "26.") || len(p.Environment.OSVersion) > 16 || strings.Trim(p.Environment.OSVersion, "0123456789.") != "" || len(p.Nodes) == 0 || len(p.Nodes) > 128 || len(p.Actions) != len(p.Nodes) || len(p.Inputs) == 0 || len(p.Inputs) > 10000 {
		return ports.Prepared{}, errors.New("unsupported persisted execution environment or inventory")
	}
	inventory := map[string]domain.Digest{}
	for _, file := range p.Inputs {
		if !safeRelative(file.Path) || !file.SHA256.Valid() || inventory[file.Path] != "" {
			return ports.Prepared{}, errors.New("invalid persisted input inventory")
		}
		inventory[file.Path] = file.SHA256
	}
	for i, node := range p.Nodes {
		action := p.Actions[i]
		if action.Name != node.Artifact.Name || action.Operation != "install" && action.Operation != "upgrade" && action.Operation != "keep" {
			return ports.Prepared{}, errors.New("invalid persisted action graph")
		}
		for _, e := range node.Evidence {
			if _, err := domain.NewEvidence(e); err != nil {
				return ports.Prepared{}, err
			}
			if inventory["observations/"+string(e.RawSHA256)+".json"] != e.RawSHA256 {
				return ports.Prepared{}, errors.New("persisted evidence observation missing")
			}
		}
	}
	policyBytes, _ := json.Marshal(struct{ MinimumAge int64 }{p.MinimumAge})
	graph := []struct {
		Artifact     domain.Artifact
		Dependencies []domain.Artifact
	}{}
	for _, node := range p.Nodes {
		graph = append(graph, struct {
			Artifact     domain.Artifact
			Dependencies []domain.Artifact
		}{node.Artifact, node.Dependencies})
	}
	graphBytes, _ := json.Marshal(graph)
	environmentBytes, _ := json.Marshal(p.Environment)
	binding := domain.Binding{Plan: id, Policy: digestBytes(policyBytes), Graph: digestBytes(graphBytes), Environment: digestBytes(environmentBytes), Attempt: p.Attempt}
	result := ports.Prepared{Assessment: domain.Assessment{Policy: policy, Binding: binding, Targets: p.Targets, Nodes: p.Nodes}, BeforeState: p.BeforeState, ExpiresAt: p.ExpiresAt}
	if len(p.Waivers) > 0 {
		exception := domain.AgeException{Binding: binding, IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt, Waivers: p.Waivers}
		raw, _ := json.Marshal(exception)
		result.ExceptionID = digestBytes(raw)
		result.Assessment.Exception = &exception
	}
	return result, nil
}
func (w workspace) freezeInputs() ([]frozenInput, error) {
	names := []string{"inputs.json", metadataCachePath, "metadata.json", "fetch.json"}
	for _, directory := range []string{"inputs", "observations", "cache/downloads"} {
		files, err := os.ReadDir(filepath.Join(w.root, directory))
		if err != nil {
			return nil, err
		}
		parent, err := os.Open(filepath.Join(w.root, directory))
		if err != nil {
			return nil, err
		}
		syncErr := parent.Sync()
		closeErr := parent.Close()
		if syncErr != nil || closeErr != nil {
			return nil, errors.New("frozen directory synchronization failed")
		}
		for _, file := range files {
			info, err := file.Info()
			if err != nil {
				return nil, err
			}
			if info.Mode().IsRegular() {
				names = append(names, filepath.Join(directory, file.Name()))
			} else if info.Mode()&os.ModeSymlink == 0 {
				return nil, errors.New("unexpected frozen input type")
			}
		}
	}
	if len(names) > 10000 {
		return nil, errors.New("frozen input inventory exceeds limit")
	}
	result := []frozenInput{}
	for _, name := range names {
		raw, err := readRegular(filepath.Join(w.root, name), 128*1024*1024)
		if err != nil {
			return nil, err
		}
		result = append(result, frozenInput{filepath.ToSlash(name), digestBytes(raw)})
		file, err := os.Open(filepath.Join(w.root, name))
		if err != nil {
			return nil, err
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil || closeErr != nil {
			return nil, errors.New("frozen input synchronization failed")
		}
	}
	return result, nil
}
func (w workspace) readPrepared(expected domain.Digest) (ports.Prepared, error) {
	raw, err := readRegular(filepath.Join(w.root, "plan.json"), maxManifest)
	if err != nil || digestBytes(raw) != expected {
		return ports.Prepared{}, errors.New("saved plan changed")
	}
	var plan executionPlan
	if err := decodeStrict(raw, &plan); err != nil {
		return ports.Prepared{}, err
	}
	for _, file := range plan.Inputs {
		if !safeRelative(file.Path) || !file.SHA256.Valid() {
			return ports.Prepared{}, errors.New("invalid frozen input")
		}
		raw, err := readRegular(filepath.Join(w.root, file.Path), 128*1024*1024)
		if err != nil || digestBytes(raw) != file.SHA256 {
			return ports.Prepared{}, errors.New("frozen input changed")
		}
	}
	snapshot, err := readRegular(filepath.Join(w.root, "states", string(plan.BeforeState)+".json"), maxManifest)
	if err != nil || digestBytes(snapshot) != plan.BeforeState {
		return ports.Prepared{}, errors.New("saved before-state unavailable")
	}
	return plan.prepared(digestBytes(raw))
}
