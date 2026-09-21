package discovery_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestAdvisoryCallGraph(t *testing.T) {
	t.Parallel()

	root := callGraphFixture(t)
	opts := callGraphOptions(root)
	opts.CallGraph = false

	basic := loadCallGraphFixture(t, opts)

	if basic.CallGraph != nil || len(basic.CallEdges) != 0 {
		t.Fatal("basic analysis built a call graph")
	}

	opts.CallGraph = true
	first := loadCallGraphFixture(t, opts)

	checkCallGraph(t, first)

	second := loadCallGraphFixture(t, opts)

	if !reflect.DeepEqual(first.CallEdges, second.CallEdges) {
		t.Fatal("call graph changed on repeated analysis")
	}

	relocatedRoot := t.TempDir()

	err := os.CopyFS(relocatedRoot, os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}

	opts.Root = relocatedRoot
	relocated := loadCallGraphFixture(t, opts)

	if !reflect.DeepEqual(first.CallEdges, relocated.CallEdges) {
		t.Fatal("checkout relocation changed calls")
	}

	first.CallGraph, first.CallEdges = nil, nil

	basicJSON, err := json.Marshal(basic)
	if err != nil {
		t.Fatal(err)
	}

	advancedJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}

	if string(basicJSON) != string(advancedJSON) {
		t.Fatal("call graph changed semantic inventory")
	}
}

func TestCallGraphCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := discovery.LoadContext(ctx, callGraphOptions(t.TempDir()))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation during discovery, got %v", err)
	}
}

func callGraphFixture(t *testing.T) string {
	t.Helper()

	files := map[string]string{"calls.go": `package shop
import "strings"
type Boundary interface { Run() }
type A struct{}
func (A) Run() {}
type B struct{}
func (*B) Run() {}
type Embedded struct { A }
func Promoted(value Embedded) { value.Run() }
func Bound(value A) { f := value.Run; f() }
func Target() {}
func Direct() { Target(); Target() }
func Dispatch(b Boundary) { b.Run() }
func Indirect(f func()) { f() }
func Generic[T any](value T) T { return value }
func UseGeneric() { Generic(1) }
func External() { strings.TrimSpace(" value ") }
func Closure() func() { return func() { Target() } }
`}

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

	files["go.mod"] = "module example.com/shop\n\ngo 1.27\n"

	for name, content := range files {
		err := directory.WriteFile(name, []byte(content), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func checkCallGraph(t *testing.T, first *model.CodeModel) {
	t.Helper()

	if first.CallGraph == nil || first.CallGraph.Algorithm != "cha" ||
		!first.CallGraph.Conservative || len(first.CallGraph.Limitations) == 0 {
		t.Fatal("missing call graph precision metadata")
	}

	const (
		valueMethod    = "example.com/shop.(A).Run"
		targetFunction = "example.com/shop.Target"
	)

	expected := []model.CallRelation{
		{Caller: "example.com/shop.Promoted", Callee: valueMethod, Precision: model.CallPrecisionStatic},
		{Caller: "example.com/shop.Bound", Callee: valueMethod, Precision: model.CallPrecisionStatic},
		{Caller: "example.com/shop.Direct", Callee: targetFunction, Precision: model.CallPrecisionStatic},
		{Caller: "example.com/shop.Dispatch", Callee: valueMethod, Precision: model.CallPrecisionConservative},
		{
			Caller:    "example.com/shop.Dispatch",
			Callee:    "example.com/shop.(*B).Run",
			Precision: model.CallPrecisionConservative,
		},
		{Caller: "example.com/shop.Indirect", Callee: targetFunction, Precision: model.CallPrecisionConservative},
		{Caller: "example.com/shop.UseGeneric", Callee: "example.com/shop.Generic", Precision: model.CallPrecisionStatic},
		{Caller: "example.com/shop.External", Callee: "strings.TrimSpace", Precision: model.CallPrecisionStatic},
		{Caller: "example.com/shop.Closure", Callee: targetFunction, Precision: model.CallPrecisionConservative},
	}

	for _, want := range expected {
		count := 0

		for _, edge := range first.CallEdges {
			if edge == want {
				count++
			}
		}

		if count != 1 {
			t.Fatalf("edge %+v count=%d; graph=%+v", want, count, first.CallEdges)
		}
	}
}

func loadCallGraphFixture(t *testing.T, opts discovery.Options) *model.CodeModel {
	t.Helper()

	code, err := discovery.Load(opts)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func callGraphOptions(root string) discovery.Options {
	return discovery.Options{
		Root:                root,
		Patterns:            nil,
		BuildTags:           nil,
		BuildFlags:          nil,
		CallGraph:           true,
		IncludeTests:        false,
		IncludeDependencies: false,
		GOOS:                "",
		GOARCH:              "",
		Env:                 nil,
		Offline:             true,
	}
}
