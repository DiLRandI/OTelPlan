package lockfile_test

import (
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestCreateRejectsInvalidTargetsWithoutPartialLock(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		change  func(*model.CodeModel, *model.ResolvedPlan)
		message string
	}{
		{
			name: "duplicate", message: "duplicate lock target",
			change: func(_ *model.CodeModel, plan *model.ResolvedPlan) {
				plan.Targets = append(plan.Targets, plan.Targets[0])
			},
		},
		{
			name: "missing symbol", message: "target signature does not match analyzed symbol",
			change: func(_ *model.CodeModel, plan *model.ResolvedPlan) { plan.Targets[0].SymbolID = "missing" },
		},
		{
			name: "stale signature", message: "target signature does not match analyzed symbol",
			change: func(_ *model.CodeModel, plan *model.ResolvedPlan) { plan.Targets[0].Signature = "func(int)" },
		},
		{
			name: "source path", message: "source path must be module-relative",
			change: func(code *model.CodeModel, _ *model.ResolvedPlan) { code.Symbols[0].Location.File = "../app.go" },
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			policy, code, plan := creationInputs()
			testCase.change(code, &plan)

			var backend model.LockBackend

			backend.Name = policy.Backend.Name
			backend.Version = policy.Backend.Version

			lock, err := lockfile.Create(policy, code, plan, backend, lockfile.Digest(nil), nil)
			if err == nil || err.Error() != testCase.message {
				t.Fatalf("Create error = %v; want %q", err, testCase.message)
			}

			var empty model.Lockfile

			if !reflect.DeepEqual(lock, empty) {
				t.Fatal("failed creation returned a partial lockfile")
			}
		})
	}
}

func creationInputs() (*model.Policy, *model.CodeModel, model.ResolvedPlan) {
	var policy model.Policy

	policy.Backend = model.BackendConfig{Name: "otelc", Version: "v1.1.0"}

	var symbol model.Symbol

	symbol.ID = "example.com/app.Run"
	symbol.Signature = "func()"
	symbol.Location.File = "app.go"

	var code model.CodeModel

	code.GoVersion = "go1.27.0"
	code.Symbols = []model.Symbol{symbol}

	var target model.ResolvedTarget

	target.SymbolID = symbol.ID
	target.Signature = symbol.Signature
	target.SpanName = "app.Run"
	target.RuleID = "run"
	target.ContextStrategy.Strategy = model.ContextStrategyRoot

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{target}

	return &policy, &code, plan
}
