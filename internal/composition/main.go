package composition

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/attestation"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/githubrelease"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/homebrew"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/localstate"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/osv"
	"github.com/pHo9UBenaA/brew-warden/internal/application"
	"github.com/pHo9UBenaA/brew-warden/internal/cli"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
	"github.com/pHo9UBenaA/brew-warden/internal/ports"
)

// RuntimeSHA256 is set only by the distribution build after runtime validation.
// Development builds without a trusted bundle remain diagnostic-only.
var RuntimeSHA256 string

type clock struct{}

func (clock) Now() int64 { return time.Now().Unix() }

func Main() {
	configPath := ""
	if base, err := os.UserConfigDir(); err == nil {
		configPath = filepath.Join(base, "brewwarden", "config.json")
	}
	statePath := ""
	if configPath != "" {
		statePath = filepath.Join(filepath.Dir(configPath), "history")
	}
	files := localstate.Files{ConfigPath: configPath, StatePath: statePath}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.RunWithRuntime(ctx, os.Args[1:], os.Stdout, os.Stderr, files, files, service(configPath))
	stop()
	os.Exit(code)
}
func service(configPath string) *application.Service {
	if !domain.Digest(RuntimeSHA256).Valid() || configPath == "" {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil
	}
	collector := &homebrew.Collector{
		Runtime:     homebrew.Runtime{Root: filepath.Join(filepath.Dir(executable), "runtime"), ManifestSHA256: domain.Digest(RuntimeSHA256)},
		Directory:   filepath.Join(filepath.Dir(configPath), "collections"),
		Publication: githubrelease.New(), Vulnerabilities: osv.New(),
		Verifier: func(path string, sha domain.Digest) ports.ProvenanceVerifier {
			return attestation.Verifier{Path: path, SHA256: sha}
		},
	}
	engine := homebrew.Engine{Collector: collector, Streams: homebrew.Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}}
	return &application.Service{Planner: engine, Recovery: engine, Diagnostics: engine, Journal: localstate.Journal{Path: filepath.Join(filepath.Dir(configPath), "attempts")}, Clock: clock{}}
}
