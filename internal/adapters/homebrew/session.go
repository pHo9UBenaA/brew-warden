package homebrew

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

type Streams struct {
	In       io.Reader
	Out, Err io.Writer
}
type nativeAction struct {
	Name      string `json:"name" required:"true"`
	Operation string `json:"operation" required:"true"`
}
type nativeReady struct {
	Schema      int            `json:"schema" required:"true"`
	BeforeState domain.Digest  `json:"beforeState" required:"true"`
	Actions     []nativeAction `json:"actions" required:"true"`
	OSVersion   string         `json:"osVersion" required:"true"`
}
type frozenInput struct {
	Path   string        `json:"path" required:"true"`
	SHA256 domain.Digest `json:"sha256" required:"true"`
}
type nativeSeal struct {
	Schema      int           `json:"schema" required:"true"`
	Plan        domain.Digest `json:"plan" required:"true"`
	BeforeState domain.Digest `json:"beforeState" required:"true"`
	ExpiresAt   int64         `json:"expiresAt" required:"true"`
	IssuedAt    int64         `json:"issuedAt" required:"true"`
	Inputs      []frozenInput `json:"inputs" required:"true"`
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
	Actions     []nativeAction       `json:"actions" required:"true"`
	BeforeState domain.Digest        `json:"beforeState" required:"true"`
	Environment executionEnvironment `json:"environment" required:"true"`
	Inputs      []frozenInput        `json:"inputs" required:"true"`
	Attempt     domain.Digest        `json:"attempt" required:"true"`
	IssuedAt    int64                `json:"issuedAt" required:"true"`
	ExpiresAt   int64                `json:"expiresAt" required:"true"`
	Waivers     []domain.AgeWaiver   `json:"waivers" required:"true"`
}
type nativeSession struct {
	w        workspace
	command  *exec.Cmd
	cancel   context.CancelFunc
	done     chan struct{}
	waitErr  error
	sequence int
	prepared ports.Prepared
	consumed bool
}

