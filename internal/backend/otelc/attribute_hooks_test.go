package otelc

import (
	"errors"
	"go/ast"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestGenericAttributesUseDirectTypedInputs(t *testing.T) {
	t.Parallel()

	_, code, target := accessorFixture(t)
	symbol, _ := code.Symbol(target.SymbolID)
	symbol.Generics = new(model.GenericInfo)
	symbol.Generics.TypeParams = []string{"T"}
	target.ContextStrategy = model.ContextStrategy{Strategy: model.ContextStrategyRoot, Index: 0}
	target.SpanName = "generic.capture"
	target.ErrorStrategy = model.ErrorStrategy{Record: true, Indexes: []int{1}}
	argument := new(model.AttributePlan)
	argument.Key, argument.From.Argument = "request.id", "req.ID"
	result := new(model.AttributePlan)
	result.Key, result.From.Result = "result.message", "result.Message"
	target.Attributes = []model.AttributePlan{*argument, *result}
	plan := new(model.ResolvedPlan)
	plan.Targets = []model.ResolvedTarget{target}

	source, err := RenderHooks(SupportedVersion, "runtime", code, *plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{"GetParam(", "SetParam(", "GetReturnVal(", "SetReturnVal("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("generic capture called an unavailable backend API: %s", source)
		}
	}

	if !strings.Contains(string(source), "argument_1") || !strings.Contains(string(source), "returned_0") ||
		!strings.Contains(string(source), "returned_1.(error)") {
		t.Fatalf("generic capture omitted direct argument or result bindings: %s", source)
	}

	imports := importedPaths(parseGeneratedHooks(t, source))
	if imports[symbol.PackageImportPath] {
		t.Fatal("generic capture imports the application package")
	}
}

func TestAttributeHooksAreTypedAndBound(t *testing.T) {
	t.Parallel()

	_, code, target := accessorFixture(t)
	target.SpanName = "accessor.operation"
	target.Attributes = []model.AttributePlan{
		accessorAttributePlan("request.bool", argumentAttributeSource("req.Enabled")),
		accessorAttributePlan("request.float", argumentAttributeSource("req.Inner.Score")),
		accessorAttributePlan("request.int", argumentAttributeSource("req.Signed")),
		accessorAttributePlan("request.string", argumentAttributeSource("req.ID")),
		accessorAttributePlan("result.int", model.AttributeSource{Argument: "", Result: "result.Big", Constant: nil}),
		accessorAttributePlan("result.string", model.AttributeSource{Argument: "", Result: "result.Message", Constant: nil}),
		accessorAttributePlan("fixed.bool", constantAccessorSource(false)),
	}
	plan := resolvedPlanWithTargets(target)
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

	assertAttributeHookLinks(t, file, source, expected)

	_, hooks, err := RenderRules(SupportedVersion, code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	before := findFunction(t, file, hooks[0].Before)

	after := findFunction(t, file, hooks[0].After)

	assertAttributeHookReads(t, before, after, source, expected)
	assertAttributeHookConstructors(t, source)
	assertAttributeHookOrdering(t, source)

	assertAttributeHookReordering(t, code, plan, target.Attributes, source)
}

func assertAttributeHookLinks(t *testing.T, file *ast.File, source []byte, expected []AccessorBinding) {
	t.Helper()

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
}

func assertAttributeHookReads(t *testing.T, before, after *ast.FuncDecl, source []byte, expected []AccessorBinding) {
	t.Helper()

	if !containsCall(before, "IsRecording") || !containsCall(after, "IsRecording") {
		t.Fatal("attribute access is not guarded by span.IsRecording")
	}

	for _, binding := range expected {
		reader := "read_" + binding.Function
		readHook, otherHook := before, after

		if binding.Source == accessorResultSource {
			readHook, otherHook = after, before
		}

		assertAccessorReadGuarded(t, readHook, reader)

		if containsCall(otherHook, reader) {
			t.Errorf("accessor %q is called in the wrong lifecycle phase", binding.Function)
		}

		if !strings.Contains(string(source), reader+"(") || !strings.Contains(string(source), ", ok") {
			t.Errorf("accessor %q does not expose availability handling", binding.Function)
		}
	}
}

func assertAttributeHookConstructors(t *testing.T, source []byte) {
	t.Helper()

	for _, want := range []string{"attribute.Bool(", "attribute.Float64(", "attribute.Int64(", "attribute.String("} {
		if !strings.Contains(string(source), want) {
			t.Errorf("missing scalar constructor %s", want)
		}
	}
}

func assertAttributeHookOrdering(t *testing.T, source []byte) {
	t.Helper()

	if strings.Index(string(source), "result.int") < strings.Index(string(source), "request.int") {
		t.Fatal("result attributes were emitted before argument attributes")
	}
}

func assertAttributeHookReordering(t *testing.T, code *model.CodeModel, plan model.ResolvedPlan,
	attributes []model.AttributePlan, source []byte) {
	t.Helper()

	reordered := append([]model.AttributePlan(nil), attributes...)
	sort.Slice(reordered, func(i, j int) bool { return reordered[i].Key > reordered[j].Key })

	plan.Targets = append([]model.ResolvedTarget(nil), plan.Targets...)
	plan.Targets[0].Attributes = reordered

	reorderedSource, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
	if err != nil || string(source) != string(reorderedSource) {
		t.Fatalf("attribute reordering changed generated hooks: %v", err)
	}
}

func assertAccessorReadGuarded(t *testing.T, function *ast.FuncDecl, reader string) {
	t.Helper()

	guarded := recordingGuardedCalls(function)
	reads := 0

	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}

		callee, isIdentifier := call.Fun.(*ast.Ident)
		if !isIdentifier || callee.Name != reader {
			return true
		}

		reads++

		if !guarded[call] {
			t.Errorf("accessor %q is evaluated outside span.IsRecording", reader)
		}

		return true
	})

	if reads != 1 {
		t.Errorf("accessor %q is evaluated %d times, want once", reader, reads)
	}
}

