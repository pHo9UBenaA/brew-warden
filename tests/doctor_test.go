package tests

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/pHo9UBenaA/brew-warden/internal/application"
	"github.com/pHo9UBenaA/brew-warden/internal/cli"
)

type runtimeDiagnostics struct{ err error }

func (d runtimeDiagnostics) Check(context.Context) error { return d.err }

type unavailableOutput struct{}

func (unavailableOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRuntimeDoctorReportsCapabilityWithoutPreparingMutation(t *testing.T) {
	for _, test := range []struct {
		name              string
		diagnostics       *runtimeDiagnostics
		outputUnavailable bool
		wantCode          int
		wantOutput        string
		wantDiagnostic    string
	}{
		{
			name: "supported runtime", diagnostics: &runtimeDiagnostics{},
			wantOutput: "Minimum release age: 3600 seconds.",
		},
		{
			name: "missing diagnostics", wantCode: 1,
			wantDiagnostic: "runtime_unavailable",
		},
		{
			name: "unsupported runtime", diagnostics: &runtimeDiagnostics{err: errors.New("platform unsupported")},
			wantCode: 1, wantDiagnostic: "runtime_unavailable: platform unsupported",
		},
		{
			name: "output failure", diagnostics: &runtimeDiagnostics{},
			outputUnavailable: true, wantCode: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			planner := &servicePlanner{}
			service := application.Service{Planner: planner, Clock: &executionClock{now: 1000}}
			if test.diagnostics != nil {
				service.Diagnostics = *test.diagnostics
			}
			var output, diagnostics bytes.Buffer
			var writer io.Writer = &output
			if test.outputUnavailable {
				writer = unavailableOutput{}
			}
			code := cli.RunWithRuntime(context.Background(), []string{"--minimum-release-age", "1h", "doctor"}, writer, &diagnostics, nil, &service)
			if code != test.wantCode || planner.calls != 0 {
				t.Fatalf("doctor: want exit=%d without mutation planning, got exit=%d preparations=%d output=%q diagnostics=%q", test.wantCode, code, planner.calls, &output, &diagnostics)
			}
			if !strings.Contains(output.String(), test.wantOutput) || !strings.Contains(diagnostics.String(), test.wantDiagnostic) {
				t.Fatalf("doctor: want output containing %q and diagnostics containing %q, got output=%q diagnostics=%q", test.wantOutput, test.wantDiagnostic, &output, &diagnostics)
			}
			if test.wantOutput == "" && output.Len() != 0 {
				t.Fatalf("doctor must not write stdout in this case: output=%q", &output)
			}
			if test.wantDiagnostic == "" && diagnostics.Len() != 0 {
				t.Fatalf("doctor must not write stderr in this case: diagnostics=%q", &diagnostics)
			}
		})
	}
}
