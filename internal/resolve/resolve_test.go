package resolve_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func functionMatch(names []string) model.Match {
	var match model.Match

	match.Functions = names

	return match
}

func resolverInputs() (*model.Policy, *model.CodeModel) {
	var rule model.Rule

	rule.ID = "business"
	rule.Match = functionMatch([]string{"Run"})

	var policyInput model.Policy

	policyInput.APIVersion = model.APIVersionV1Alpha1
	policyInput.Kind = model.KindInstrumentationPlan
	policyInput.Backend = model.BackendConfig{Name: "otelc", Version: "v0.1.0"}
	policyInput.Defaults.Errors.Record = true
	policyInput.Rules = []model.Rule{rule}

	var symbol model.Symbol

	symbol.ID = "example.com/app.Run"
	symbol.Name = "Run"
	symbol.PackageImportPath = "example.com/app"
	symbol.PackageName = "app"
	symbol.Kind = model.SymbolFunction
	symbol.Signature = "func(context.Context) error"
	symbol.ContextIndexes = []int{0}
	symbol.ErrorIndexes = []int{0}
	symbol.Ownership = model.OwnershipApplication

	var code model.CodeModel

	code.Symbols = []model.Symbol{symbol}

	return &policyInput, &code
}

func TestResolveIdenticalOverlapKeepsDeterministicProvenance(t *testing.T) {
	t.Parallel()

	policyInput, code := resolverInputs()

	var rule model.Rule

	rule.ID = "another"
	rule.Match = policyInput.Rules[0].Match
	policyInput.Rules = append(policyInput.Rules, rule)

	result := resolve.Resolve(policyInput, code)
	if result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 1 {
		t.Fatalf("identical overlap: %+v", result)
	}

	if result.Plan.Targets[0].RuleID != "another" || len(result.Explanations[0].Decisions) != 2 {
		t.Fatalf("missing deterministic provenance: %+v", result)
	}
}

func TestResolveConflictingOverlapIsRejected(t *testing.T) {
	t.Parallel()

	policyInput, code := resolverInputs()

	var rule model.Rule

	rule.ID = "another"
	rule.Match = policyInput.Rules[0].Match

	var span model.SpanConfig

	span.Name = "different"
	rule.Span = &span
	policyInput.Rules = append(policyInput.Rules, rule)

	result := resolve.Resolve(policyInput, code)
	if !result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 0 {
		t.Fatalf("conflict selected target: %+v", result)
	}
}

func TestResolveGlobalExclusionPrecedesConflict(t *testing.T) {
	t.Parallel()

	policyInput, code := resolverInputs()

	var rule model.Rule

	rule.ID = "another"
	rule.Match = policyInput.Rules[0].Match

	var span model.SpanConfig

	span.Name = "different"
	rule.Span = &span
	policyInput.Rules = append(policyInput.Rules, rule)
	policyInput.Exclusions = []model.Exclusion{{ID: "exclude", Match: policyInput.Rules[0].Match}}

	result := resolve.Resolve(policyInput, code)
	if result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 0 || len(result.Plan.Skipped) != 1 {
		t.Fatalf("global exclusion must precede conflict: %+v", result)
	}
}

func TestResolveLocalExclusionKeepsOtherRule(t *testing.T) {
	t.Parallel()

	policyInput, code := resolverInputs()

	var rule model.Rule

	rule.ID = "another"
	rule.Match = policyInput.Rules[0].Match
	rule.Span = &model.SpanConfig{Name: "different", Kind: ""}
	rule.Exclude = &policyInput.Rules[0].Match
	policyInput.Rules = append(policyInput.Rules, rule)

	result := resolve.Resolve(policyInput, code)
	if result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 1 || result.Plan.Targets[0].SpanName != "app.Run" {
		t.Fatalf("local exclusion: %+v", result)
	}
}

func TestResolveArgumentContextRecordsErrors(t *testing.T) {
	t.Parallel()

	policyInput, code := resolverInputs()
	result := resolve.Resolve(policyInput, code)

	target := result.Plan.Targets[0]
	if target.ContextStrategy.Strategy != model.ContextStrategyArgument ||
		!reflect.DeepEqual(target.ErrorStrategy.Indexes, []int{0}) {
		t.Fatalf("strategies: %+v", target)
	}
}

func TestResolveMissingContextIsRejected(t *testing.T) {
	t.Parallel()

	policyInput, code := resolverInputs()
	code.Symbols[0].ContextIndexes = nil

	result := resolve.Resolve(policyInput, code)
	if !result.Diagnostics.HasErrors() || result.Diagnostics[0].Code != model.CodeMissingContext ||
		len(result.Plan.Targets) != 0 {
		t.Fatalf("missing context accepted: %+v", result)
	}
}

func TestResolveRootContextAllowsErrorOptOut(t *testing.T) {
	t.Parallel()

	policyInput, code := resolverInputs()
	code.Symbols[0].ContextIndexes = nil
	policyInput.Defaults.Context.Mode = model.ContextModeRoot
	policyInput.Rules[0].Errors = &model.ErrorConfig{Record: false}

	result := resolve.Resolve(policyInput, code)
	if result.Diagnostics.HasErrors() || result.Plan.Targets[0].ContextStrategy.Strategy != model.ContextStrategyRoot ||
		result.Plan.Targets[0].ErrorStrategy.Record {
		t.Fatalf("explicit root/error opt-out: %+v", result)
	}
}

func TestResolveMultipleContextsAreRejected(t *testing.T) {
	t.Parallel()

	policyInput, code := resolverInputs()
	code.Symbols[0].ContextIndexes = []int{0, 1}
	policyInput.Defaults.Context.Mode = model.ContextModeRoot
	policyInput.Rules[0].Errors = &model.ErrorConfig{Record: false}

	result := resolve.Resolve(policyInput, code)
	if !result.Diagnostics.HasErrors() || result.Diagnostics[0].Code != model.CodeMultipleContexts {
		t.Fatalf("ambiguous context accepted: %+v", result)
	}
}

