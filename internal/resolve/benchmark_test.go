package resolve_test

import (
	"fmt"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func BenchmarkResolveExactApplicationFunctions(b *testing.B) {
	const (
		ruleCount        = 100
		functionsPerRule = 20
	)

	var buildEnvironment model.BuildEnvironment

	code := &model.CodeModel{
		GoVersion: "", ModuleRoot: "", WorkspaceFile: "", Modules: nil, Packages: nil,
		Symbols: make([]model.Symbol, 0, ruleCount*functionsPerRule), Types: nil, Implements: nil,
		InterfaceMethods: nil, CallGraph: nil, CallEdges: nil, BuildTags: nil, GOOS: "", GOARCH: "",
		EffectiveBuild: buildEnvironment,
	}

	policy := &model.Policy{
		APIVersion: model.APIVersionV1Alpha1,
		Kind:       model.KindInstrumentationPlan,
		Project:    model.ProjectConfig{Packages: nil, IncludeTests: false, IncludeDependencies: false, BuildTags: nil},
		Backend:    model.BackendConfig{Name: model.BackendNameOTelC, Version: "v1.1.0"},
		Defaults: model.Defaults{
			SpanName:   "",
			Attributes: model.CaptureDefaults{Arguments: false, Results: false},
			Context:    model.ContextDefaults{Mode: model.ContextModeRequire},
			Errors:     model.ErrorDefaults{Record: true},
		},
		Rules:      make([]model.Rule, 0, ruleCount),
		Exclusions: nil,
	}

	for ruleIndex := range ruleCount {
		var selector model.Match

		rule := model.Rule{
			ID: fmt.Sprintf("rule-%03d", ruleIndex), Description: "", Match: selector,
			Exclude: nil, Span: nil, Errors: nil, Attributes: nil,
		}

		rule.Match.Symbols = make([]string, 0, functionsPerRule)

		for functionIndex := range functionsPerRule {
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
				Results:           []model.Result{{Name: "", Type: "error"}},
				ContextIndexes:    []int{0},
				ErrorIndexes:      []int{0},
				Receiver:          nil,
				Location:          model.SourceLocation{File: "", Line: 0, Column: 0},
				Generics:          nil, Generated: false, TestFile: false, HasBody: false, Variadic: false,
			})
		}

		policy.Rules = append(policy.Rules, rule)
	}

	warm := resolve.Resolve(policy, code)
	if warm.Diagnostics.HasErrors() {
		b.Fatalf("synthetic policy resolution returned errors: %+v", warm.Diagnostics)
	}

	if len(warm.Plan.Targets) != ruleCount*functionsPerRule {
		b.Fatalf("resolved %d targets, want %d", len(warm.Plan.Targets), ruleCount*functionsPerRule)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		resolve.Resolve(policy, code)
	}
}
