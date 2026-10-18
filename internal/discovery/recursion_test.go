package discovery_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestDiscoveryMarksStaticRecursiveSymbols(t *testing.T) {
	t.Parallel()

	root := recursionFixture(t)
	code := loadRecursionModel(t, root, false)
	recursive := recursiveSymbols(code)
	assertRecursiveFunctions(t, recursive)
	assertRecursiveMethods(t, recursive)
	assertOptionalGraphPreservesRecursion(t, root)
	assertRecursionDeterministic(t, root, code)
	assertRecursionRelocationAndCache(t, root, recursive)
}

func assertRecursiveFunctions(t *testing.T, recursive map[model.SymbolID]bool) {
	t.Helper()

	for _, name := range []string{"Direct", "MutualA", "MutualB", "Generic", "AliasEntry", "AliasHelper", "Immediate"} {
		if !recursive[model.SymbolID("example.com/recursion."+name)] {
			t.Errorf("%s was not marked recursive: %+v", name, recursive)
		}
	}

	for _, name := range []string{"Wrapper", "ClosureOnly"} {
		if recursive[model.SymbolID("example.com/recursion."+name)] {
			t.Errorf("%s was incorrectly marked recursive: %+v", name, recursive)
		}
	}
}

func assertRecursiveMethods(t *testing.T, recursive map[model.SymbolID]bool) {
	t.Helper()

	valueMethod := recursive["example.com/recursion.(Methods).Value"]

	pointerMethod := recursive["example.com/recursion.(*Methods).Pointer"]
	if !valueMethod || !pointerMethod {
		t.Fatalf("method identities were not preserved: %+v", recursive)
	}

	if !recursive["example.com/recursion.(Base).Run"] {
		t.Fatalf("promoted method cycle was not marked recursive: %+v", recursive)
	}
}

func assertOptionalGraphPreservesRecursion(t *testing.T, root string) {
	t.Helper()

	withoutGraph := loadRecursionModel(t, root, false)
	withGraph := loadRecursionModel(t, root, true)

	if !slices.EqualFunc(withoutGraph.Symbols, withGraph.Symbols, func(left, right model.Symbol) bool {
		return left.ID == right.ID && left.Recursive == right.Recursive
	}) {
		t.Fatal("optional call graph changed recursion metadata")
	}
}

func assertRecursionDeterministic(t *testing.T, root string, first *model.CodeModel) {
	t.Helper()

	repeated := loadRecursionModel(t, root, false)

	firstJSON, err := json.Marshal(first.Symbols)
	if err != nil {
		t.Fatal(err)
	}

	secondJSON, err := json.Marshal(repeated.Symbols)
	if err != nil {
		t.Fatal(err)
	}

	if string(firstJSON) != string(secondJSON) {
		t.Fatal("recursion metadata was not deterministic")
	}
}

func assertRecursionRelocationAndCache(t *testing.T, root string, want map[model.SymbolID]bool) {
	t.Helper()

	relocatedRoot := t.TempDir()

	err := os.CopyFS(relocatedRoot, os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(want, recursiveSymbols(loadRecursionModel(t, relocatedRoot, false))) {
		t.Fatal("relocation changed recursion metadata")
	}

	cache := t.TempDir()

	var cachedOptions discovery.Options

	cachedOptions.Root, cachedOptions.CacheDir, cachedOptions.Offline = root, cache, true

	warm, err := discovery.LoadContext(t.Context(), cachedOptions)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(want, recursiveSymbols(warm)) {
		t.Fatal("cached recursion metadata differs from uncached metadata")
	}

	cachedOptions.CacheReadOnly = true

	cached, err := discovery.LoadContext(t.Context(), cachedOptions)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(recursiveSymbols(warm), recursiveSymbols(cached)) {
		t.Fatal("cached recursion metadata differs from uncached metadata")
	}
}

func loadRecursionModel(t *testing.T, root string, callGraph bool) *model.CodeModel {
	t.Helper()

	var options discovery.Options

	options.Root, options.Offline, options.CallGraph = root, true, callGraph

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func recursiveSymbols(code *model.CodeModel) map[model.SymbolID]bool {
	result := make(map[model.SymbolID]bool)
	for _, symbol := range code.Symbols {
		result[symbol.ID] = symbol.Recursive
	}

	return result
}

func recursionFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	files := map[string]string{
		"go.mod": `module example.com/recursion

go 1.27
`,
		"recursion.go": `package recursion

func Direct() { Direct() }
func MutualA() { MutualB() }
func MutualB() { MutualA() }
func Wrapper() { MutualA() }
func ClosureOnly() { _ = func() { ClosureOnly() } }
func Immediate() { func() { Immediate() }() }
func Generic[T any]() { Generic[T]() }

type Named int
type Alias = Named
func AliasEntry(value Alias) { AliasHelper(value) }
func AliasHelper(value Named) { AliasEntry(value) }

type Methods struct{}
func (Methods) Value() { (&Methods{}).Pointer() }
func (*Methods) Pointer() { var value Methods; value.Value() }

type Base struct{}
type Outer struct{ Base }
func (Base) Run() { var value Outer; value.Run() }
`,
	}
	for name, contents := range files {
		err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root
}
