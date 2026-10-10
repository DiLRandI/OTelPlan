package otelc_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPrivateAccessorsWithPinnedBackend(t *testing.T) {
	t.Parallel()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC to run the real backend private accessor test")
	}

	_, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	root := privateAccessorWorkspace(t)
	original := snapshotApplicationFiles(t, root)
	code := discoverPrivateAccessors(t, root)
	plan := privateAccessorPlan(t, code)

	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	files, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	writePrivateAccessorBundle(t, root, files, original)
	buildPrivateAccessorProbe(t, root, executable)
	assertApplicationFilesUnchanged(t, root, original)

	result := runPrivateAccessorProbe(t, root)
	checkPrivateAccessorReturns(t, result)
	handles, downstream, parent := privateAccessorSpans(t, result.Spans)
	checkPrivateAccessorParents(t, handles, downstream, parent)
	checkPrivateCapturedAttributes(t, handles)
}

func privateAccessorWorkspace(t *testing.T) *os.Root {
	t.Helper()

	path := t.TempDir()

	err := os.CopyFS(path, os.DirFS("testdata/accessors"))
	if err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Errorf("close private accessor fixture root: %v", closeErr)
		}
	})

	return root
}

func discoverPrivateAccessors(t *testing.T, root *os.Root) *model.CodeModel {
	t.Helper()

	var options discovery.Options

	options.Root = root.Name()
	options.Patterns = []string{"./ops", "./unused"}
	options.Env = []string{"GOWORK=off", "GOFLAGS="}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func privateAccessorPlan(t *testing.T, code *model.CodeModel) model.ResolvedPlan {
	t.Helper()

	symbol, exists := code.Symbol(model.SymbolID("example.com/probe/ops.(*Worker).Handle"))
	if !exists {
		t.Fatal("fixture symbol missing")
	}

	var target model.ResolvedTarget

	target.SymbolID = symbol.ID
	target.Signature = symbol.Signature
	target.RuleID, target.SpanName = "handle-request", "handle-request"
	target.ContextStrategy.Strategy = model.ContextStrategyArgument
	target.ErrorStrategy.Record, target.ErrorStrategy.Indexes = true, []int{1}
	target.Attributes = privateAccessorAttributes()

	unused, exists := code.Symbol("example.com/probe/unused.Process")
	if !exists {
		t.Fatal("unused fixture symbol missing")
	}

	var unusedTarget model.ResolvedTarget

	unusedTarget.SymbolID = unused.ID
	unusedTarget.Signature = unused.Signature
	unusedTarget.SpanName = "unused"
	unusedTarget.ContextStrategy.Strategy = model.ContextStrategyArgument

	var quantity model.AttributePlan

	quantity.Key, quantity.From.Argument = "quantity", "1"
	unusedTarget.Attributes = []model.AttributePlan{quantity}

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{target, unusedTarget}

	return plan
}

func privateAccessorAttributes() []model.AttributePlan {
	var requestID, size, component, cacheHit, weight model.AttributePlan

	requestID.Key, requestID.From.Argument = "request.id", "request.ID"
	size.Key, size.From.Result = "result.size", "size"
	component.Key, component.From.Constant = "component", "probe"
	cacheHit.Key, cacheHit.From.Constant = "cache.hit", false
	weight.Key, weight.From.Constant = "weight", 1.5

	return []model.AttributePlan{requestID, size, component, cacheHit, weight}
}

func writePrivateAccessorBundle(t *testing.T, root *os.Root, files []otelc.GeneratedFile, original fileSnapshot) {
	t.Helper()

	for _, file := range files {
		name := "generated/" + file.Path

		err := root.MkdirAll(filepath.Dir(filepath.FromSlash(name)), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = root.WriteFile(filepath.FromSlash(name), file.Data, 0o600)
		if err != nil {
			t.Fatal(err)
		}

		original[name] = file.Data
	}

	err := root.WriteFile("go.work", []byte("go 1.27\n\nuse (\n.\n./generated\n)\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func buildPrivateAccessorProbe(t *testing.T, root *os.Root, executable string) {
	t.Helper()

	rules := filepath.Join(root.Name(), "generated", "rules")
	workspace := filepath.Join(root.Name(), "go.work")
	build := exec.CommandContext(t.Context(), executable, "--rules", rules, "go", "build", "-race", "-o", "probe", ".")
	build.Dir = root.Name()
	build.Env = append(os.Environ(), "GOTMPDIR="+t.TempDir(), "GOWORK="+workspace, "GOFLAGS=",
		"OTELC_BUILD_FLAGS=", "OTELC_WORK_DIR="+root.Name(), "OTELC_RULES="+rules)

	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("real backend private accessor build failed: %v\n%s", err, output)
	}
}

type privateAccessorSpan struct {
	Name       string         `json:"Name"`
	ID         string         `json:"ID"`
	Parent     string         `json:"Parent"`
	Trace      string         `json:"Trace"`
	Error      bool           `json:"Error"`
	Attributes map[string]any `json:"Attributes"`
	Events     int            `json:"Events"`
}

type privateAccessorResult struct {
	Errors []string              `json:"Errors"`
	Sizes  []int                 `json:"Sizes"`
	Spans  []privateAccessorSpan `json:"Spans"`
}

func runPrivateAccessorProbe(t *testing.T, root *os.Root) privateAccessorResult {
	t.Helper()

	command := exec.CommandContext(t.Context(), "./probe")
	command.Dir = root.Name()

	output, err := command.Output()
	if err != nil {
		t.Fatalf("run private accessor probe: %v", err)
	}

	var result privateAccessorResult

	err = json.Unmarshal(output, &result)
	if err != nil {
		t.Fatalf("decode private accessor probe output: %v\n%s", err, output)
	}

	return result
}

func checkPrivateAccessorReturns(t *testing.T, result privateAccessorResult) {
	t.Helper()

	if !reflect.DeepEqual(result.Errors, []string{"operation failed", "<nil>"}) {
		t.Fatalf("returned errors changed: %#v", result.Errors)
	}

	if !reflect.DeepEqual(result.Sizes, []int{11, 0}) {
		t.Fatalf("returned values changed: %#v", result.Sizes)
	}
}

func privateAccessorSpans(t *testing.T, spans []privateAccessorSpan) ([]privateAccessorSpan,
	[]privateAccessorSpan, privateAccessorSpan) {
	t.Helper()

	if len(spans) != 5 {
		t.Fatalf("unexpected instrumentation count: %d", len(spans))
	}

	var handles, downstream []privateAccessorSpan

	var parent privateAccessorSpan

	for _, span := range spans {
		switch span.Name {
		case "root":
			parent = span
		case "handle-request":
			handles = append(handles, span)
		case "downstream":
			downstream = append(downstream, span)
		}
	}

	if parent.ID == "" {
		t.Fatal("missing root span")
	}

	if len(handles) != 2 || len(downstream) != 2 {
		t.Fatalf("unexpected operation spans: handles=%d downstream=%d", len(handles), len(downstream))
	}

	return handles, downstream, parent
}

func checkPrivateAccessorParents(t *testing.T, handles, downstream []privateAccessorSpan, parent privateAccessorSpan) {
	t.Helper()

	for _, handle := range handles {
		if handle.Parent != parent.ID || handle.Trace != parent.Trace {
			t.Fatalf("context propagation changed: %+v", handle)
		}

		children := 0

		for _, child := range downstream {
			if child.Parent == handle.ID && child.Trace == handle.Trace {
				children++
			}
		}

		if children != 1 {
			t.Fatalf("child context was not propagated: %+v", handle)
		}
	}
}

func checkPrivateCapturedAttributes(t *testing.T, handles []privateAccessorSpan) {
	t.Helper()

	first, second := handles[0], handles[1]
	if _, captured := first.Attributes["request.id"]; !captured {
		first, second = second, first
	}

	checkPrivateNullableAttributes(t, first, second)

	checkPrivateConstantAttributes(t, handles)

	if !first.Error || first.Events != 1 || second.Error || second.Events != 0 {
		t.Fatalf("returned error semantics changed: first=%+v second=%+v", first, second)
	}
}

func checkPrivateNullableAttributes(t *testing.T, first, second privateAccessorSpan) {
	t.Helper()

	if first.Attributes["request.id"] != "approved-id" || first.Attributes["result.size"] != float64(11) ||
		len(first.Attributes) != 5 {
		t.Fatalf("unexpected selected attributes: %#v", first.Attributes)
	}

	if _, captured := second.Attributes["request.id"]; captured || second.Attributes["result.size"] != float64(0) ||
		len(second.Attributes) != 4 {
		t.Fatalf("nil request should omit attributes: %#v", second.Attributes)
	}
}

func checkPrivateConstantAttributes(t *testing.T, handles []privateAccessorSpan) {
	t.Helper()

	for _, span := range handles {
		if span.Attributes["component"] != "probe" || span.Attributes["cache.hit"] != false ||
			span.Attributes["weight"] != 1.5 {
			t.Fatalf("constant attributes changed: %#v", span.Attributes)
		}
	}
}

type fileSnapshot map[string][]byte

func snapshotApplicationFiles(t *testing.T, root *os.Root) fileSnapshot {
	t.Helper()

	snapshot := fileSnapshot{}

	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk private accessor fixture: %w", walkErr)
		}

		if entry.IsDir() && filepath.Base(name) == ".otelc-build" {
			return fs.SkipDir
		}

		if entry.IsDir() || filepath.Base(name) == "otelc.runtime.go" || !isApplicationSnapshotFile(name) {
			return nil
		}

		data, readErr := root.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("read private accessor fixture: %w", readErr)
		}

		snapshot[name] = data

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return snapshot
}

func isApplicationSnapshotFile(name string) bool {
	switch filepath.Ext(name) {
	case ".go", ".mod", ".sum", ".json", ".yaml":
		return true
	default:
		return false
	}
}

func assertApplicationFilesUnchanged(t *testing.T, root *os.Root, original fileSnapshot) {
	t.Helper()

	current := snapshotApplicationFiles(t, root)
	if reflect.DeepEqual(current, original) {
		return
	}

	keys := make([]string, 0, len(original))
	for key := range original {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		data, exists := current[key]
		if !exists {
			t.Fatalf("backend removed application file %s", key)
		}

		if !bytes.Equal(data, original[key]) {
			t.Fatalf("backend modified application file %s", key)
		}
	}

	added := make([]string, 0, len(current))

	for key := range current {
		if _, exists := original[key]; !exists {
			added = append(added, key)
		}
	}

	sort.Strings(added)
	t.Fatalf("backend changed application file set; added %v", added)
}
