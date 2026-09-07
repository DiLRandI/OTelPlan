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
	for _, name := range []string{"Outer", "Inner"} {
		symbol, ok := code.Symbol(model.SymbolID("example.com/probe/ops." + name))
		if !ok {
			t.Fatal("fixture symbol missing")
		}
		plan.Targets = append(plan.Targets, model.ResolvedTarget{SymbolID: symbol.ID, Signature: symbol.Signature, RuleID: name, SpanName: strings.ToLower(name), ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0}, ErrorStrategy: model.ErrorStrategy{Record: true, Indexes: []int{0}}})
	}
	data, bindings, err := RenderRules(SupportedVersion, code, plan, "example.com/probe/hooks")
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "rules.yaml")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	hookFile := filepath.Join(root, "hooks", "hooks.go")
	hooks, err := os.ReadFile(hookFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(hooks)
	for _, binding := range bindings {
		symbol, _ := code.Symbol(binding.Symbol)
		text = strings.ReplaceAll(text, "Before"+symbol.Name, binding.Before)
		text = strings.ReplaceAll(text, "After"+symbol.Name, binding.After)
	}
	if err := os.WriteFile(hookFile, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(root, "ops", "ops.go"))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "probe")
	build := exec.CommandContext(t.Context(), executable, "--rules", filename, "go", "build", "-o", binary, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "OTELC_RULES="+filename)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("real backend build failed: %v\n%s", err, output)
	}
	after, err := os.ReadFile(filepath.Join(root, "ops", "ops.go"))
	if err != nil || string(after) != string(original) {
		t.Fatal("backend modified fixture source")
	}
	output, err := exec.CommandContext(t.Context(), binary).Output()
	if err != nil {
		t.Fatal(err)
	}
	var spans []struct {
		Name, ID, Parent, Trace string
		Error                   bool
		Events                  int
	}
	if err := json.Unmarshal(output, &spans); err != nil {
		t.Fatal(err)
	}
	if len(spans) != 3 {
		t.Fatalf("unexpected instrumentation count: %s", output)
	}
	byName := map[string]int{}
	for i, span := range spans {
		byName[span.Name] = i
	}
	for _, name := range []string{"root", "outer", "inner"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing span %s: %s", name, output)
		}
	}
	rootSpan, outer, inner := spans[byName["root"]], spans[byName["outer"]], spans[byName["inner"]]
	if outer.Parent != rootSpan.ID || inner.Parent != outer.ID || inner.Trace != outer.Trace || outer.Trace != rootSpan.Trace || !outer.Error || !inner.Error || outer.Events != 1 || inner.Events != 1 {
		t.Fatalf("incorrect context/error propagation: %s", output)
	}
}
