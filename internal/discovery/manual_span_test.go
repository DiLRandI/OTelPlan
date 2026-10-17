package discovery_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestDiscoveryMarksManualSpanCalls(t *testing.T) {
	t.Setenv("GOOS", runtime.GOOS)
	t.Setenv("GOARCH", runtime.GOARCH)
	testenv.PrepareOffline(t, "../cli/testdata/architectures/common")
	root := manualSpanFixture(t)

	first := loadManualSpanModel(t, root, false)
	manual := manualSpanSymbols(first)
	assertManualSpanClassification(t, manual)
	assertManualSpanDeterministic(t, root, first)
	assertManualSpanCacheAndRelocation(t, root, manual)
}

func assertManualSpanClassification(t *testing.T, manual map[model.SymbolID]bool) {
	t.Helper()

	for _, symbolID := range []model.SymbolID{
		"example.com/manual.Direct", "example.com/manual.Alias", "example.com/manual.Wrapped",
		"example.com/manual.Promoted", "example.com/manual.Expression", "example.com/manual.Immediate",
		"example.com/manual.Helper", "example.com/manual.AliasType",
	} {
		if !manual[symbolID] {
			t.Errorf("manual span call not marked for %s", symbolID)
		}
	}

	for _, symbolID := range []model.SymbolID{
		"example.com/manual.Unrelated", "example.com/manual.Parent", "example.com/manual.Stored", "example.com/manual.Bound",
	} {
		if manual[symbolID] {
			t.Errorf("unrelated or indirect symbol marked manual: %s", symbolID)
		}
	}
}

func assertManualSpanDeterministic(t *testing.T, root string, first *model.CodeModel) {
	t.Helper()

	second := loadManualSpanModel(t, root, false)
	if !reflect.DeepEqual(manualSpanSymbols(first), manualSpanSymbols(second)) {
		t.Fatal("manual span metadata was not deterministic")
	}

	withGraph := loadManualSpanModel(t, root, true)
	if !reflect.DeepEqual(manualSpanSymbols(first), manualSpanSymbols(withGraph)) {
		t.Fatal("optional call graph changed manual span metadata")
	}

	firstJSON, err := json.Marshal(first.Symbols)
	if err != nil {
		t.Fatal(err)
	}

	secondJSON, err := json.Marshal(second.Symbols)
	if err != nil || string(firstJSON) != string(secondJSON) {
		t.Fatalf("repeated manual span inventory differs: %v", err)
	}

	if strings.Contains(string(firstJSON), "private-span-label-do-not-log") {
		t.Fatal("manual-span metadata exposed a span-name value")
	}
}

func assertManualSpanCacheAndRelocation(t *testing.T, root string, want map[model.SymbolID]bool) {
	t.Helper()

	relocated := t.TempDir()

	err := os.CopyFS(relocated, os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(want, manualSpanSymbols(loadManualSpanModel(t, relocated, false))) {
		t.Fatal("checkout relocation changed manual-span metadata")
	}

	var options discovery.Options

	options.Root, options.Offline, options.CacheDir = root, true, t.TempDir()

	warm, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	options.CacheReadOnly = true

	cached, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(want, manualSpanSymbols(warm)) || !reflect.DeepEqual(want, manualSpanSymbols(cached)) {
		t.Fatal("cached manual-span metadata differs from fresh discovery")
	}
}

func loadManualSpanModel(t *testing.T, root string, callGraph bool) *model.CodeModel {
	t.Helper()

	var options discovery.Options

	options.Root, options.Offline, options.CallGraph = root, true, callGraph

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func manualSpanSymbols(code *model.CodeModel) map[model.SymbolID]bool {
	result := make(map[model.SymbolID]bool)
	for _, symbol := range code.Symbols {
		result[symbol.ID] = symbol.ManualSpan
	}

	return result
}

func manualSpanFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	files := map[string]string{
		"manual.go": `package manual

import (
	"context"
	"go.opentelemetry.io/otel/trace"
	tracealias "go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer
var aliased tracealias.Tracer

func Direct(ctx context.Context) { tracer.Start(ctx, "direct") }
func Alias(ctx context.Context) { aliased.Start(ctx, "alias") }
type AliasTracer = trace.Tracer
func AliasType(ctx context.Context, value AliasTracer) { value.Start(ctx, "alias type") }
func Unrelated(ctx context.Context) { unrelatedTracer{}.Start(ctx, "unrelated") }
func Helper(ctx context.Context) { tracer.Start(ctx, "private-span-label-do-not-log") }
func Parent(ctx context.Context) { Helper(ctx) }
func Stored(ctx context.Context) { call := func() { tracer.Start(ctx, "stored") }; _ = call }
func Bound(ctx context.Context) { start := tracer.Start; start(ctx, "bound") }
func Immediate(ctx context.Context) { func() { tracer.Start(ctx, "immediate") }() }
func Expression(ctx context.Context) { trace.Tracer.Start(tracer, ctx, "expression") }

type Wrapper struct { trace.Tracer }
func (w Wrapper) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
 return w.Tracer.Start(ctx, name, opts...)
}
func Wrapped(ctx context.Context) { wrapper := Wrapper{Tracer: tracer}; wrapper.Start(ctx, "wrapped") }
func Promoted(ctx context.Context) { value := outer{Wrapper{Tracer: tracer}}; value.Start(ctx, "promoted") }
type outer struct { Wrapper }
type unrelatedTracer struct{}
func (unrelatedTracer) Start(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
 return ctx, nil
}
`,
	}

	copyManualSpanManifests(t, root)

	for name, contents := range files {
		err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func copyManualSpanManifests(t *testing.T, destination string) {
	t.Helper()

	source, err := os.OpenRoot("../cli/testdata/architectures/common")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = source.Close() }()

	target, err := os.OpenRoot(destination)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = target.Close() }()

	for _, name := range []string{"go.mod", "go.sum"} {
		contents, readErr := source.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}

		if name == "go.mod" {
			module := strings.Replace(string(contents), "module example.com/architecture", "module example.com/manual", 1)
			contents = []byte(module)
		}

		writeErr := target.WriteFile(name, contents, 0o600)
		if writeErr != nil {
			t.Fatal(writeErr)
		}
	}
}