func (c *Collection) Prepare(ctx context.Context, policy domain.Policy, waivers []domain.AgeWaiver, now int64, streams Streams) (ports.Prepared, ports.ExecutionSession, error) {
	if c == nil || ctx == nil || !policy.Valid() || now < c.observedAt || now >= c.observedAt+3600 || len(c.nodes) == 0 {
		return ports.Prepared{}, nil, errors.New("collection is unavailable or expired")
	}
	s := &nativeSession{w: workspace{c.root}, sequence: 1, done: make(chan struct{})}
	prepared, err := s.prepare(ctx, c, policy, waivers, now, streams)
	if err != nil {
		_ = s.Close()
		return ports.Prepared{}, nil, err
	}
	return prepared, s, nil
}
func (s *nativeSession) prepare(ctx context.Context, c *Collection, policy domain.Policy, waivers []domain.AgeWaiver, now int64, streams Streams) (ports.Prepared, error) {
	files, err := s.w.freezeInputs(c.inputs)
	if err == nil && !slices.Equal(files, c.frozen) {
		err = errors.New("collected inputs changed before planning")
	}
	if err != nil {
		return ports.Prepared{}, err
	}
	if err := os.Mkdir(filepath.Join(s.w.root, "control"), 0700); err != nil {
		return ports.Prepared{}, err
	}
	immutable := []string{filepath.Join(s.w.root, "runtime/brew/Library"), filepath.Join(s.w.root, "runtime/brew/bin"), filepath.Join(s.w.root, "runtime/verifier"), filepath.Join(s.w.root, "plan.json"), filepath.Join(s.w.root, "seal.json")}
	for _, file := range files {
		immutable = append(immutable, filepath.Join(s.w.root, file.Path))
	}
	profile, err := s.w.sandbox("execution", false, true, immutable)
	if err != nil {
		return ports.Prepared{}, err
	}
	processContext, cancel := context.WithTimeout(ctx, 15*time.Minute)
	s.cancel = cancel
	s.command = s.w.command(processContext, profile, "ruby", filepath.Join(s.w.root, "bootstrap.rb"), "/opt/homebrew", filepath.Join(s.w.root, "session.rb"), s.w.root)
	s.command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	s.command.Cancel = func() error { return syscall.Kill(-s.command.Process.Pid, syscall.SIGKILL) }
	s.command.Stdin, s.command.Stdout, s.command.Stderr = streams.In, streams.Out, streams.Err
	if err := s.command.Start(); err != nil {
		return ports.Prepared{}, errors.New("cannot start native session")
	}
	go func() { s.waitErr = s.command.Wait(); close(s.done) }()
	raw, err := s.event(ctx, 0)
	if err != nil {
		return ports.Prepared{}, err
	}
	var ready nativeReady
	if err := decodeStrict(raw, &ready); err != nil || ready.Schema != 1 || !ready.BeforeState.Valid() || len(ready.Actions) != len(c.nodes) || ready.OSVersion == "" {
		return ports.Prepared{}, errors.New("invalid native ready state")
	}
	for i, action := range ready.Actions {
		if action.Name != c.nodes[i].Artifact.Name || action.Operation != "install" && action.Operation != "upgrade" && action.Operation != "keep" {
			return ports.Prepared{}, errors.New("native action plan mismatch")
		}
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return ports.Prepared{}, err
	}
	plan := executionPlan{Schema: 1, MinimumAge: policy.MinimumAgeSeconds(), Nodes: c.Evidence(), Actions: ready.Actions, BeforeState: ready.BeforeState, Environment: executionEnvironment{c.runtimeDigest, ready.OSVersion, "/opt/homebrew"}, Inputs: files, Attempt: domain.Digest(hex.EncodeToString(nonce)), IssuedAt: now, ExpiresAt: min(now+600, c.observedAt+3600), Waivers: append([]domain.AgeWaiver{}, waivers...), Targets: []domain.Artifact{}}
	for _, name := range c.inputs.Targets {
		for _, node := range plan.Nodes {
			if node.Artifact.Name == name {
				plan.Targets = append(plan.Targets, node.Artifact)
			}
		}
	}
	raw, err = json.Marshal(plan)
	if err != nil {
		return ports.Prepared{}, err
	}
	if err := writeRecord(filepath.Join(s.w.root, "plan.json"), raw); err != nil {
		return ports.Prepared{}, err
	}
	s.prepared, err = plan.prepared(digestBytes(raw))
	if err != nil {
		return ports.Prepared{}, err
	}
	sealBytes, _ := json.Marshal(nativeSeal{Schema: 1, Plan: s.prepared.Assessment.Binding.Plan, BeforeState: ready.BeforeState, ExpiresAt: plan.ExpiresAt, IssuedAt: plan.IssuedAt, Inputs: files})
	if err := writeRecord(filepath.Join(s.w.root, "seal.json"), sealBytes); err != nil {
		return ports.Prepared{}, err
	}
	return s.prepared, nil
}
func (p executionPlan) prepared(id domain.Digest) (ports.Prepared, error) {
	policy, err := domain.NewPolicy(p.MinimumAge)
	if err != nil || p.Schema != 1 || !p.Attempt.Valid() || !p.BeforeState.Valid() || p.IssuedAt <= 0 || p.ExpiresAt <= p.IssuedAt || p.ExpiresAt > p.IssuedAt+600 {
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
func (w workspace) freezeInputs(inputs nativeInputs) ([]frozenInput, error) {
	names := []string{"native-inputs.json", metadataCachePath, "metadata.json", "fetch.json", "inspect.json"}
	scripts, err := nativeScripts.ReadDir(".")
	if err != nil {
		return nil, err
	}
	for _, script := range scripts {
		names = append(names, script.Name())
	}
	for _, recipe := range inputs.Recipes {
		names = append(names, filepath.Join("runtime/brew/Library/Taps/homebrew/homebrew-core", recipe.RecipePath))
	}
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
func (s *nativeSession) readPrepared() (ports.Prepared, error) {
	raw, err := readRegular(filepath.Join(s.w.root, "plan.json"), maxManifest)
	if err != nil || digestBytes(raw) != s.prepared.Assessment.Binding.Plan {
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
		raw, err := readRegular(filepath.Join(s.w.root, file.Path), 128*1024*1024)
		if err != nil || digestBytes(raw) != file.SHA256 {
			return ports.Prepared{}, errors.New("frozen input changed")
		}
	}
	snapshot, err := readRegular(filepath.Join(s.w.root, "states", string(plan.BeforeState)+".json"), maxManifest)
	if err != nil || digestBytes(snapshot) != plan.BeforeState {
		return ports.Prepared{}, errors.New("saved before-state unavailable")
	}
	return plan.prepared(digestBytes(raw))
}
func (s *nativeSession) Revalidate(ctx context.Context) (ports.Prepared, error) {
	if s.consumed {
		return ports.Prepared{}, errors.New("session already consumed")
	}
	fresh, err := s.readPrepared()
	if err != nil {
		return ports.Prepared{}, err
	}
	raw, err := s.exchange(ctx, "revalidate")
	if err != nil {
		return ports.Prepared{}, err
	}
	var event struct {
		Schema      int           `json:"schema" required:"true"`
		BeforeState domain.Digest `json:"beforeState" required:"true"`
	}
	if err := decodeStrict(raw, &event); err != nil || event.Schema != 1 || event.BeforeState != fresh.BeforeState {
		return ports.Prepared{}, errors.New("native state changed")
	}
	return fresh, nil
}
func (s *nativeSession) Run(ctx context.Context, binding domain.Binding) (ports.ExecutionResult, error) {
	if s.consumed || binding != s.prepared.Assessment.Binding {
		return ports.ExecutionResult{}, errors.New("session binding mismatch or replay")
	}
	s.consumed = true
	if _, err := s.readPrepared(); err != nil {
		return ports.ExecutionResult{}, err
	}
	raw, err := s.exchange(ctx, "execute")
	if err != nil {
		return s.knownExit(), err
	}
	var event struct {
		Schema      int           `json:"schema" required:"true"`
		ExitCode    int           `json:"exitCode" required:"true"`
		AfterState  domain.Digest `json:"afterState" required:"true"`
		MatchesPlan bool          `json:"matchesPlan" required:"true"`
	}
	if err := decodeStrict(raw, &event); err != nil || event.Schema != 1 || event.ExitCode < 0 || event.ExitCode > 255 {
		return ports.ExecutionResult{}, errors.New("invalid native execution result")
	}
	if event.AfterState != "" {
		snapshot, err := readRegular(filepath.Join(s.w.root, "states", string(event.AfterState)+".json"), maxManifest)
		if err != nil || digestBytes(snapshot) != event.AfterState {
			return s.knownExit(), errors.New("saved after-state unavailable")
		}
	}
	select {
	case <-ctx.Done():
		return ports.ExecutionResult{}, ctx.Err()
	case <-s.done:
	}
	code := 0
	if s.waitErr != nil {
		var exit *exec.ExitError
		if !errors.As(s.waitErr, &exit) || exit.ExitCode() < 0 {
			return ports.ExecutionResult{}, errors.New("native exit status unknown")
		}
		code = exit.ExitCode()
	}
	if code != event.ExitCode {
		return ports.ExecutionResult{}, errors.New("native exit status mismatch")
	}
	return ports.ExecutionResult{ExitKnown: true, ExitCode: code, AfterState: event.AfterState, MatchesPlan: event.MatchesPlan}, nil
}
func (s *nativeSession) exchange(ctx context.Context, operation string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(struct {
		Schema    int           `json:"schema"`
		Operation string        `json:"operation"`
		Plan      domain.Digest `json:"plan"`
	}{1, operation, s.prepared.Assessment.Binding.Plan})
	name := filepath.Join(s.w.root, "control", "command-"+strconv.Itoa(s.sequence)+".json")
	if err := writeNew(name+".pending", raw, 0600); err != nil {
		return nil, err
	}
	if err := os.Rename(name+".pending", name); err != nil {
		return nil, err
	}
	directory, err := os.Open(filepath.Dir(name))
	if err != nil {
		return nil, err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil || closeErr != nil {
		return nil, errors.New("command durability unknown")
	}
	result, err := s.event(ctx, s.sequence)
	s.sequence++
	return result, err
}
func (s *nativeSession) event(ctx context.Context, sequence int) ([]byte, error) {
	wait, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	file := filepath.Join(s.w.root, "control", "event-"+strconv.Itoa(sequence)+".json")
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if raw, err := readRegular(file, maxManifest); err == nil {
			return raw, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Lstat(file); !errors.Is(statErr, os.ErrNotExist) {
				return nil, errors.New("invalid native event file")
			}
		}
		select {
		case <-wait.Done():
			return nil, wait.Err()
		case <-s.done:
			raw, err := readRegular(file, maxManifest)
			if err != nil {
				return nil, errors.New("native session ended without result")
			}
			return raw, nil
		case <-ticker.C:
		}
	}
}
func (s *nativeSession) Close() error {
	if s.cancel != nil {
		defer s.cancel()
	}
	if s.command == nil || s.command.Process == nil {
		return nil
	}
	select {
	case <-s.done:
		return nil
	default:
	}
	if err := syscall.Kill(-s.command.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return errors.New("cannot terminate native session")
	}
	select {
	case <-s.done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("native session termination unknown")
	}
}

// Exit status remains useful even when a crash prevents a final state event.
// A missing after-state still makes the attempt unknown in the application.
func (s *nativeSession) knownExit() ports.ExecutionResult {
	select {
	case <-s.done:
	default:
		return ports.ExecutionResult{}
	}
	if s.command == nil || s.command.ProcessState == nil {
		return ports.ExecutionResult{}
	}
	code := s.command.ProcessState.ExitCode()
	if status, ok := s.command.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		code = 128 + int(status.Signal())
	}
	if code < 0 || code > 255 {
		return ports.ExecutionResult{}
	}
	return ports.ExecutionResult{ExitKnown: true, ExitCode: code}
}