func TestResolveMissingAndMalformedSelectors(t *testing.T) {
	t.Parallel()

	selectorCases := []struct {
		name  string
		match model.Match
	}{
		{name: "missing symbol", match: matchWithSymbols([]string{"example.com/app.Missing", "example.com/app.Run"})},
		{name: "missing function", match: functionMatch([]string{"Missing"})},
		{name: "malformed function", match: functionMatch([]string{"["})},
	}

	for _, selectorCase := range selectorCases {
		t.Run(selectorCase.name, func(t *testing.T) {
			t.Parallel()

			policyInput, code := resolverInputs()
			policyInput.Rules[0].Match = selectorCase.match

			result := resolve.Resolve(policyInput, code)
			if !result.Diagnostics.HasErrors() {
				t.Fatalf("invalid selector accepted: %+v", selectorCase.match)
			}
		})
	}
}

func matchWithSymbols(symbols []string) model.Match {
	var match model.Match

	match.Symbols = symbols

	return match
}

func TestResolveDoesNotMutateInputAndIgnoresRuleOrder(t *testing.T) {
	t.Parallel()

	policyInput, code := resolverInputs()

	var rule model.Rule

	rule.ID = "a"
	rule.Match = policyInput.Rules[0].Match
	policyInput.Rules = append(policyInput.Rules, rule)

	before, err := json.Marshal(policyInput)
	if err != nil {
		t.Fatalf("encode policy before resolution: %v", err)
	}

	first := resolve.Resolve(policyInput, code)

	after, err := json.Marshal(policyInput)
	if err != nil {
		t.Fatalf("encode policy after resolution: %v", err)
	}

	if string(before) != string(after) {
		t.Fatal("policy mutated")
	}

	policyInput.Rules[0], policyInput.Rules[1] = policyInput.Rules[1], policyInput.Rules[0]

	second := resolve.Resolve(policyInput, code)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("rule order changed resolution")
	}
}

func TestResolveArchitectureFixtures(t *testing.T) {
	t.Parallel()

	fixtureCases := []struct {
		name     string
		files    map[string]string
		selector string
		symbol   string
		span     string
	}{
		{
			name: "functional", files: map[string]string{
				"work.go": `package app
import "context"
func Calculate(ctx context.Context) error { return nil }
`,
			}, selector: "functions: [Calculate]", symbol: "example.com/app.Calculate", span: "app.Calculate",
		},
		{
			name: "feature", files: map[string]string{
				"checkout/work.go": `package checkout
import "context"
type Flow struct{}
func (*Flow) Submit(ctx context.Context) error { return nil }
`,
			},
			selector: "packages: [example.com/app/checkout]",
			symbol:   "example.com/app/checkout.(*Flow).Submit", span: "checkout.Flow.Submit",
		},
		{
			name: "hexagonal", files: map[string]string{
				"ports/port.go": `package ports
import "context"
type Boundary interface { Execute(context.Context) error }
`,
				"adapter/work.go": `package adapter
import "context"
type Local struct{}
func (Local) Execute(ctx context.Context) error { return nil }
func (Local) Other(ctx context.Context) error { return nil }
`,
			},
			selector: "implements: [example.com/app/ports.Boundary]",
			symbol:   "example.com/app/adapter.(Local).Execute", span: "adapter.Local.Execute",
		},
	}

	for _, fixtureCase := range fixtureCases {
		t.Run(fixtureCase.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			fixtureCase.files["go.mod"] = "module example.com/app\n\ngo 1.27\n"

			for name, contents := range fixtureCase.files {
				filename := filepath.Join(root, name)

				err := os.MkdirAll(filepath.Dir(filename), 0o700)
				if err != nil {
					t.Fatal(err)
				}

				err = os.WriteFile(filename, []byte(contents), 0o600)
				if err != nil {
					t.Fatal(err)
				}
			}

			policyBytes := []byte("apiVersion: otelplan.io/v1alpha1\n" +
				"kind: InstrumentationPlan\n" +
				"backend: {name: otelc, version: v0.1.0}\n" +
				"rules:\n- id: business\n  match:\n    " + fixtureCase.selector + "\n")

			policyInput, err := policy.Parse(policyBytes)
			if err != nil {
				t.Fatal(err)
			}

			code, err := discovery.Load(discovery.Options{Root: root, Patterns: nil, BuildTags: nil, BuildFlags: nil,
				CallGraph: false, IncludeTests: false, IncludeDependencies: false, GOOS: "", GOARCH: "", Env: nil, Offline: false})
			if err != nil {
				t.Fatal(err)
			}

			result := resolve.Resolve(policyInput, code)
			if result.Diagnostics.HasErrors() {
				t.Fatalf("diagnostics: %+v", result.Diagnostics)
			}

			var target model.ResolvedTarget

			target.SymbolID = model.SymbolID(fixtureCase.symbol)
			target.SpanName = fixtureCase.span
			target.RuleID = "business"
			target.Signature = "func(ctx context.Context) error"
			target.ContextStrategy.Strategy = model.ContextStrategyArgument
			target.ErrorStrategy.Record = true
			target.ErrorStrategy.Indexes = []int{0}

			var want model.ResolvedPlan

			want.APIVersion = model.APIVersionV1Alpha1

			want.Targets = []model.ResolvedTarget{target}
			if !reflect.DeepEqual(result.Plan, want) {
				t.Fatalf("resolved plan = %+v; want %+v", result.Plan, want)
			}
		})
	}
}
