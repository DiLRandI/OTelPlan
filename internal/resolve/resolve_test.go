package resolve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func resolverInputs() (*model.Policy, *model.CodeModel) {
	p := &model.Policy{APIVersion: model.APIVersionV1Alpha1, Kind: model.KindInstrumentationPlan, Backend: model.BackendConfig{Name: "otelc", Version: "v0.1.0"}, Defaults: model.Defaults{Errors: model.ErrorDefaults{Record: true}}, Rules: []model.Rule{{ID: "business", Match: model.Match{Functions: []string{"Run"}}}}}
	code := &model.CodeModel{Symbols: []model.Symbol{{ID: "example.com/app.Run", Name: "Run", PackageImportPath: "example.com/app", PackageName: "app", Kind: model.SymbolFunction, Signature: "func(context.Context) error", ContextIndexes: []int{0}, ErrorIndexes: []int{0}, Ownership: model.OwnershipApplication}}}
	return p, code
}

func TestResolveConflictExclusionAndProvenance(t *testing.T) {
	p, code := resolverInputs()
	p.Rules = append(p.Rules, model.Rule{ID: "another", Match: p.Rules[0].Match})
	result := Resolve(p, code)
	if result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 1 {
		t.Fatalf("identical overlap: %+v", result)
	}
	if result.Plan.Targets[0].RuleID != "another" || len(result.Explanations[0].Decisions) != 2 {
		t.Fatalf("missing deterministic provenance: %+v", result)
	}
	p.Rules[1].Span = &model.SpanConfig{Name: "different"}
	result = Resolve(p, code)
	if !result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 0 {
		t.Fatalf("conflict selected target: %+v", result)
	}
	p.Exclusions = []model.Exclusion{{ID: "exclude", Match: p.Rules[0].Match}}
	result = Resolve(p, code)
	if result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 0 || len(result.Plan.Skipped) != 1 {
		t.Fatalf("global exclusion must precede conflict: %+v", result)
	}
	p.Exclusions = nil
	p.Rules[1].Exclude = &p.Rules[1].Match
	result = Resolve(p, code)
	if result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 1 || result.Plan.Targets[0].SpanName != "app.Run" {
		t.Fatalf("local exclusion: %+v", result)
	}
}

func TestResolveContextAndErrors(t *testing.T) {
	p, code := resolverInputs()
	result := Resolve(p, code)
	target := result.Plan.Targets[0]
	if target.ContextStrategy.Strategy != model.ContextStrategyArgument || !reflect.DeepEqual(target.ErrorStrategy.Indexes, []int{0}) {
		t.Fatalf("strategies: %+v", target)
	}
	code.Symbols[0].ContextIndexes = nil
	result = Resolve(p, code)
	if !result.Diagnostics.HasErrors() || result.Diagnostics[0].Code != model.CodeMissingContext || len(result.Plan.Targets) != 0 {
		t.Fatalf("missing context accepted: %+v", result)
	}
	p.Defaults.Context.Mode = model.ContextModeRoot
	p.Rules[0].Errors = &model.ErrorConfig{Record: false}
	result = Resolve(p, code)
	if result.Diagnostics.HasErrors() || result.Plan.Targets[0].ContextStrategy.Strategy != model.ContextStrategyRoot || result.Plan.Targets[0].ErrorStrategy.Record {
		t.Fatalf("explicit root/error opt-out: %+v", result)
	}
	code.Symbols[0].ContextIndexes = []int{0, 1}
	result = Resolve(p, code)
	if !result.Diagnostics.HasErrors() || result.Diagnostics[0].Code != model.CodeMultipleContexts {
		t.Fatalf("ambiguous context accepted: %+v", result)
	}
}

func TestResolveMissingAndMalformedSelectors(t *testing.T) {
	for _, match := range []model.Match{{Symbols: []string{"example.com/app.Missing", "example.com/app.Run"}}, {Functions: []string{"Missing"}}, {Functions: []string{"["}}} {
		p, code := resolverInputs()
		p.Rules[0].Match = match
		result := Resolve(p, code)
		if !result.Diagnostics.HasErrors() {
			t.Fatalf("invalid selector accepted: %+v", match)
		}
	}
}

func TestResolveDoesNotMutateInputAndIgnoresRuleOrder(t *testing.T) {
	p, code := resolverInputs()
	p.Rules = append(p.Rules, model.Rule{ID: "a", Match: p.Rules[0].Match})
	before, _ := json.Marshal(p)
	first := Resolve(p, code)
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("policy mutated")
	}
	p.Rules[0], p.Rules[1] = p.Rules[1], p.Rules[0]
	second := Resolve(p, code)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("rule order changed resolution")
	}
}

func TestResolveArchitectureFixtures(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		files                  map[string]string
		selector, symbol, span string
	}{
		{"functional", map[string]string{"work.go": "package app\nimport \"context\"\nfunc Calculate(ctx context.Context) error { return nil }\n"}, "functions: [Calculate]", "example.com/app.Calculate", "app.Calculate"},
		{"feature", map[string]string{"checkout/work.go": "package checkout\nimport \"context\"\ntype Flow struct{}\nfunc (*Flow) Submit(ctx context.Context) error { return nil }\n"}, "packages: [example.com/app/checkout]", "example.com/app/checkout.(*Flow).Submit", "checkout.Flow.Submit"},
		{"hexagonal", map[string]string{"ports/port.go": "package ports\nimport \"context\"\ntype Boundary interface { Execute(context.Context) error }\n", "adapter/work.go": "package adapter\nimport \"context\"\ntype Local struct{}\nfunc (Local) Execute(ctx context.Context) error { return nil }\nfunc (Local) Other(ctx context.Context) error { return nil }\n"}, "implements: [example.com/app/ports.Boundary]", "example.com/app/adapter.(Local).Execute", "adapter.Local.Execute"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.files["go.mod"] = "module example.com/app\n\ngo 1.27\n"
			for name, contents := range tc.files {
				filename := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(contents), 0644); err != nil {
					t.Fatal(err)
				}
			}
			p, err := policy.Parse([]byte("apiVersion: otelplan.io/v1alpha1\nkind: InstrumentationPlan\nbackend: {name: otelc, version: v0.1.0}\nrules:\n- id: business\n  match:\n    " + tc.selector + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			code, err := discovery.Load(discovery.Options{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			result := Resolve(p, code)
			if result.Diagnostics.HasErrors() {
				t.Fatalf("diagnostics: %+v", result.Diagnostics)
			}
			want := model.ResolvedPlan{APIVersion: model.APIVersionV1Alpha1, Targets: []model.ResolvedTarget{{SymbolID: model.SymbolID(tc.symbol), SpanName: tc.span, RuleID: "business", Signature: "func(ctx context.Context) error", ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument}, ErrorStrategy: model.ErrorStrategy{Record: true, Indexes: []int{0}}}}}
			if !reflect.DeepEqual(result.Plan, want) {
				got, _ := json.MarshalIndent(result.Plan, "", "  ")
				t.Fatalf("resolved plan differs: %s", got)
			}
		})
	}
}