func recordingGuardedCalls(function *ast.FuncDecl) map[*ast.CallExpr]bool {
	guarded := map[*ast.CallExpr]bool{}

	ast.Inspect(function.Body, func(node ast.Node) bool {
		statement, isIf := node.(*ast.IfStmt)
		if !isIf || !isRecordingGuard(statement) {
			return true
		}

		ast.Inspect(statement.Body, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if isCall {
				guarded[call] = true
			}

			return true
		})

		return true
	})

	return guarded
}

func isRecordingGuard(statement *ast.IfStmt) bool {
	call, isCall := statement.Cond.(*ast.CallExpr)
	if !isCall {
		return false
	}

	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "IsRecording" {
		return false
	}

	receiver, isIdentifier := selector.X.(*ast.Ident)

	return isIdentifier && receiver.Name == "span"
}

func TestAttributeHooksOffsetMethodReceiver(t *testing.T) {
	t.Parallel()

	_, code, target := accessorFixture(t)

	symbol, ok := code.Symbol(target.SymbolID)
	if !ok {
		t.Fatal("fixture symbol missing")
	}

	method := *symbol
	method.Kind = model.SymbolMethod
	method.Receiver = new(model.Receiver)
	method.Receiver.Type, method.Receiver.Pointer = "Worker", true
	method.ID = model.MethodID(method.PackageImportPath, *method.Receiver, method.Name)

	method.Parameters = append([]model.Parameter(nil), symbol.Parameters...)
	method.Signature = "func(context.Context, *request, dep.Code) (output, error)"
	code.Symbols = append(code.Symbols, method)
	target.SymbolID = method.ID
	target.Signature = method.Signature
	target.SpanName = "method.operation"
	target.Attributes = []model.AttributePlan{accessorAttributePlan("request.id", argumentAttributeSource("req.ID"))}
	plan := resolvedPlanWithTargets(target)

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
	t.Parallel()

	_, code, target := accessorFixture(t)
	target.SpanName = "unsafe.capture"
	target.Attributes = []model.AttributePlan{{
		Key:            "request.secret",
		From:           argumentAttributeSource("req.Secret"),
		Classification: model.ClassificationSecret,
		Allow:          false,
	}}

	plan := resolvedPlanWithTargets(target)

	data, err := RenderHooks(SupportedVersion, "runtime", code, plan, hookImportPath)
	if err == nil || data != nil {
		t.Fatalf("unsafe attribute produced partial hooks: %q, %v", data, err)
	}

	if !errors.Is(err, errAccessorUnsafePlan) {
		t.Fatalf("unsafe attribute failed for an unrelated reason: %v", err)
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
