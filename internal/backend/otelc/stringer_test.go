package otelc_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestStringerReadersAndGuardPreserveBusinessPanics(t *testing.T) {
	t.Parallel()

	root, code, target := stringerReaderTarget(t)

	source, bindings, err := otelc.RenderAccessors(code, target)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(source), "recover(") {
		t.Fatal("application accessor owns panic recovery")
	}

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{target}

	hooks, err := otelc.RenderHooks(otelc.SupportedVersion, "test", code, plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(hooks, []byte("otelplanReadStringer")) || !bytes.Contains(hooks, []byte("if span.IsRecording()")) {
		t.Fatal("Stringer hook lacks guarded runtime extraction")
	}

	guard := extractStringerGuard(t, hooks)
	assertStringerCallsRecordingGuarded(t, hooks, len(bindings)-1)
	compileStringerReaders(t, root, source, guard, bindings)
}

func stringerReaderTarget(t *testing.T) (*os.Root, *model.CodeModel, model.ResolvedTarget) {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	for name, contents := range map[string]string{
		"go.mod": "module example.com/stringer\n\ngo 1.27\n",
		"app.go": stringerReaderFixture,
	} {
		err := root.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	var options discovery.Options

	options.Root, options.Offline = root.Name(), true

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	symbol, exists := code.Symbol("example.com/stringer.Run")
	if !exists {
		t.Fatal("Stringer fixture symbol missing")
	}

	var target model.ResolvedTarget

	target.SymbolID, target.Signature, target.SpanName = symbol.ID, symbol.Signature, "run"
	target.ContextStrategy.Strategy = model.ContextStrategyArgument

	for _, selector := range []string{"value", "pointer", "category", "bad", "nested.Label", "printable"} {
		var attribute model.AttributePlan

		attribute.Key, attribute.From.Argument, attribute.Stringer = selector, selector, true
		target.Attributes = append(target.Attributes, attribute)
	}

	var plain, result model.AttributePlan

	plain.Key, plain.From.Argument = "plain", "category"
	result.Key, result.From.Result, result.Stringer = "result", "result", true
	target.Attributes = append(target.Attributes, plain, result)

	return root, code, target
}

func compileStringerReaders(t *testing.T, root *os.Root, source, guard []byte, bindings []otelc.AccessorBinding) {
	t.Helper()

	var tests bytes.Buffer
	tests.WriteString("package hooks\nimport (\"testing\"; application \"example.com/stringer\")\n")
	tests.WriteString("var readers = map[string]func(any)(string,bool){\n")

	for _, binding := range bindings {
		if binding.Kind != "string" || binding.Stringer != (binding.Key != "plain") {
			t.Fatalf("incorrect Stringer binding: %+v", binding)
		}

		fmt.Fprintf(&tests, "%q: application.%s,\n", binding.Key, binding.Function)
	}

	tests.WriteString("}\n" + stringerReaderAssertions)

	for name, contents := range map[string][]byte{
		"shadow.go":           []byte("package app\nfunc recover() any { panic(\"application recover must not run\") }\n"),
		"readers.go":          []byte(strings.TrimPrefix(string(source), "//go:build ignore\n\n")),
		"hooks/guard.go":      append([]byte("package hooks\n"), guard...),
		"hooks/guard_test.go": tests.Bytes(),
	} {
		err := root.MkdirAll("hooks", 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = root.WriteFile(name, contents, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	command := exec.CommandContext(t.Context(), "go", "test", "./...")
	command.Dir = root.Name()

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local",
		"GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Stringer reader/guard contract failed: %v\n%s", err, output)
	}
}

const stringerReaderAssertions = `
func TestValues(t *testing.T) {
 wants := map[string]string{"value":"value:input", "pointer":"pointer:input", "category":"category:raw",
  "plain":"raw", "nested.Label":"pointer:nested", "result":"value:output", "printable":"value:interface"}
 for key, want := range wants {
  value, ok := otelplanReadStringer(readers[key], application.Input(key))
  if !ok || value != want { t.Fatalf("key=%s value=%s ok=%t", key, value, ok) }
 }
 if value, ok := otelplanReadStringer(readers["bad"], application.Input("bad")); ok || value != "" {
  t.Fatal("panicking Stringer produced a value")
 }
 for _, key := range []string{"pointer", "nested.Label"} {
  if value, ok := otelplanReadStringer(readers[key], nil); ok || value != "" { t.Fatal("nil source captured") }
 }
 if _, ok := otelplanReadStringer(readers["pointer"], application.NilPointer()); ok { t.Fatal("nil pointer captured") }
 if _, ok := otelplanReadStringer(readers["printable"], application.NilPointer()); ok {
  t.Fatal("nil interface method panic captured")
 }
 for _, value := range []any{application.NilPointer(), application.NilEnvelope(), application.EmptyEnvelope()} {
  if captured, ok := otelplanReadStringer(readers["nested.Label"], value); ok || captured != "" {
   t.Fatal("nil path captured")
  }
 }
 if value, ok := otelplanReadStringer(func(any)(string,bool){panic(nil)}, nil); ok || value != "" {
  t.Fatal("nil panic was not omitted")
 }
}
func TestBusinessPanic(t *testing.T) {
 for _, key := range []string{"value", "bad"} {
  func() {
   defer func() {
    if recovered := recover(); recovered != "business-panic" { t.Fatal("business panic changed", recovered) }
   }()
   business(key)
   t.Fatal("business panic was swallowed")
  }()
 }
}
func business(key string) {
 defer func() { _, _ = otelplanReadStringer(readers[key], application.Input(key)) }()
 panic("business-panic")
}
`

const stringerReaderFixture = `package app
import "context"
type Value struct{ Text string }
func (value Value) String() string { return "value:"+value.Text }
type Pointer struct{ Text string }
func (value *Pointer) String() string { return "pointer:"+value.Text }
type Category string
func (value Category) String() string { return "category:"+string(value) }
type Bad struct{}
func (Bad) String() string { panic("caller-private-value") }
type Envelope struct{ Label *Pointer }
type Printable interface{ String() string }
func Run(ctx context.Context, value Value, pointer *Pointer, category Category, bad Bad, nested *Envelope,
 printable Printable)(result Value) {
 return value
}
func Input(key string) any {
 switch key {
 case "value":return Value{Text:"input"}
 case "pointer":return &Pointer{Text:"input"}
 case "category", "plain":return Category("raw")
 case "bad":return Bad{}
 case "nested.Label":return &Envelope{Label:&Pointer{Text:"nested"}}
 case "result":return Value{Text:"output"}
 case "printable":return Value{Text:"interface"}
 }
 return nil
}
func NilPointer() any { return (*Pointer)(nil) }
func NilEnvelope() any { return (*Envelope)(nil) }
func EmptyEnvelope() any { return &Envelope{} }
`

func assertStringerCallsRecordingGuarded(t *testing.T, source []byte, want int) {
	t.Helper()
	file := parseGeneratedHooks(t, source)
	calls := 0

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil || function.Name.Name == "otelplanReadStringer" {
			continue
		}

		guarded := stringerRecordingCalls(function)
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}

			name, isIdentifier := call.Fun.(*ast.Ident)
			if !isIdentifier || name.Name != "otelplanReadStringer" {
				return true
			}

			calls++

			if !guarded[call] {
				t.Error("formatting runs for a non-recording span")
			}

			return true
		})
	}

	if calls != want {
		t.Fatalf("Stringer calls=%d; want %d", calls, want)
	}
}

func extractStringerGuard(t *testing.T, source []byte) []byte {
	t.Helper()

	positions := token.NewFileSet()

	file, err := parser.ParseFile(positions, "hooks.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "otelplanReadStringer" {
			continue
		}

		var body bytes.Buffer

		err := format.Node(&body, positions, function)
		if err != nil {
			t.Fatal(err)
		}

		return body.Bytes()
	}

	t.Fatal("Stringer guard missing")

	return nil
}

func stringerRecordingCalls(function *ast.FuncDecl) map[*ast.CallExpr]bool {
	guarded := map[*ast.CallExpr]bool{}

	ast.Inspect(function.Body, func(node ast.Node) bool {
		statement, ok := node.(*ast.IfStmt)
		if !ok || !stringerRecordingGuard(statement) {
			return true
		}

		ast.Inspect(statement.Body, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				guarded[call] = true
			}

			return true
		})

		return true
	})

	return guarded
}

func stringerRecordingGuard(statement *ast.IfStmt) bool {
	call, isCall := statement.Cond.(*ast.CallExpr)
	if !isCall {
		return false
	}

	selector, ok := call.Fun.(*ast.SelectorExpr)

	return ok && selector.Sel.Name == "IsRecording"
}
