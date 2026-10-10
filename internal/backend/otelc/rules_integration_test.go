package otelc_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	lifecycleRuntimeVersion = "v0.1.0-test"
	lifecycleScope          = "otelplan.io/business"
	concurrentInvocations   = 16
)

func TestGeneratedRulesWithPinnedBackend(t *testing.T) {
	t.Parallel()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC to run the real backend trace test")
	}

	_, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	root := lifecycleWorkspace(t)
	code := discoverLifecycleFixture(t, root)
	plan := lifecyclePlan(t, code)
	writeLifecycleArtifacts(t, root, code, plan)
	original := lifecycleSources(t, root)
	buildLifecycleProbe(t, root, executable)

	if !reflect.DeepEqual(lifecycleSources(t, root), original) {
		t.Fatal("backend modified fixture Go source")
	}

	for _, mode := range []string{"nested", "method", "root", "nil", "unrecorded", "panic", "noop", "concurrent"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			checkGeneratedLifecycle(t, root.Name(), mode)
		})
	}
}

func lifecycleWorkspace(t *testing.T) *os.Root {
	t.Helper()

	path := t.TempDir()

	err := os.CopyFS(path, os.DirFS("testdata/rules"))
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
			t.Errorf("close lifecycle fixture root: %v", closeErr)
		}
	})

	return root
}

