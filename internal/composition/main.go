package composition

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pHo9UBenaA/brew-warden/internal/adapters/attestation"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/homebrew"
	"github.com/pHo9UBenaA/brew-warden/internal/adapters/localstate"
	"github.com/pHo9UBenaA/brew-warden/internal/application"
	"github.com/pHo9UBenaA/brew-warden/internal/cli"
	"github.com/pHo9UBenaA/brew-warden/internal/domain"
)

// DistributionSource is set by the reproducible distribution build. It is not
// an execution permit: the installed Homebrew tree and all plan inputs are
// checked afresh. Development builds remain diagnostic-only.
var DistributionSource string

type clock struct{}

func (clock) Now() int64 { return time.Now().Unix() }

func Main() {
	configPath := ""
	if base, err := os.UserConfigDir(); err == nil {
		configPath = filepath.Join(base, "brewwarden", "config.json")
	}
	files := localstate.Files{ConfigPath: configPath}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.RunWithRuntime(ctx, os.Args[1:], os.Stdout, os.Stderr, files, service(configPath))
	stop()
	os.Exit(code)
}
func service(configPath string) *application.Service {
	if !domain.Digest(DistributionSource).Valid() || configPath == "" {
		return nil
	}
	gh, _ := attestation.InstalledGH() // Missing/incompatible gh holds doctor and collection.
	collector := &homebrew.Collector{
		Runtime:        homebrew.Runtime{},
		Directory:      filepath.Join(filepath.Dir(configPath), "collections"),
		BottleVerifier: gh,
		LegacyState:    filepath.Join(filepath.Dir(configPath), "attempts"),
	}
	engine := homebrew.Engine{Collector: collector, Streams: homebrew.Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}}
	return &application.Service{Planner: engine, Diagnostics: engine, Clock: clock{}}
}
