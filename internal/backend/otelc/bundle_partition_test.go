package otelc_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
	"gopkg.in/yaml.v3"
)

const partitionFixtureTargets = 65

func partitionBundleFixture() (*model.CodeModel, model.ResolvedPlan) {
	code := new(model.CodeModel)

	var plan model.ResolvedPlan

	for index := range partitionFixtureTargets {
		symbol := new(model.Symbol)
		symbol.Name = fmt.Sprintf("Operation%03d", index)
		symbol.PackageImportPath, symbol.PackageName = "example.com/app", "app"
		symbol.ID = model.FunctionID(symbol.PackageImportPath, symbol.Name)
		symbol.Kind, symbol.Ownership, symbol.HasBody = model.SymbolFunction, model.OwnershipApplication, true
		symbol.Signature = "func(context.Context) error"
		symbol.ContextIndexes, symbol.ErrorIndexes = []int{0}, []int{0}
		parameter := new(model.Parameter)
		parameter.Name, parameter.Type = "ctx", "context.Context"
		symbol.Parameters = []model.Parameter{*parameter}
		result := new(model.Result)
		result.Type = "error"
		symbol.Results = []model.Result{*result}
		code.Symbols = append(code.Symbols, *symbol)
		target := new(model.ResolvedTarget)
		target.SymbolID, target.Signature, target.SpanName = symbol.ID, symbol.Signature, string(symbol.ID)
		target.ContextStrategy.Strategy = model.ContextStrategyArgument
		target.ErrorStrategy.Record, target.ErrorStrategy.Indexes = true, []int{0}
		plan.Targets = append(plan.Targets, *target)
	}

	return code, plan
}

func renderPartitionBundle(t *testing.T, code *model.CodeModel, plan model.ResolvedPlan) []otelc.GeneratedFile {
	t.Helper()

	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	files, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	return files
}

func TestBundleBoundsHookPackages(t *testing.T) {
	t.Parallel()

	code, plan := partitionBundleFixture()
	files := renderPartitionBundle(t, code, plan)

	packages := partitionHookFunctions(t, files)
	if len(packages) != 2 {
		t.Fatalf("hook packages=%d; want 2", len(packages))
	}

	for name, functions := range packages {
		if len(functions) > 128 {
			t.Fatalf("hook package %s has %d functions; want at most 128", name, len(functions))
		}
	}

	assertPartitionRulePaths(t, files, packages)

	slices.Reverse(plan.Targets)

	original := slices.Clone(plan.Targets)

	reordered := renderPartitionBundle(t, code, plan)
	if len(files) != len(reordered) {
		t.Fatal("target ordering changed bundle inventory")
	}

	for index, file := range files {
		if file.Path != reordered[index].Path || !bytes.Equal(file.Data, reordered[index].Data) {
			t.Fatalf("target ordering changed artifact %s", file.Path)
		}
	}

	if !slices.EqualFunc(plan.Targets, original, func(a, b model.ResolvedTarget) bool {
		return a.SymbolID == b.SymbolID
	}) {
		t.Fatal("bundle rendering mutated caller target order")
	}
}

func partitionHookFunctions(t *testing.T, files []otelc.GeneratedFile) map[string]map[string]bool {
	t.Helper()

	packages := map[string]map[string]bool{}

	for _, file := range files {
		if !strings.HasPrefix(file.Path, "hooks/") || !strings.HasSuffix(file.Path, ".go") {
			continue
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), file.Path, file.Data, parser.AllErrors)
		if err != nil {
			t.Fatal(err)
		}

		functions := map[string]bool{}

		for _, declaration := range parsed.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok {
				functions[function.Name.Name] = true
			}
		}

		packages["example.com/generated/"+path.Dir(file.Path)] = functions
	}

	return packages
}

type partitionRule struct {
	Actions []partitionAction `yaml:"do"`
}

type partitionAction struct {
	Hooks partitionAdvice `yaml:"inject_hooks"`
}

type partitionAdvice struct {
	Before string `yaml:"before"`
	After  string `yaml:"after"`
	Path   string `yaml:"path"`
}

func assertPartitionRulePaths(t *testing.T, files []otelc.GeneratedFile, packages map[string]map[string]bool) {
	t.Helper()

	seen := map[string]bool{}

	for _, file := range files {
		if !strings.HasPrefix(file.Path, "rules/hooks") {
			continue
		}

		var rules map[string]partitionRule

		err := yaml.Unmarshal(file.Data, &rules)
		if err != nil {
			t.Fatal(err)
		}

		for ruleID, rule := range rules {
			if seen[ruleID] || len(rule.Actions) != 1 {
				t.Fatalf("invalid or duplicate partition rule %s", ruleID)
			}

			seen[ruleID] = true
			advice := rule.Actions[0].Hooks

			functions := packages[advice.Path]
			if !functions[advice.Before] || !functions[advice.After] {
				t.Fatalf("rule %s does not point to its generated hooks", ruleID)
			}
		}
	}

	if len(seen) != partitionFixtureTargets {
		t.Fatalf("rendered rules=%d; want %d", len(seen), partitionFixtureTargets)
	}
}

func TestPartitionBundleRejectsCrossPackageDuplicates(t *testing.T) {
	t.Parallel()

	code, plan := partitionBundleFixture()
	plan.Targets[len(plan.Targets)-1] = plan.Targets[0]

	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	files, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err == nil || files != nil {
		t.Fatal("duplicate target produced a partial partitioned bundle")
	}
}