func discoverLifecycleFixture(t *testing.T, root *os.Root) *model.CodeModel {
	t.Helper()

	var options discovery.Options

	options.Root = root.Name()
	options.Patterns = []string{"./ops"}
	options.Env = []string{"GOWORK=off", "GOFLAGS="}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func lifecyclePlan(t *testing.T, code *model.CodeModel) model.ResolvedPlan {
	t.Helper()

	var plan model.ResolvedPlan

	for _, name := range []string{"Outer", "Inner", "(*Worker).Execute", "Root", "Unrecorded", "Crash"} {
		symbol, exists := code.Symbol(model.SymbolID("example.com/probe/ops." + name))
		if !exists {
			t.Fatalf("lifecycle fixture symbol %q missing", name)
		}

		var target model.ResolvedTarget

		target.SymbolID = symbol.ID
		target.Signature = symbol.Signature
		target.RuleID = name
		target.SpanName = strings.ToLower(symbol.Name)
		target.ContextStrategy.Strategy = model.ContextStrategyRoot

		if len(symbol.ContextIndexes) == 1 {
			target.ContextStrategy.Strategy = model.ContextStrategyArgument
			target.ContextStrategy.Index = symbol.ContextIndexes[0]
		}

		if symbol.Name != "Unrecorded" && len(symbol.ErrorIndexes) > 0 {
			target.ErrorStrategy.Record = true
			target.ErrorStrategy.Indexes = symbol.ErrorIndexes
		}

		if symbol.Name == "Root" {
			target.SpanName = "root-operation"
		}

		plan.Targets = append(plan.Targets, target)
	}

	return plan
}

func writeLifecycleArtifacts(t *testing.T, root *os.Root, code *model.CodeModel, plan model.ResolvedPlan) {
	t.Helper()

	data, _, err := otelc.RenderRules(otelc.SupportedVersion, code, plan, "example.com/probe/hooks")
	if err != nil {
		t.Fatal(err)
	}

	err = root.WriteFile("rules.yaml", data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	hooks, err := otelc.RenderHooks(otelc.SupportedVersion, lifecycleRuntimeVersion, code, plan, "example.com/probe/hooks")
	if err != nil {
		t.Fatal(err)
	}

	err = root.WriteFile("hooks/hooks.go", hooks, 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func lifecycleSources(t *testing.T, root *os.Root) map[string][sha256.Size]byte {
	t.Helper()

	sources := map[string][sha256.Size]byte{}

	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk lifecycle fixture source: %w", walkErr)
		}

		// OTelC retains generated debug source in its own build directory.
		if entry.IsDir() && name == ".otelc-build" {
			return fs.SkipDir
		}

		if entry.IsDir() || filepath.Ext(name) != ".go" {
			return nil
		}

		data, readErr := root.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("read lifecycle fixture source: %w", readErr)
		}

		sources[name] = sha256.Sum256(data)

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return sources
}

func buildLifecycleProbe(t *testing.T, root *os.Root, executable string) {
	t.Helper()

	rulesPath := filepath.Join(root.Name(), "rules.yaml")
	build := exec.CommandContext(t.Context(), executable, "--rules", rulesPath, "go", "build", "-race", "-o", "probe", ".")
	build.Dir = root.Name()
	build.Env = append(os.Environ(), "GOTMPDIR="+t.TempDir(), "GOWORK=off", "GOFLAGS=",
		"OTELC_WORK_DIR="+root.Name(), "OTELC_BUILD_FLAGS=", "OTELC_RULES="+rulesPath)

	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("real backend lifecycle build failed: %v\n%s", err, output)
	}
}

type lifecycleSpan struct {
	Name    string `json:"Name"`
	ID      string `json:"ID"`
	Parent  string `json:"Parent"`
	Trace   string `json:"Trace"`
	Scope   string `json:"Scope"`
	Version string `json:"Version"`
	Error   bool   `json:"Error"`
	Events  int    `json:"Events"`
}

type lifecycleResult struct {
	Spans          []lifecycleSpan `json:"Spans"`
	ReturnedError  string          `json:"ReturnedError"`
	RecoveredPanic string          `json:"RecoveredPanic"`
}

func checkGeneratedLifecycle(t *testing.T, root, mode string) {
	t.Helper()

	command := exec.CommandContext(t.Context(), "./probe", mode)
	command.Dir = root

	output, err := command.Output()
	if err != nil {
		t.Fatalf("run lifecycle probe in %q mode: %v", mode, err)
	}

	var result lifecycleResult

	err = json.Unmarshal(output, &result)
	if err != nil {
		t.Fatalf("decode lifecycle probe in %q mode: %v", mode, err)
	}

	checkLifecycleReturns(t, result, mode)

	switch mode {
	case "noop":
		if len(result.Spans) != 0 {
			t.Fatalf("no-op provider exported spans: %s", output)
		}
	case "concurrent":
		checkConcurrentSpans(t, result.Spans)
	default:
		checkLifecycleSpans(t, result.Spans, mode)
	}
}

func checkLifecycleReturns(t *testing.T, result lifecycleResult, mode string) {
	t.Helper()

	wantError := ""

	switch mode {
	case "nested", "method", "nil", "noop", "concurrent":
		wantError = "probe failure"
	case "unrecorded":
		wantError = "unrecorded failure"
	}

	if result.ReturnedError != wantError {
		t.Fatalf("returned error in %q mode = %q, want %q", mode, result.ReturnedError, wantError)
	}

	wantPanic := ""
	if mode == "panic" {
		wantPanic = "application panic"
	}

	if result.RecoveredPanic != wantPanic {
		t.Fatalf("recovered panic in %q mode = %q, want %q", mode, result.RecoveredPanic, wantPanic)
	}
}

func lifecycleSpansByName(t *testing.T, spans []lifecycleSpan) map[string]lifecycleSpan {
	t.Helper()

	byName := map[string]lifecycleSpan{}

	for _, span := range spans {
		if _, exists := byName[span.Name]; exists {
			t.Fatalf("span %q ended more than once", span.Name)
		}

		byName[span.Name] = span

		if span.Scope == lifecycleScope && span.Version != lifecycleRuntimeVersion {
			t.Fatalf("instrumentation version missing: %+v", span)
		}
	}

	return byName
}

func checkLifecycleSpans(t *testing.T, spans []lifecycleSpan, mode string) {
	t.Helper()

	byName := lifecycleSpansByName(t, spans)

	parent, exists := byName["root"]
	if !exists {
		t.Fatalf("missing parent span in %q mode", mode)
	}

	name := map[string]string{
		"nested": "outer", "method": "execute", "nil": "outer", "root": "root-operation",
		"unrecorded": "unrecorded", "panic": "crash",
	}[mode]

	operation, exists := byName[name]
	if !exists || operation.Scope != lifecycleScope {
		t.Fatalf("missing operation span in %q mode", mode)
	}

	checkLifecycleParent(t, parent, operation, mode)

	wantSpans := 2

	switch mode {
	case "nested", "method", "nil":
		wantSpans = 3

		checkNestedLifecycleErrors(t, byName, operation)
	default:
		if operation.Error || operation.Events != 0 {
			t.Fatalf("unexpected error recording in %q mode: %+v", mode, operation)
		}
	}

	if len(spans) != wantSpans {
		t.Fatalf("instrumentation count in %q mode = %d, want %d", mode, len(spans), wantSpans)
	}
}

func checkLifecycleParent(t *testing.T, parent, operation lifecycleSpan, mode string) {
	t.Helper()

	if mode == "nil" || mode == "root" {
		if operation.Parent != "0000000000000000" || operation.Trace == parent.Trace {
			t.Fatalf("expected separate root trace in %q mode: %+v", mode, operation)
		}

		return
	}

	if operation.Parent != parent.ID || operation.Trace != parent.Trace {
		t.Fatalf("parent context changed in %q mode: %+v", mode, operation)
	}
}

func checkNestedLifecycleErrors(t *testing.T, byName map[string]lifecycleSpan, operation lifecycleSpan) {
	t.Helper()

	inner, exists := byName["inner"]
	if !exists || inner.Parent != operation.ID || inner.Trace != operation.Trace || !inner.Error || inner.Events != 1 {
		t.Fatalf("child context/error changed: %+v", inner)
	}

	if !operation.Error || operation.Events != 1 {
		t.Fatalf("operation error not recorded: %+v", operation)
	}
}

func checkConcurrentSpans(t *testing.T, spans []lifecycleSpan) {
	t.Helper()

	if len(spans) != 1+2*concurrentInvocations {
		t.Fatalf("expected one span per invocation, got %d", len(spans))
	}

	byID, root := concurrentSpanIndex(t, spans)
	children, outerCount := concurrentInvocationState(t, spans, byID, root)

	if outerCount != concurrentInvocations || len(children) != concurrentInvocations {
		t.Fatal("invocation state was shared")
	}

	for _, count := range children {
		if count != 1 {
			t.Fatal("invocation has more than one child")
		}
	}
}

func concurrentInvocationState(t *testing.T, spans []lifecycleSpan, byID map[string]lifecycleSpan,
	root lifecycleSpan) (map[string]int, int) {
	t.Helper()

	children := map[string]int{}
	outerCount := 0

	for _, span := range spans {
		if span.Name == "root" {
			continue
		}

		checkConcurrentSpanMetadata(t, span, root)

		switch span.Name {
		case "outer":
			outerCount++

			if span.Parent != root.ID {
				t.Fatal("outer call inherited another invocation")
			}
		case "inner":
			if byID[span.Parent].Name != "outer" {
				t.Fatal("inner call lost its parent")
			}

			children[span.Parent]++
		default:
			t.Fatalf("unexpected span %s", span.Name)
		}
	}

	return children, outerCount
}

func concurrentSpanIndex(t *testing.T, spans []lifecycleSpan) (map[string]lifecycleSpan, lifecycleSpan) {
	t.Helper()

	byID := map[string]lifecycleSpan{}

	var root lifecycleSpan

	for _, span := range spans {
		if _, duplicate := byID[span.ID]; duplicate {
			t.Fatal("duplicate span end")
		}

		byID[span.ID] = span

		if span.Name == "root" {
			root = span
		}
	}

	if root.ID == "" {
		t.Fatal("missing root span")
	}

	return byID, root
}

func checkConcurrentSpanMetadata(t *testing.T, span, root lifecycleSpan) {
	t.Helper()

	if span.Trace != root.Trace || span.Scope != lifecycleScope || span.Version != lifecycleRuntimeVersion ||
		!span.Error || span.Events != 1 {
		t.Fatalf("invalid concurrent span: %+v", span)
	}
}
