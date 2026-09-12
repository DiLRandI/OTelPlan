package otelc

import (
	"go/ast"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestAttributeHooksAreTypedAndBound(t *testing.T) {
	_, code, target := accessorFixture(t)
	target.SpanName = "accessor.operation"
	target.Attributes = []model.AttributePlan{
		{Key: "request.bool", From: model.AttributeSource{Argument: "req.Enabled"}},
		{Key: "request.float", From: model.AttributeSource{Argument: "req.Inner.Score"}},
		{Key: "request.int", From: model.AttributeSource{Argument: "req.Signed"}},
		{Key: "request.string", From: model.AttributeSource{Argument: "req.ID"}},
		{Key: "result.int", From: model.AttributeSource{Result: "result.Big"}},
		{Key: "result.string", From: model.AttributeSource{Result: "result.Message"}},
		{Key: "fixed.bool", From: model.AttributeSource{Constant: false}},
	}
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{target}}
	original := append([]model.AttributePlan(nil), target.Attributes...)

	source, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}
	file := parseGeneratedHooks(t, source)
	if !reflect.DeepEqual(plan.Targets[0].Attributes, original) {
		t.Fatal("RenderHooks mutated target attributes")
	}

	_, expected, err := RenderAccessors(code, target)
	if err != nil {
		t.Fatal(err)
	}
	imports := importedPaths(file)
	if imports["example.com/accessorprobe/ops"] {
		t.Fatalf("generated hooks import application package: %s", source)
	}
	for _, binding := range expected {
		want := "//go:linkname read_" + binding.Function + " example.com/accessorprobe/ops." + binding.Function
		if !strings.Contains(string(source), want) {
			t.Errorf("missing linkname declaration %q", want)
		}
	}

	_, hooks, err := RenderRules(SupportedVersion, code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}
	before := findFunction(t, file, hooks[0].Before)
	after := findFunction(t, file, hooks[0].After)
	if !containsCall(before, "IsRecording") || !containsCall(after, "IsRecording") {
		t.Fatal("attribute access is not guarded by span.IsRecording")
	}
	for _, binding := range expected {
		if !containsCall(before, "read_"+binding.Function) && !containsCall(after, "read_"+binding.Function) {
			t.Errorf("accessor %q is not called", binding.Function)
		}
		if !strings.Contains(string(source), "read_"+binding.Function+"(") || !strings.Contains(string(source), ", ok") {
			t.Errorf("accessor %q does not expose availability handling", binding.Function)
		}
	}
	for _, want := range []string{"attribute.Bool(", "attribute.Float64(", "attribute.Int64(", "attribute.String("} {
		if !strings.Contains(string(source), want) {
			t.Errorf("missing scalar constructor %s", want)
		}
	}
	if strings.Index(string(source), "result.int") < strings.Index(string(source), "request.int") {
		t.Fatal("result attributes were emitted before argument attributes")
	}

	reordered := append([]model.AttributePlan(nil), target.Attributes...)
	sort.Slice(reordered, func(i, j int) bool { return reordered[i].Key > reordered[j].Key })
	plan.Targets[0].Attributes = reordered
	reorderedSource, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
	if err != nil || string(source) != string(reorderedSource) {
		t.Fatalf("attribute reordering changed generated hooks: %v", err)
	}
}

func TestAttributeHooksOffsetMethodReceiver(t *testing.T) {
	_, code, target := accessorFixture(t)
	symbol, ok := code.Symbol(target.SymbolID)
	if !ok {
		t.Fatal("fixture symbol missing")
	}
	method := *symbol
	method.Kind = model.SymbolMethod
	method.Receiver = &model.Receiver{Type: "Worker", Pointer: true}
	method.ID = model.MethodID(method.PackageImportPath, *method.Receiver, method.Name)
	method.Parameters = append([]model.Parameter(nil), symbol.Parameters...)
	method.Signature = "func(context.Context, *request, dep.Code) (output, error)"
	code.Symbols = append(code.Symbols, method)
	target.SymbolID = method.ID
	target.Signature = method.Signature
	target.SpanName = "method.operation"
	target.Attributes = []model.AttributePlan{{Key: "request.id", From: model.AttributeSource{Argument: "req.ID"}}}
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{target}}
	source, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}
	file := parseGeneratedHooks(t, source)
	_, bindings, err := RenderRules(SupportedVersion, code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = findFunction(t, file, bindings[0].Before)
	if !strings.Contains(string(source), "GetParam(1)") || !strings.Contains(string(source), "GetParam(2)") {
		t.Fatalf("method receiver offset was not applied to context and attribute arguments: %s", source)
	}
}

func TestAttributeHooksRejectsUnsafeAttributeWithoutPartialOutput(t *testing.T) {
	_, code, target := accessorFixture(t)
	target.Attributes = []model.AttributePlan{{
		Key:            "request.secret",
		From:           model.AttributeSource{Argument: "req.Secret"},
		Classification: model.ClassificationSecret,
	}}
	data, err := RenderHooks(SupportedVersion, "runtime", code, model.ResolvedPlan{Targets: []model.ResolvedTarget{target}}, hookImportPath)
	if err == nil || data != nil {
		t.Fatalf("unsafe attribute produced partial hooks: %q, %v", data, err)
	}
}

func importedPaths(file *ast.File) map[string]bool {
	paths := make(map[string]bool)
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, "\"")
		paths[path] = true
	}
	return paths
}

func containsCall(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == name {
			found = true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == name {
			found = true
		}
		return true
	})
	return found
}
