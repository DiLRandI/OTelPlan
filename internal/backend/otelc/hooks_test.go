package otelc

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const hookImportPath = "example.com/app/hooks"

func TestRenderHooksDeterministicAndDoesNotMutatePlan(t *testing.T) {
	code, plan := hookFixture()
	original := append([]model.ResolvedTarget(nil), plan.Targets...)

	want, err := RenderHooks(SupportedVersion, "runtime-1.2.3", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
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

func TestRenderHooksUsesMethodContextOffsetAndStableResultIndexes(t *testing.T) {
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
	if !strings.Contains(string(source), "WithInstrumentationVersion") || !strings.Contains(string(source), "otelplan.io/business") {
		t.Fatalf("instrumentation scope/version missing: %s", source)
	}
}

func TestRenderHooksEmptyPlanHasNoUnusedImports(t *testing.T) {
	code, _ := ruleFixture()
	source, err := RenderHooks(SupportedVersion, "runtime", code, model.ResolvedPlan{}, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}
	file := parseGeneratedHooks(t, source)
	if len(file.Imports) != 0 {
		t.Fatalf("empty plan generated imports: %s", source)
	}
}

func TestRenderHooksRejectsUnsupportedPlansWithoutPartialOutput(t *testing.T) {
	tests := []struct {
		name                string
		change              func(*model.CodeModel, *model.ResolvedPlan)
		blankRuntimeVersion bool
		wantError           string
	}{
		{name: "attributes", change: func(_ *model.CodeModel, p *model.ResolvedPlan) {
			p.Targets[0].Attributes = []model.AttributePlan{{Key: "request.id", From: model.AttributeSource{Argument: "request.ID"}}}
		}},
		{name: "variadic", change: func(c *model.CodeModel, _ *model.ResolvedPlan) { c.Symbols[0].Variadic = true }},
		{name: "duplicate error indexes", wantError: "unique", change: func(c *model.CodeModel, p *model.ResolvedPlan) {
			c.Symbols[0].Results = []model.Result{{Type: "error"}}
			c.Symbols[0].ErrorIndexes = []int{0}
			c.Symbols[0].Signature = "func() error"
			p.Targets[0].Signature = c.Symbols[0].Signature
			p.Targets[0].ErrorStrategy = model.ErrorStrategy{Record: true, Indexes: []int{0, 0}}
		}},
		{name: "blank runtime version", blankRuntimeVersion: true},
		{name: "blank span name", change: func(_ *model.CodeModel, p *model.ResolvedPlan) { p.Targets[0].SpanName = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, plan := hookFixture()
			if tc.change != nil {
				tc.change(code, &plan)
			}
			runtime := "runtime"
			if tc.blankRuntimeVersion {
				runtime = ""
			}
			data, err := RenderHooks(SupportedVersion, runtime, code, plan, hookImportPath)
			if err == nil || data != nil {
				t.Fatalf("unsupported plan produced partial output: %q, %v", data, err)
			}
			if tc.wantError != "" && !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error %q does not contain %q", err, tc.wantError)
			}
		})
	}
}

func TestRenderHooksPropagatesRenderRulesValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*model.CodeModel)
	}{
		{name: "invalid", change: func(c *model.CodeModel) { c.Symbols[0].Name = "bad-name" }},
		{name: "mismatched", change: func(c *model.CodeModel) { c.Symbols[0].Name = "Other" }},
		{name: "generic", change: func(c *model.CodeModel) { c.Symbols[0].Generics = &model.GenericInfo{TypeParams: []string{"T"}} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, plan := ruleFixture()
			tc.change(code)
			if data, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath); err == nil || data != nil {
				t.Fatalf("invalid RenderRules target produced output: %q, %v", data, err)
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
	file, err := parser.ParseFile(token.NewFileSet(), "hooks.go", source, parser.AllErrors)
	if err != nil {
		t.Fatalf("generated hooks do not parse: %v\n%s", err, source)
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
	for _, literal := range literals {
		if literal == want {
			return true
		}
	}
	return false
}
