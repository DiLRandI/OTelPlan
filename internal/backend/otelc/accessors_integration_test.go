package otelc

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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
	code, err := discovery.LoadContext(t.Context(), discovery.Options{Root: root, Patterns: []string{"./ops"}, Env: []string{"GOWORK=off", "GOFLAGS="}})
	if err != nil {
		t.Fatal(err)
	}
	symbol, ok := code.Symbol(model.SymbolID("example.com/probe/ops.Handle"))
	if !ok {
		t.Fatal("fixture symbol missing")
	}
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{{
		SymbolID:        symbol.ID,
		Signature:       symbol.Signature,
		RuleID:          "handle-request",
		SpanName:        "handle-request",
		ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0},
		ErrorStrategy:   model.ErrorStrategy{Record: true, Indexes: []int{0}},
		Attributes:      []model.AttributePlan{{Key: "request.id", From: model.AttributeSource{Argument: "request.ID"}}},
	}}}
	helperSource, accessors, err := RenderAccessors(code, plan.Targets[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(accessors) != 1 || accessors[0].Key != "request.id" {
		t.Fatal("unexpected accessor bindings")
	}
	helperFile := filepath.Join(root, "accessors", "helper.go")
	if err := os.WriteFile(helperFile, helperSource, 0600); err != nil {
		t.Fatal(err)
	}
	original["accessors/helper.go"] = helperSource
	hookData, bindings, err := RenderRules(SupportedVersion, code, plan, "example.com/probe/hooks")
	if err != nil {
		t.Fatal(err)
	}
	rules := append([]byte("accessor_helper:\n  target: example.com/probe/ops\n  do:\n    - add_file:\n        file: helper.go\n        path: example.com/probe/accessors\n"), hookData...)
	if err := os.WriteFile(filepath.Join(root, "rules.yaml"), rules, 0600); err != nil {
		t.Fatal(err)
	}
	hookFile := filepath.Join(root, "hooks", "hooks.go")
	hooks, err := os.ReadFile(hookFile)
	if err != nil {
		t.Fatal(err)
	}
	hookSource := strings.ReplaceAll(string(hooks), "RequestID", accessors[0].Function)
	for _, binding := range bindings {
		symbol, _ := code.Symbol(binding.Symbol)
		hookSource = strings.ReplaceAll(hookSource, "Before"+symbol.Name, binding.Before)
		hookSource = strings.ReplaceAll(hookSource, "After"+symbol.Name, binding.After)
	}
	if err := os.WriteFile(hookFile, []byte(hookSource), 0600); err != nil {
		t.Fatal(err)
	}
	original["hooks/hooks.go"] = []byte(hookSource)
	binary := filepath.Join(root, "probe")
	build := exec.CommandContext(t.Context(), executable, "--rules", filepath.Join(root, "rules.yaml"), "go", "build", "-o", binary, ".")
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
	if len(first.Attributes) == 0 {
		first, second = second, first
	}
	if first.Attributes["request.id"] != "approved-id" || len(first.Attributes) != 1 {
		t.Fatalf("unexpected selected attributes: %#v", first.Attributes)
	}
	if len(second.Attributes) != 0 {
		t.Fatalf("nil request should omit attributes: %#v", second.Attributes)
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
