package discovery_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestAttributeTypeGraph(t *testing.T) {
	t.Parallel()

	code := loadAttributeTypeGraph(t)
	request := graphType(t, code, "example.com/graph.Request")

	if request.Kind != "struct" || len(request.Fields) != 4 || !request.Fields[0].Embedded {
		t.Fatalf("invalid recursive struct shape: %+v", request)
	}

	details := graphType(t, code, "example.com/graph.Details")
	if len(details.Fields) != 2 || details.Fields[1].Exported || details.Fields[1].Name != "password" {
		t.Fatalf("private field metadata changed: %+v", details)
	}

	pointer := graphType(t, code, "*example.com/graph.Request")
	if pointer.Kind != "pointer" || pointer.Element != request.Type || request.Fields[1].Type != pointer.Type {
		t.Fatalf("recursive pointer identity changed: pointer=%+v request=%+v", pointer, request)
	}
}

func TestAttributeTypeGraphKindsAndAliases(t *testing.T) {
	t.Parallel()

	code := loadAttributeTypeGraph(t)
	kind := graphType(t, code, "example.com/graph.Kind")

	if kind.Kind != "string" {
		t.Fatalf("named primitive kind changed: %+v", kind)
	}

	for name, kind := range map[string]string{
		"bool": "bool", "int": "integer", "float64": "float", "string": "string",
		"[]byte": "slice", "map[string]int": "map", "[2]int": "array", "chan int": "channel",
		"func(int) string": "function", "interface{Run()}": "interface", "complex64": "unsupported",
	} {
		if got := graphType(t, code, name); got.Kind != kind {
			t.Fatalf("type %s kind=%s; want %s", name, got.Kind, kind)
		}
	}

	symbol, found := code.Symbol("example.com/graph.Execute")
	if !found || symbol.Parameters[0].Type != "*example.com/graph.Request" {
		t.Fatalf("ambiguous parameter type: %+v", symbol)
	}

	alias := graphType(t, code, "example.com/graph.RequestAlias")
	if alias.Kind != "struct" || !reflect.DeepEqual(alias.Fields, graphType(t, code, "example.com/graph.Request").Fields) {
		t.Fatalf("type alias lost its underlying fields: %+v", alias)
	}
}

func TestAttributeTypeGraphImportsAndDeterminism(t *testing.T) {
	t.Parallel()

	code := loadAttributeTypeGraph(t)
	generic := graphType(t, code, "example.com/graph.Box[example.com/graph/other.Value]")

	if len(generic.Imports) != 2 || generic.Imports[0].Path != "example.com/graph" ||
		generic.Imports[1].Path != "example.com/graph/other" {
		t.Fatalf("type import order or package identity changed: %+v", generic)
	}

	for _, dependency := range generic.Imports {
		digest := sha256.Sum256([]byte(dependency.Path))
		want := fmt.Sprintf("otelplanpkg_%x", digest[:8])

		if dependency.Alias != want || !strings.Contains(generic.Expression, want) {
			t.Fatalf("unstable type import alias: %+v", generic)
		}
	}

	seen := make(map[string]bool, len(code.Types))
	for _, info := range code.Types {
		if seen[info.Type] {
			t.Fatalf("duplicate discovered type: %s", info.Type)
		}

		seen[info.Type] = true
	}

	repeated := loadAttributeTypeGraph(t)
	if !reflect.DeepEqual(code.Types, repeated.Types) {
		t.Fatal("identical declarations produced a different type graph")
	}
}

func graphType(t *testing.T, code *model.CodeModel, name string) model.TypeInfo {
	t.Helper()

	for _, info := range code.Types {
		if info.Type == name {
			return info
		}
	}

	var missing model.TypeInfo

	t.Fatalf("missing type %s", name)

	return missing
}

func loadAttributeTypeGraph(t *testing.T) *model.CodeModel {
	t.Helper()

	root := t.TempDir()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := directory.Close()
		if err != nil {
			t.Error(err)
		}
	})

	err = directory.Mkdir("other", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"go.mod":         "module example.com/graph\n\ngo 1.27.0\n",
		"other/value.go": "package other\ntype Value struct { Number int }\n",
		"values.go": `package graph
import "example.com/graph/other"
type Kind string
type Details struct { Kind Kind; password string }
type Request struct { *Details; Next *Request; Count int; Payload []byte }
type RequestAlias = Request
type Box[T any] struct { Value T }
func Execute(request *Request) (result Details) { return Details{} }
func Alias(value RequestAlias) {}
func Generic(value Box[other.Value]) {}
func Kinds(a bool, b int, c float64, d string, e []byte, f map[string]int,
 g [2]int, h chan int, i func(int) string, j interface{Run()}, k complex64) {}
`,
	}

	for name, contents := range files {
		err := directory.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}
