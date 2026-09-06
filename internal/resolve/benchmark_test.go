package resolve

import (
	"fmt"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func BenchmarkResolveExactApplicationFunctions(b *testing.B) {
	const (
		ruleCount        = 100
		functionsPerRule = 20
	)

	code := &model.CodeModel{Symbols: make([]model.Symbol, 0, ruleCount*functionsPerRule)}
	policy := &model.Policy{
		APIVersion: model.APIVersionV1Alpha1,
		Kind:       model.KindInstrumentationPlan,
		Backend:    model.BackendConfig{Name: model.BackendNameOTelC, Version: "v1.1.0"},
		Defaults: model.Defaults{
			Context: model.ContextDefaults{Mode: model.ContextModeRequire},
			Errors:  model.ErrorDefaults{Record: true},
		},
		Rules: make([]model.Rule, 0, ruleCount),
	}
	for ruleIndex := 0; ruleIndex < ruleCount; ruleIndex++ {
		rule := model.Rule{ID: fmt.Sprintf("rule-%03d", ruleIndex), Match: model.Match{}}
		rule.Match.Symbols = make([]string, 0, functionsPerRule)
		for functionIndex := 0; functionIndex < functionsPerRule; functionIndex++ {
			index := ruleIndex*functionsPerRule + functionIndex
			id := model.FunctionID("example.com/app", fmt.Sprintf("Function%04d", index))
			rule.Match.Symbols = append(rule.Match.Symbols, string(id))
			code.Symbols = append(code.Symbols, model.Symbol{
				ID:                id,
				Kind:              model.SymbolFunction,
				PackageImportPath: "example.com/app",
				PackageName:       "app",
				Name:              fmt.Sprintf("Function%04d", index),
				Visibility:        model.VisibilityExported,
				Ownership:         model.OwnershipApplication,
				Signature:         "func(context.Context) error",
				Parameters:        []model.Parameter{{Name: "ctx", Type: "context.Context"}},
				Results:           []model.Result{{Type: "error"}},
				ContextIndexes:    []int{0},
				ErrorIndexes:      []int{0},
			})
		}
		policy.Rules = append(policy.Rules, rule)
	}

	warm := Resolve(policy, code)
	if warm.Diagnostics.HasErrors() {
		b.Fatalf("synthetic policy resolution returned errors: %+v", warm.Diagnostics)
	}
	if len(warm.Plan.Targets) != ruleCount*functionsPerRule {
		b.Fatalf("resolved %d targets, want %d", len(warm.Plan.Targets), ruleCount*functionsPerRule)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Resolve(policy, code)
	}
}
