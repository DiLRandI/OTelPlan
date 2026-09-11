package otelc

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestGeneratedRulesWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC to run the real backend trace test")
	}
	if _, err := VerifyExecutable(t.Context(), executable, SupportedVersion); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("testdata/rules")); err != nil {
		t.Fatal(err)
	}
	code, err := discovery.LoadContext(t.Context(), discovery.Options{Root: root, Patterns: []string{"./ops"}, Env: []string{"GOWORK=off", "GOFLAGS="}})
	if err != nil {
		t.Fatal(err)
	}
	plan := model.ResolvedPlan{}
	for _, name := range []string{"Outer", "Inner", "(*Worker).Execute", "Root", "Unrecorded", "Crash"} {
		symbol, ok := code.Symbol(model.SymbolID("example.com/probe/ops." + name))
		if !ok {
			t.Fatal("fixture symbol missing")
		}
		target := model.ResolvedTarget{SymbolID: symbol.ID, Signature: symbol.Signature, RuleID: name, SpanName: strings.ToLower(symbol.Name), ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyRoot}}
		if len(symbol.ContextIndexes) == 1 {
			target.ContextStrategy = model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: symbol.ContextIndexes[0]}
		}
		if symbol.Name != "Unrecorded" && len(symbol.ErrorIndexes) > 0 {
			target.ErrorStrategy = model.ErrorStrategy{Record: true, Indexes: symbol.ErrorIndexes}
		}
		if symbol.Name == "Root" {
			target.SpanName = "root-operation"
		}
		plan.Targets = append(plan.Targets, target)
	}
	data, _, err := RenderRules(SupportedVersion, code, plan, "example.com/probe/hooks")
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "rules.yaml")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	hookFile := filepath.Join(root, "hooks", "hooks.go")
	hooks, err := RenderHooks(SupportedVersion, "v0.1.0-test", code, plan, "example.com/probe/hooks")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookFile, hooks, 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(root, "ops", "ops.go"))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "probe")
	build := exec.CommandContext(t.Context(), executable, "--rules", filename, "go", "build", "-race", "-o", binary, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "OTELC_WORK_DIR="+root, "OTELC_BUILD_FLAGS=", "OTELC_RULES="+filename)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("real backend build failed: %v\n%s", err, output)
	}
	after, err := os.ReadFile(filepath.Join(root, "ops", "ops.go"))
	if err != nil || string(after) != string(original) {
		t.Fatal("backend modified fixture source")
	}
	for _, mode := range []string{"nested", "method", "root", "nil", "unrecorded", "panic", "noop", "concurrent"} {
		t.Run(mode, func(t *testing.T) { checkGeneratedLifecycle(t, binary, mode) })
	}
}

type lifecycleSpan struct {
	Name, ID, Parent, Trace, Scope, Version string
	Error                                   bool
	Events                                  int
}

func checkGeneratedLifecycle(t *testing.T, binary, mode string) {
	t.Helper()
	output, err := exec.CommandContext(t.Context(), binary, mode).Output()
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Spans                         []lifecycleSpan
		ReturnedError, RecoveredPanic string
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	wantError := ""
	if mode == "nested" || mode == "method" || mode == "nil" || mode == "noop" || mode == "concurrent" {
		wantError = "probe failure"
	}
	if mode == "unrecorded" {
		wantError = "unrecorded failure"
	}
	if result.ReturnedError != wantError {
		t.Fatalf("returned error changed: %s", output)
	}
	wantPanic := ""
	if mode == "panic" {
		wantPanic = "application panic"
	}
	if result.RecoveredPanic != wantPanic {
		t.Fatalf("application panic changed: %s", output)
	}
	if mode == "noop" {
		if len(result.Spans) != 0 {
			t.Fatalf("no-op provider exported spans: %s", output)
		}
		return
	}
	if mode == "concurrent" {
		checkConcurrentSpans(t, result.Spans)
		return
	}
	byName := map[string]lifecycleSpan{}
	for _, span := range result.Spans {
		if _, exists := byName[span.Name]; exists {
			t.Fatalf("span ended more than once: %s", output)
		}
		byName[span.Name] = span
		if span.Scope == "otelplan.io/business" && span.Version != "v0.1.0-test" {
			t.Fatalf("instrumentation version missing: %s", output)
		}
	}
	parent, ok := byName["root"]
	if !ok {
		t.Fatalf("missing parent span: %s", output)
	}
	name := map[string]string{"nested": "outer", "method": "execute", "nil": "outer", "root": "root-operation", "unrecorded": "unrecorded", "panic": "crash"}[mode]
	operation, ok := byName[name]
	if !ok || operation.Scope != "otelplan.io/business" {
		t.Fatalf("missing operation span: %s", output)
	}
	if mode == "nil" || mode == "root" {
		if operation.Parent != "0000000000000000" || operation.Trace == parent.Trace {
			t.Fatalf("expected separate root trace: %s", output)
		}
	} else if operation.Parent != parent.ID || operation.Trace != parent.Trace {
		t.Fatalf("parent context changed: %s", output)
	}
	wantSpans := 2
	if mode == "nested" || mode == "method" || mode == "nil" {
		wantSpans = 3
		inner, ok := byName["inner"]
		if !ok || inner.Parent != operation.ID || inner.Trace != operation.Trace || !inner.Error || inner.Events != 1 {
			t.Fatalf("child context/error changed: %s", output)
		}
		if !operation.Error || operation.Events != 1 {
			t.Fatalf("error not recorded: %s", output)
		}
	} else if operation.Error || operation.Events != 0 {
		t.Fatalf("unexpected error recording: %s", output)
	}
	if len(result.Spans) != wantSpans {
		t.Fatalf("unexpected instrumentation count: %s", output)
	}
}

func checkConcurrentSpans(t *testing.T, spans []lifecycleSpan) {
	t.Helper()
	if len(spans) != 33 {
		t.Fatalf("expected one span per invocation, got %d", len(spans))
	}
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
	children := map[string]int{}
	outerCount := 0
	for _, span := range spans {
		if span.Name == "root" {
			continue
		}
		if span.Trace != root.Trace || span.Scope != "otelplan.io/business" || span.Version != "v0.1.0-test" || !span.Error || span.Events != 1 {
			t.Fatalf("invalid concurrent span: %+v", span)
		}
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
	if outerCount != 16 || len(children) != 16 {
		t.Fatal("invocation state was shared")
	}
	for _, count := range children {
		if count != 1 {
			t.Fatal("invocation has more than one child")
		}
	}
}
