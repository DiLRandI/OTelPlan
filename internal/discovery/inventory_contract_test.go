package discovery_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"slices"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const inventoryPortsSource = `package ports
import "context"
type Boundary interface { Execute(context.Context) error }
`

const inventoryOperationsSource = `package shop
import "context"
type Worker struct{}
func (*Worker) Execute(ctx context.Context) error { return nil }
func helper(ctx context.Context) error { return nil }
func Generic[T any](ctx context.Context, v T) T { return v }
type Alias = context.Context
func AliasContext(ctx Alias) {}
`

func TestLoadSemanticInventory(t *testing.T) {
	t.Parallel()

	root, directory, files := inventoryFixture(t)
	options := inventoryOptions(root)

	code := loadInventory(t, options)
	assertDefaultInventory(t, code)
	assertDeterministicInventory(t, code, options)

	options.IncludeTests = true
	options.BuildTags = []string{"special"}
	withTests := loadInventory(t, options)
	assertTestInventory(t, withTests)
	assertUniqueSymbols(t, withTests)
	checkInventoryFiles(t, directory, files)
}

func TestLoadInvalidProject(t *testing.T) {
	t.Parallel()

	root, directory, files := inventoryFixture(t)
	writeInventoryFile(t, directory, "broken.go", "package shop\nfunc Broken( {")

	files["broken.go"] = "package shop\nfunc Broken( {"

	code, err := discovery.LoadContext(t.Context(), inventoryOptions(root))
	if err == nil || code != nil {
		t.Fatalf("invalid Go project result=%+v error=%v", code, err)
	}

	checkInventoryFiles(t, directory, files)
}

func inventoryOptions(root string) discovery.Options {
	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	return options
}

func loadInventory(t *testing.T, options discovery.Options) *model.CodeModel {
	t.Helper()

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func assertDefaultInventory(t *testing.T, code *model.CodeModel) {
	t.Helper()

	assertContextSymbols(t, code)
	assertGeneratedClassification(t, code)
	assertInterfaceRelations(t, code)
	assertOmittedSymbols(t, code)
}

func assertContextSymbols(t *testing.T, code *model.CodeModel) {
	t.Helper()

	for _, name := range []string{"helper", "Generic", "AliasContext"} {
		symbol, exists := code.Symbol(model.FunctionID("example.com/shop", name))
		if !exists || !symbol.HasContext() || symbol.Location.File != "operations.go" ||
			symbol.Location.Line == 0 || symbol.Ownership != model.OwnershipApplication {
			t.Fatalf("invalid %s: %+v", name, symbol)
		}
	}

	generic, exists := code.Symbol("example.com/shop.Generic")
	if !exists || generic.Generics == nil || len(generic.Generics.TypeParams) != 1 {
		t.Fatalf("generics: %+v", generic)
	}
}

func assertGeneratedClassification(t *testing.T, code *model.CodeModel) {
	t.Helper()

	generated, exists := code.Symbol("example.com/shop.Generated")
	if !exists {
		t.Fatal("generated symbol missing")
	}

	realSymbol, exists := code.Symbol("example.com/shop.Real")
	if !exists {
		t.Fatal("real symbol missing")
	}

	if !generated.Generated || realSymbol.Generated {
		t.Fatal("generated classification must use source marker")
	}
}

func assertInterfaceRelations(t *testing.T, code *model.CodeModel) {
	t.Helper()

	implementors := code.Implementors("example.com/shop/ports", "Boundary")
	if len(implementors) != 1 || !implementors[0].Pointer ||
		implementors[0].InterfaceID != "example.com/shop/ports.Boundary" {
		t.Fatalf("interface relations: %+v", implementors)
	}
}

func assertOmittedSymbols(t *testing.T, code *model.CodeModel) {
	t.Helper()

	for _, name := range []model.SymbolID{"example.com/shop.testHelper", "example.com/shop.Tagged"} {
		if _, exists := code.Symbol(name); exists {
			t.Fatalf("unexpected symbol %s", name)
		}
	}
}

func assertDeterministicInventory(t *testing.T, first *model.CodeModel, options discovery.Options) {
	t.Helper()

	second := loadInventory(t, options)

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first inventory: %v", err)
	}

	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second inventory: %v", err)
	}

	if !slices.Equal(firstJSON, secondJSON) {
		t.Fatal("inventory is not deterministic")
	}
}

func assertTestInventory(t *testing.T, code *model.CodeModel) {
	t.Helper()

	testSymbol, exists := code.Symbol("example.com/shop.testHelper")
	if !exists || !testSymbol.TestFile {
		t.Fatalf("test symbol: %+v", testSymbol)
	}

	if _, exists := code.Symbol("example.com/shop.Tagged"); !exists {
		t.Fatal("tagged symbol missing")
	}
}

func assertUniqueSymbols(t *testing.T, code *model.CodeModel) {
	t.Helper()

	seen := make(map[model.SymbolID]bool, len(code.Symbols))
	for _, symbol := range code.Symbols {
		if seen[symbol.ID] {
			t.Fatalf("duplicate %s", symbol.ID)
		}

		seen[symbol.ID] = true
	}
}

func inventoryFixture(t *testing.T) (string, *os.Root, map[string]string) {
	t.Helper()

	rootPath := t.TempDir()

	directory, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	files := map[string]string{
		"go.mod":             "module example.com/shop\n\ngo 1.27\n",
		"ports/port.go":      inventoryPortsSource,
		"operations.go":      inventoryOperationsSource,
		"generated.go":       "// Code generated by fixture. DO NOT EDIT.\npackage shop\nfunc Generated() {}\n",
		"mock_real.go":       "package shop\nfunc Real() {}\n",
		"operations_test.go": "package shop\nfunc testHelper() {}\n",
		"tagged.go":          "//go:build special\n\npackage shop\nfunc Tagged() {}\n",
	}
	for name, contents := range files {
		writeInventoryFile(t, directory, name, contents)
	}

	return rootPath, directory, files
}

func writeInventoryFile(t *testing.T, directory *os.Root, name, contents string) {
	t.Helper()

	err := directory.MkdirAll(path.Dir(name), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = directory.WriteFile(name, []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func checkInventoryFiles(t *testing.T, directory *os.Root, files map[string]string) {
	t.Helper()

	expectedFiles := make(map[string]bool, len(files))
	for name := range files {
		expectedFiles[name] = true
	}

	seenFiles := make(map[string]bool, len(files))

	walkErr := fs.WalkDir(directory.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		if !expectedFiles[name] {
			t.Fatalf("unexpected fixture file %s", name)
		}

		seenFiles[name] = true

		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}

	if len(seenFiles) != len(expectedFiles) {
		t.Fatalf("fixture file set changed: got %d files, want %d", len(seenFiles), len(expectedFiles))
	}

	for name, want := range files {
		got, readErr := directory.ReadFile(name)
		if readErr != nil || string(got) != want {
			t.Fatalf("discovery changed %s: %v", name, readErr)
		}
	}
}
