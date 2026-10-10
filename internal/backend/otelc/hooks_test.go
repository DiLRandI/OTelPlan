package otelc

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	hookImportPath    = "example.com/app/hooks"
	hookFixtureDigest = "abafbd73d5e844e73135f50bb1ed2751c8895724588b03b387e0facc89629577"
)

func TestGenericRootHooksAvoidUnsupportedAPIs(t *testing.T) {
	t.Parallel()

	code, plan := genericRootFixture()
	attribute := new(model.AttributePlan)
	attribute.Key, attribute.From.Constant = "operation.kind", "generic"
	attribute.Classification = model.ClassificationPublic
	plan.Targets[0].Attributes = []model.AttributePlan{*attribute}

	source, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{"GetParam(", "SetParam(", "GetReturnVal(", "SetReturnVal("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("generic hook uses an unsupported API: %s", source)
		}
	}

	if !strings.Contains(string(source), "returned_1.(error)") || !strings.Contains(string(source), "span.RecordError") {
		t.Fatalf("generic hook omitted direct returned-error recording: %s", source)
	}

	parseGeneratedHooks(t, source)
}

func TestRenderHooksDeterministicAndDoesNotMutatePlan(t *testing.T) {
	t.Parallel()

	code, plan := hookFixture()
	original := append([]model.ResolvedTarget(nil), plan.Targets...)

	want, err := RenderHooks(SupportedVersion, "runtime-1.2.3", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	digest := fmt.Sprintf("%x", sha256.Sum256(want))
	if digest != hookFixtureDigest {
		t.Fatalf("generated fixture hash = %s, want %s", digest, hookFixtureDigest)
	}

	if !reflect.DeepEqual(plan.Targets, original) {
		t.Fatal("render mutated caller plan")
	}

	plan.Targets[0], plan.Targets[2] = plan.Targets[2], plan.Targets[0]

	got, err := RenderHooks(SupportedVersion, "runtime-1.2.3", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(want, got) {
		t.Fatal("input ordering changed generated hooks")
	}
}

func TestRenderHooksMatchesRuleBindingsAndSignatureIndexes(t *testing.T) {
	t.Parallel()

	code, plan := hookFixture()

	source, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	file := parseGeneratedHooks(t, source)

	_, bindings, err := RenderRules(SupportedVersion, code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, binding := range bindings {
		symbol, ok := code.Symbol(binding.Symbol)
		if !ok {
			t.Fatalf("missing symbol %q", binding.Symbol)
		}

		before := findFunction(t, file, binding.Before)
		after := findFunction(t, file, binding.After)

		wantBefore := len(symbol.Parameters) + 1
		if symbol.Receiver != nil {
			wantBefore++
		}

		if got := fieldCount(before.Type.Params); got != wantBefore {
			t.Errorf("%s has %d parameters, want %d", binding.Before, got, wantBefore)
		}

		if got := fieldCount(after.Type.Params); got != len(symbol.Results)+1 {
			t.Errorf("%s has %d parameters, want %d", binding.After, got, len(symbol.Results)+1)
		}
	}
}

func TestRenderHooksUsesTypedVariadicParameter(t *testing.T) {
	t.Parallel()

	code, plan := compatibilityFixture()
	code.Symbols[0].Variadic = true
	code.Symbols[0].Kind = model.SymbolFunction
	code.Symbols[0].Name = "Run"
	code.Symbols[0].PackageImportPath = "example.com/app"
	code.Symbols[0].Parameters = append(code.Symbols[0].Parameters,
		model.Parameter{Name: "values", Type: "[]int"})
	code.Symbols[0].Signature = "func(context.Context, ...int) error"
	plan.Targets[0].Signature = code.Symbols[0].Signature
	plan.Targets[0].SpanName = "Run"

	source, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	file := parseGeneratedHooks(t, source)

	_, bindings, err := RenderRules(SupportedVersion, code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	before := findFunction(t, file, bindings[0].Before)
	parameter := before.Type.Params.List[len(before.Type.Params.List)-1]

	ellipsis, ok := parameter.Type.(*ast.Ellipsis)
	if !ok {
		t.Fatalf("variadic hook parameter is not typed: %s", source)
	}

	if name, ok := ellipsis.Elt.(*ast.Ident); !ok || name.Name != "int" {
		t.Fatalf("variadic hook has wrong element type: %s", source)
	}
}

func TestRenderHooksUsesMethodContextOffsetAndStableResultIndexes(t *testing.T) {
	t.Parallel()

	code, plan := hookFixture()
	method := &code.Symbols[1]
	method.Parameters = []model.Parameter{{Name: "ctx", Type: "context.Context"}, {Name: "request", Type: "Request"}}
	method.Results = []model.Result{{Name: "err", Type: "error"}, {Name: "ok", Type: "bool"}}
	method.ContextIndexes = []int{0}
	method.ErrorIndexes = []int{0}
	method.Signature = "func(context.Context, Request) (error, bool)"
	plan.Targets[1].Signature = method.Signature
	plan.Targets[1].ContextStrategy = model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0}
	plan.Targets[1].ErrorStrategy = model.ErrorStrategy{Record: true, Indexes: []int{0}}

	source, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	file := parseGeneratedHooks(t, source)

	_, bindings, err := RenderRules(SupportedVersion, code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	var methodBinding HookBinding

	for _, binding := range bindings {
		if binding.Symbol == method.ID {
			methodBinding = binding

			break
		}
	}

	if methodBinding.Symbol == "" {
		t.Fatal("method binding missing")
	}

	before := findFunction(t, file, methodBinding.Before)

	after := findFunction(t, file, methodBinding.After)

	if got, want := fieldCount(before.Type.Params), 4; got != want {
		t.Fatalf("method before parameters: got %d, want %d", got, want)
	}

	if got, want := fieldCount(after.Type.Params), 3; got != want {
		t.Fatalf("method after parameters: got %d, want %d", got, want)
	}

	if !strings.Contains(string(source), "GetParam(1)") {
		t.Fatalf("method context index was not offset for receiver: %s", source)
	}

	if !strings.Contains(string(source), "GetReturnVal(0)") {
		t.Fatalf("method result error index changed: %s", source)
	}
}

func TestRenderHooksQuotesSpanAndRuntimeVersion(t *testing.T) {
	t.Parallel()

	code, plan := ruleFixture()
	span := "line\n\"quoted\\span"
	runtime := "runtime\n\"version\\suffix"

	for i := range plan.Targets {
		plan.Targets[i].SpanName = span
	}

	source, err := RenderHooks(SupportedVersion, runtime, code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	parseGeneratedHooks(t, source)

	literals := stringLiterals(t, source)
	if !containsLiteral(literals, span) || !containsLiteral(literals, runtime) {
		t.Fatalf("generated source did not preserve quoted values: %v", literals)
	}

	if !strings.Contains(string(source), "WithInstrumentationVersion") ||
		!strings.Contains(string(source), "otelplan.io/business") {
		t.Fatalf("instrumentation scope/version missing: %s", source)
	}
}

func TestRenderHooksEmptyPlanHasNoUnusedImports(t *testing.T) {
	t.Parallel()

	code, _ := ruleFixture()

	plan := new(model.ResolvedPlan)

	source, err := RenderHooks(SupportedVersion, "runtime", code, *plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	file := parseGeneratedHooks(t, source)
	if len(file.Imports) != 0 {
		t.Fatalf("empty plan generated imports: %s", source)
	}
}

func TestRenderHooksRejectsUnsupportedPlansWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		change              func(*model.CodeModel, *model.ResolvedPlan)
		blankRuntimeVersion bool
		wantExactError      string
	}{
		{
			name: "invalid attribute source",
			change: func(_ *model.CodeModel, plan *model.ResolvedPlan) {
				attribute := new(model.AttributePlan)
				attribute.Key = "request.id"
				attribute.From.Argument = "request.ID"
				plan.Targets[0].Attributes = []model.AttributePlan{*attribute}
			},
			blankRuntimeVersion: false,
			wantExactError:      "",
		},
		{
			name:                "variadic",
			change:              func(code *model.CodeModel, _ *model.ResolvedPlan) { code.Symbols[0].Variadic = true },
			blankRuntimeVersion: false,
			wantExactError:      "",
		},
		{
			name: "duplicate error indexes",
			change: func(code *model.CodeModel, plan *model.ResolvedPlan) {
				result := new(model.Result)
				result.Type = "error"
				code.Symbols[0].Results = []model.Result{*result}
				code.Symbols[0].ErrorIndexes = []int{0}
				code.Symbols[0].Signature = "func() error"
				plan.Targets[0].Signature = code.Symbols[0].Signature
				errorStrategy := new(model.ErrorStrategy)
				errorStrategy.Record = true
				errorStrategy.Indexes = []int{0, 0}
				plan.Targets[0].ErrorStrategy = *errorStrategy
			},
			blankRuntimeVersion: false,
			wantExactError:      "hook error result indexes must be unique",
		},
		{
			name:                "blank runtime version",
			change:              nil,
			blankRuntimeVersion: true,
			wantExactError:      "hook generation requires an instrumentation version",
		},
		{
			name:                "blank span name",
			change:              func(_ *model.CodeModel, plan *model.ResolvedPlan) { plan.Targets[0].SpanName = "" },
			blankRuntimeVersion: false,
			wantExactError:      "hook generation requires a span name",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, plan := hookFixture()
			if testCase.change != nil {
				testCase.change(code, &plan)
			}

			runtime := "runtime"
			if testCase.blankRuntimeVersion {
				runtime = ""
			}

			data, firstErr := RenderHooks(SupportedVersion, runtime, code, plan, hookImportPath)
			if firstErr == nil || data != nil {
				t.Fatalf("unsupported plan produced partial output: %q, %v", data, firstErr)
			}

			if testCase.wantExactError == "" {
				return
			}

			if firstErr.Error() != testCase.wantExactError {
				t.Fatalf("error = %q, want %q", firstErr, testCase.wantExactError)
			}

			secondData, secondErr := RenderHooks(SupportedVersion, runtime, code, plan, hookImportPath)
			if secondData != nil || !errors.Is(secondErr, firstErr) {
				t.Fatalf("repeated error %q does not match first error %q", secondErr, firstErr)
			}
		})
	}
}

func TestRenderHooksPropagatesRenderRulesValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*model.CodeModel)
	}{
		{name: "invalid", change: func(code *model.CodeModel) { code.Symbols[0].Name = "bad-name" }},
		{name: "mismatched", change: func(code *model.CodeModel) { code.Symbols[0].Name = "Other" }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, plan := ruleFixture()
			testCase.change(code)

			data, renderErr := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
			if renderErr == nil || data != nil {
				t.Fatalf("invalid RenderRules target produced output: %q, %v", data, renderErr)
			}
		})
	}
}

func hookFixture() (*model.CodeModel, model.ResolvedPlan) {
	code, plan := ruleFixture()
	for i := range plan.Targets {
		plan.Targets[i].SpanName = string(plan.Targets[i].SymbolID)
	}

	return code, plan
}

func parseGeneratedHooks(t *testing.T, source []byte) *ast.File {
	t.Helper()

	file, parseErr := parser.ParseFile(token.NewFileSet(), "hooks.go", source, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated hooks do not parse: %v\n%s", parseErr, source)
	}

	return file
}

func findFunction(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()

	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}

	t.Fatalf("generated function %q missing", name)

	return nil
}

func fieldCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}

	count := 0

	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++

			continue
		}

		count += len(field.Names)
	}

	return count
}

func stringLiterals(t *testing.T, source []byte) []string {
	t.Helper()
	file := parseGeneratedHooks(t, source)

	var literals []string

	ast.Inspect(file, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("invalid generated string literal %q: %v", lit.Value, err)
		}

		literals = append(literals, value)

		return true
	})

	return literals
}

func containsLiteral(literals []string, want string) bool {
	return slices.Contains(literals, want)
}
