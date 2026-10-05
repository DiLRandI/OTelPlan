package discovery_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const symbolIndexSource = `package symbols
import "context"
type ContextAlias = context.Context
type ErrorAlias = error
type NamedError interface { Error() string }
type Worker struct{}
func (worker *Worker) Method(other int, ctx ContextAlias) (NamedError, ErrorAlias) { return nil, nil }
func (Worker) Value() {}
type Box[T any] struct{ value T }
func (box *Box[T]) Handle(ctx context.Context, value T) (T, error) { return value, nil }
func Generic[T any](other int, ctx context.Context, rest ...T) (out T, err error) { return out, nil }
func hidden() {}
func init() {}
func _() {}
`

func TestSymbolIndexDeclarationMetadata(t *testing.T) {
	t.Parallel()

	code := loadSymbolIndexFixture(t)
	generic := indexedSymbol(t, code, "example.com/symbols.Generic")
	assertIndexedDeclaration(t, generic, model.SymbolFunction, model.VisibilityExported, true, "symbols.go")

	if !generic.Variadic || !slices.Equal(generic.ContextIndexes, []int{1}) ||
		!slices.Equal(generic.ErrorIndexes, []int{1}) {
		t.Fatalf("generic signature indexes changed: %+v", generic)
	}

	assertIndexedTypeParameters(t, generic)

	if !strings.HasPrefix(generic.Signature, "func[T any]") || generic.Parameters[2].Name != "rest" ||
		generic.Results[0].Name != "out" || generic.Results[1].Name != "err" {
		t.Fatalf("generic declaration signature changed: %+v", generic)
	}

	hidden := indexedSymbol(t, code, "example.com/symbols.hidden")
	assertIndexedDeclaration(t, hidden, model.SymbolFunction, model.VisibilityUnexported, true, "symbols.go")

	assembly := indexedSymbol(t, code, "example.com/symbols.Assembly")
	assertIndexedDeclaration(t, assembly, model.SymbolFunction, model.VisibilityExported, false, "assembly.go")

	for _, omitted := range []model.SymbolID{"example.com/symbols.init", "example.com/symbols._"} {
		_, exists := code.Symbol(omitted)
		if exists {
			t.Fatalf("nonselectable declaration indexed: %s", omitted)
		}
	}
}

func TestSymbolIndexReceiverAndSourceFlags(t *testing.T) {
	t.Parallel()

	code := loadSymbolIndexFixture(t)
	method := indexedSymbol(t, code, "example.com/symbols.(*Worker).Method")
	assertIndexedDeclaration(t, method, model.SymbolMethod, model.VisibilityExported, true, "symbols.go")
	assertIndexedReceiver(t, method, "worker", "Worker", true)

	if !slices.Equal(method.ContextIndexes, []int{1}) || !slices.Equal(method.ErrorIndexes, []int{1}) {
		t.Fatalf("context/error aliases or named error identity changed: %+v", method)
	}

	value := indexedSymbol(t, code, "example.com/symbols.(Worker).Value")
	assertIndexedReceiver(t, value, "", "Worker", false)
	box := indexedSymbol(t, code, "example.com/symbols.(*Box).Handle")
	assertIndexedReceiver(t, box, "box", "Box", true)
	assertIndexedTypeParameters(t, box)

	generated := indexedSymbol(t, code, "example.com/symbols.Generated")
	assertIndexedDeclaration(t, generated, model.SymbolFunction, model.VisibilityExported, true, "generated.go")

	if !generated.Generated || generated.TestFile {
		t.Fatalf("generated-file classification changed: %+v", generated)
	}

	testOnly := indexedSymbol(t, code, "example.com/symbols.testOnly")
	assertIndexedDeclaration(t, testOnly, model.SymbolFunction, model.VisibilityUnexported, true, "symbols_test.go")

	if !testOnly.TestFile || testOnly.Generated {
		t.Fatalf("test-file classification changed: %+v", testOnly)
	}
}

func assertIndexedDeclaration(t *testing.T, symbol *model.Symbol, kind model.SymbolKind,
	visibility model.Visibility, body bool, file string) {
	t.Helper()

	if symbol.Kind != kind || symbol.Visibility != visibility || symbol.HasBody != body ||
		symbol.Ownership != model.OwnershipApplication || symbol.PackageImportPath != "example.com/symbols" ||
		symbol.PackageName != "symbols" || symbol.Location.File != file || symbol.Location.Line <= 0 ||
		symbol.Location.Column <= 0 {
		t.Fatalf("declaration metadata changed: %+v", symbol)
	}
}

func assertIndexedReceiver(t *testing.T, symbol *model.Symbol, name, receiverType string, pointer bool) {
	t.Helper()

	if symbol.Receiver == nil || symbol.Receiver.Name != name || symbol.Receiver.Type != receiverType ||
		symbol.Receiver.Pointer != pointer {
		t.Fatalf("receiver identity changed: %+v", symbol)
	}
}

func assertIndexedTypeParameters(t *testing.T, symbol *model.Symbol) {
	t.Helper()

	if symbol.Generics == nil || !slices.Equal(symbol.Generics.TypeParams, []string{"T"}) {
		t.Fatalf("declaration type parameters changed: %+v", symbol)
	}
}

func indexedSymbol(t *testing.T, code *model.CodeModel, id model.SymbolID) *model.Symbol {
	t.Helper()

	symbol, exists := code.Symbol(id)
	if !exists {
		t.Fatalf("missing declaration %s", id)
	}

	return symbol
}

func loadSymbolIndexFixture(t *testing.T) *model.CodeModel {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Errorf("close symbol index fixture: %v", closeErr)
		}
	})

	files := map[string]string{
		"go.mod":          "module example.com/symbols\n\ngo 1.27\n",
		"symbols.go":      symbolIndexSource,
		"generated.go":    "// Code generated by fixture. DO NOT EDIT.\npackage symbols\nfunc Generated() {}\n",
		"symbols_test.go": "package symbols\nfunc testOnly() {}\n",
		"assembly.go":     "package symbols\n//go:noescape\nfunc Assembly()\n",
		"assembly.s":      "// fixture assembly file\n",
	}
	for name, contents := range files {
		err = root.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	var options discovery.Options

	options.Root = root.Name()
	options.Offline, options.IncludeTests = true, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}
