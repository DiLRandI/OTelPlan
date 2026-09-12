package otelc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPrivateAccessorsWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC to run the real backend private accessor test")
	}
	if _, err := VerifyExecutable(t.Context(), executable, SupportedVersion); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("testdata/accessors")); err != nil {
		t.Fatal(err)
	}
	original := snapshotApplicationFiles(t, root)
	code, err := discovery.LoadContext(t.Context(), discovery.Options{Root: root, Patterns: []string{"./ops", "./unused"}, Env: []string{"GOWORK=off", "GOFLAGS="}})
	if err != nil {
		t.Fatal(err)
	}
	symbol, ok := code.Symbol(model.SymbolID("example.com/probe/ops.(*Worker).Handle"))
	if !ok {
		t.Fatal("fixture symbol missing")
	}
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{{
		SymbolID:        symbol.ID,
		Signature:       symbol.Signature,
		RuleID:          "handle-request",
		SpanName:        "handle-request",
		ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0},
		ErrorStrategy:   model.ErrorStrategy{Record: true, Indexes: []int{1}},
		Attributes: []model.AttributePlan{
			{Key: "request.id", From: model.AttributeSource{Argument: "request.ID"}},
			{Key: "result.size", From: model.AttributeSource{Result: "size"}},
			{Key: "component", From: model.AttributeSource{Constant: "probe"}},
			{Key: "cache.hit", From: model.AttributeSource{Constant: false}},
			{Key: "weight", From: model.AttributeSource{Constant: 1.5}},
		},
	}}}
	unused, ok := code.Symbol("example.com/probe/unused.Process")
	if !ok {
		t.Fatal("unused fixture symbol missing")
	}
	plan.Targets = append(plan.Targets, model.ResolvedTarget{
		SymbolID: unused.ID, Signature: unused.Signature, SpanName: "unused",
		ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0},
		Attributes:      []model.AttributePlan{{Key: "quantity", From: model.AttributeSource{Argument: "1"}}},
	})
	rules, _, err := RenderRules(SupportedVersion, code, plan, "example.com/probe/hooks")
	if err != nil {
		t.Fatal(err)
	}
	for i, target := range plan.Targets {
		helperSource, _, err := RenderAccessors(code, target)
		if err != nil {
			t.Fatal(err)
		}
		filename := fmt.Sprintf("helper%d.go", i)
		if err := os.WriteFile(filepath.Join(root, "accessors", filename), helperSource, 0600); err != nil {
			t.Fatal(err)
		}
		original["accessors/"+filename] = helperSource
		symbol, _ := code.Symbol(target.SymbolID)
		rules = append(rules, []byte(fmt.Sprintf("accessor%d:\n  target: %s\n  do:\n    - add_file:\n        file: %s\n        path: example.com/probe/accessors\n", i, symbol.PackageImportPath, filename))...)
	}
	if err := os.WriteFile(filepath.Join(root, "rules.yaml"), rules, 0600); err != nil {
		t.Fatal(err)
	}
	hookFile := filepath.Join(root, "hooks", "hooks.go")
	hooks, err := RenderHooks(SupportedVersion, "test", code, plan, "example.com/probe/hooks")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookFile, hooks, 0600); err != nil {
		t.Fatal(err)
	}
	original["hooks/hooks.go"] = hooks
	binary := filepath.Join(root, "probe")
	build := exec.CommandContext(t.Context(), executable, "--rules", filepath.Join(root, "rules.yaml"), "go", "build", "-race", "-o", binary, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "OTELC_BUILD_FLAGS=", "OTELC_WORK_DIR="+root, "OTELC_RULES="+filepath.Join(root, "rules.yaml"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("real backend build failed: %v\n%s", err, output)
	}
	assertApplicationFilesUnchanged(t, root, original)
	output, err := exec.CommandContext(t.Context(), binary).Output()
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Errors []string
		Sizes  []int
		Spans  []struct {
			Name, ID, Parent, Trace string
			Error                   bool
			Attributes              map[string]any
			Events                  int
		}
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode probe output: %v\n%s", err, output)
	}
	if !reflect.DeepEqual(result.Errors, []string{"operation failed", "<nil>"}) {
		t.Fatalf("returned errors changed: %#v", result.Errors)
	}
	if !reflect.DeepEqual(result.Sizes, []int{11, 0}) {
		t.Fatalf("returned values changed: %s", output)
	}
	if len(result.Spans) != 5 {
		t.Fatalf("unexpected instrumentation count: %s", output)
	}
	rootIndex := -1
	for i, span := range result.Spans {
		if span.Name == "root" {
			rootIndex = i
			break
		}
	}
	if rootIndex < 0 {
		t.Fatalf("missing root span: %s", output)
	}
	rootSpan := result.Spans[rootIndex]
	var handles, downstream []struct {
		Name, ID, Parent, Trace string
		Error                   bool
		Attributes              map[string]any
		Events                  int
	}
	for _, span := range result.Spans {
		switch span.Name {
		case "handle-request":
			handles = append(handles, span)
		case "downstream":
			downstream = append(downstream, span)
		}
	}
	if len(handles) != 2 || len(downstream) != 2 {
		t.Fatalf("unexpected operation spans: %s", output)
	}
	for _, handle := range handles {
		if handle.Parent != rootSpan.ID || handle.Trace != rootSpan.Trace {
			t.Fatalf("context propagation changed: %s", output)
		}
		children := 0
		for _, child := range downstream {
			if child.Parent == handle.ID && child.Trace == handle.Trace {
				children++
			}
		}
		if children != 1 {
			t.Fatalf("child context was not propagated: %s", output)
		}
	}
	first, second := handles[0], handles[1]
	if _, captured := first.Attributes["request.id"]; !captured {
		first, second = second, first
	}
	if first.Attributes["request.id"] != "approved-id" || first.Attributes["result.size"] != float64(11) || len(first.Attributes) != 5 {
		t.Fatalf("unexpected selected attributes: %#v", first.Attributes)
	}
	if _, captured := second.Attributes["request.id"]; captured || second.Attributes["result.size"] != float64(0) || len(second.Attributes) != 4 {
		t.Fatalf("nil request should omit attributes: %#v", second.Attributes)
	}
	for _, span := range handles {
		if span.Attributes["component"] != "probe" || span.Attributes["cache.hit"] != false || span.Attributes["weight"] != 1.5 {
			t.Fatalf("constant attributes changed: %#v", span.Attributes)
		}
	}
	if !first.Error || first.Events != 1 || second.Error || second.Events != 0 {
		t.Fatalf("returned error semantics changed: %s", output)
	}
}

type fileSnapshot map[string][]byte

func snapshotApplicationFiles(t *testing.T, root string) fileSnapshot {
	t.Helper()
	snapshot := fileSnapshot{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && filepath.Base(path) == ".otelc-build" {
			return filepath.SkipDir
		}
		base := filepath.Base(path)
		if info.IsDir() || base == "otelc.runtime.go" {
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".mod", ".sum":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[rel] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertApplicationFilesUnchanged(t *testing.T, root string, original fileSnapshot) {
	t.Helper()
	current := snapshotApplicationFiles(t, root)
	if !reflect.DeepEqual(current, original) {
		keys := make([]string, 0, len(original))
		for key := range original {
			keys = append(keys, key)
		}
		for _, key := range keys {
			if !bytes.Equal(current[key], original[key]) {
				t.Fatalf("backend modified application file %s", key)
			}
		}
		for key := range current {
			if _, ok := original[key]; !ok {
				t.Fatalf("backend changed application file set; added %s", key)
			}
		}
		t.Fatal("backend changed application file set")
	}
}
