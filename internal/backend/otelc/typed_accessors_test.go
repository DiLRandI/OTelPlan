package otelc

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestTypedAccessorsCompileAndRun(t *testing.T) {
	root, code, target := accessorFixture(t)
	attrs := []model.AttributePlan{
		{Key: "arg.alias", From: model.AttributeSource{Argument: "req.Code"}, Classification: model.ClassificationPublic},
		{Key: "arg.direct_alias", From: model.AttributeSource{Argument: "code"}, Classification: model.ClassificationPublic},
		{Key: "arg.bool", From: model.AttributeSource{Argument: "req.Enabled"}, Classification: model.ClassificationPublic},
		{Key: "arg.float", From: model.AttributeSource{Argument: "req.Inner.Score"}, Classification: model.ClassificationPublic},
		{Key: "arg.nested", From: model.AttributeSource{Argument: "req.Inner.Child.Value"}, Classification: model.ClassificationPublic},
		{Key: "arg.string", From: model.AttributeSource{Argument: "req.ID"}, Classification: model.ClassificationPublic},
		{Key: "arg.signed", From: model.AttributeSource{Argument: "req.Signed"}, Classification: model.ClassificationPublic},
		{Key: "arg.uint64", From: model.AttributeSource{Argument: "req.Count"}, Classification: model.ClassificationPublic},
		{Key: "constant.zero", From: model.AttributeSource{Constant: math.Copysign(0, -1)}},
		{Key: "constant.bool", From: model.AttributeSource{Constant: false}, Classification: model.ClassificationPublic},
		{Key: "constant.string", From: model.AttributeSource{Constant: "fixed"}, Classification: model.ClassificationPublic},
		{Key: "constant.uint", From: model.AttributeSource{Constant: int64(7)}, Classification: model.ClassificationPublic},
		{Key: "result.message", From: model.AttributeSource{Result: "result.Message"}, Classification: model.ClassificationPublic},
		{Key: "result.nan", From: model.AttributeSource{Result: "result.NaN"}, Classification: model.ClassificationPublic},
		{Key: "result.score", From: model.AttributeSource{Result: "result.Score"}, Classification: model.ClassificationPublic},
		{Key: "result.uint64", From: model.AttributeSource{Result: "result.Big"}, Classification: model.ClassificationPublic},
	}
	target.Attributes = attrs
	original := append([]model.AttributePlan(nil), target.Attributes...)
	source, bindings, err := RenderAccessors(code, target)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(target.Attributes, original) {
		t.Fatal("RenderAccessors mutated target attributes")
	}
	keys := make([]string, len(bindings))
	for i, binding := range bindings {
		keys[i] = binding.Key
		switch binding.Source {
		case "constant":
			if binding.Index != -1 {
				t.Errorf("constant %q has index %d", binding.Key, binding.Index)
			}
		case "argument":
			want := 1
			if binding.Key == "arg.direct_alias" {
				want = 2
			}
			if binding.Index != want {
				t.Errorf("argument %q has index %d, want %d", binding.Key, binding.Index, want)
			}
		case "result":
			if binding.Index != 0 {
				t.Errorf("result %q has index %d, want 0", binding.Key, binding.Index)
			}
		}
	}
	if !sort.StringsAreSorted(keys) {
		t.Fatalf("bindings are not sorted: %v", keys)
	}
	if len(bindings) != len(attrs) {
		t.Fatalf("got %d bindings, want %d", len(bindings), len(attrs))
	}
	reordered := append([]model.AttributePlan(nil), attrs...)
	sort.Slice(reordered, func(i, j int) bool { return reordered[i].Key > reordered[j].Key })
	target.Attributes = reordered
	reorderedSource, reorderedBindings, err := RenderAccessors(code, target)
	if err != nil || !bytes.Equal(source, reorderedSource) || !reflect.DeepEqual(bindings, reorderedBindings) {
		t.Fatalf("reordering attributes changed generated output")
	}
	target.Attributes = attrs

	generated := strings.TrimPrefix(string(source), "//go:build ignore\n\n")
	if generated == string(source) {
		t.Fatal("generated accessor source is missing build-ignore directive")
	}
	opsDir := filepath.Join(root, "ops")
	if err := os.WriteFile(filepath.Join(opsDir, "generated_accessors.go"), []byte(generated), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opsDir, "generated_accessors_test.go"), []byte(accessorRuntimeTest(bindings)), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "go", "test", "./ops")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated accessors failed to compile or run: %v\n%s", err, output)
	}
}

func TestTypedAccessorsRejectInvalidPlans(t *testing.T) {
	_, code, target := accessorFixture(t)
	cases := []struct {
		name  string
		attrs []model.AttributePlan
	}{
		{"non-finite constant", []model.AttributePlan{{Key: "nonfinite", From: model.AttributeSource{Constant: math.NaN()}}}},
		{"blank key", []model.AttributePlan{{From: model.AttributeSource{Argument: "req.ID"}}}},
		{"otel key", []model.AttributePlan{{Key: "otel.trace", From: model.AttributeSource{Argument: "req.ID"}}}},
		{"duplicate key", []model.AttributePlan{{Key: "same", From: model.AttributeSource{Argument: "req.ID"}}, {Key: "same", From: model.AttributeSource{Argument: "req.Enabled"}}}},
		{"unknown field", []model.AttributePlan{{Key: "bad", From: model.AttributeSource{Argument: "req.Missing"}}}},
		{"object source", []model.AttributePlan{{Key: "bad", From: model.AttributeSource{Argument: "req"}}}},
		{"unexported field", []model.AttributePlan{{Key: "bad", From: model.AttributeSource{Argument: "req.secret"}}}},
		{"multiple sources", []model.AttributePlan{{Key: "bad", From: model.AttributeSource{Argument: "req.ID", Result: "result.Message"}}}},
		{"secret without approval", []model.AttributePlan{{Key: "secret", From: model.AttributeSource{Argument: "req.Secret"}, Classification: model.ClassificationSecret}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			target.Attributes = test.attrs
			if source, bindings, err := RenderAccessors(code, target); err == nil || source != nil || bindings != nil {
				t.Fatalf("expected no partial output, got source=%d bindings=%d err=%v", len(source), len(bindings), err)
			}
		})
	}
}

func TestTypedAccessorsEmptyPlan(t *testing.T) {
	_, code, target := accessorFixture(t)
	source, bindings, err := RenderAccessors(code, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 0 || !strings.Contains(string(source), "//go:build ignore") {
		t.Fatalf("empty plan returned source=%d bindings=%d", len(source), len(bindings))
	}
}

func accessorFixture(t *testing.T) (string, *model.CodeModel, model.ResolvedTarget) {
	t.Helper()
	root := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/accessorprobe\n\ngo 1.27\n")
	write("dep/dep.go", "package dep\n\ntype Code string\n")
	write("ops/ops.go", `package ops

import (
    "context"
    "math"
    "example.com/accessorprobe/dep"
)

type InnerChild struct { Value float64 }
type Inner struct { Score float64; Child *InnerChild }
type request struct {
    ID string
    Enabled bool
    Count uint64
    Signed int64
    Code dep.Code
    Secret string
    Inner *Inner
    secret string
}
type output struct { Message string; Big uint64; Score float64; NaN float64 }
func NewRequest() *request { return &request{ID: "", Enabled: false, Count: 9223372036854775808, Code: dep.Code("named"), Secret: "hidden", Inner: &Inner{Score: math.Inf(1), Child: nil}} }
func NewResult() output { return output{Message: "", Big: 9223372036854775808, Score: math.Inf(1), NaN: math.NaN()} }
func Handle(ctx context.Context, req *request, code dep.Code) (result output, err error) { return output{}, nil }
`)
	code, err := discovery.LoadContext(t.Context(), discovery.Options{Root: root, Patterns: []string{"./ops"}, Env: []string{"GOWORK=off", "GOFLAGS="}})
	if err != nil {
		t.Fatal(err)
	}
	symbol, ok := code.Symbol(model.SymbolID("example.com/accessorprobe/ops.Handle"))
	if !ok {
		t.Fatal("fixture symbol missing")
	}
	return root, code, model.ResolvedTarget{SymbolID: symbol.ID, Signature: symbol.Signature, ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0}}
}

func accessorRuntimeTest(bindings []AccessorBinding) string {
	byKey := make(map[string]AccessorBinding, len(bindings))
	for _, binding := range bindings {
		byKey[binding.Key] = binding
	}
	call := func(key, arg string) string {
		binding := byKey[key]
		if binding.Function == "" {
			return fmt.Sprintf("t.Fatalf(\"missing binding %s\")", key)
		}
		return fmt.Sprintf("%s(%s)", binding.Function, arg)
	}
	return fmt.Sprintf(`package ops

import ("testing"; "math")

func TestGeneratedTypedAccessors(t *testing.T) {
    if got, ok := %s; !ok || got != 0 || !math.Signbit(got) { t.Fatalf("negative zero lost: %%v,%%v", got,ok) }
    req := NewRequest()
	result := NewResult()
	code := req.Code
	code = "direct"
	if got, ok := %s; !ok || got != "" { t.Fatalf("string=%%q,%%v", got, ok) }
	if got, ok := %s; ok || got != "" { t.Fatalf("wrong input=%%q,%%v", got, ok) }
	var nilReq *request
	if got, ok := %s; ok || got != "" { t.Fatalf("typed nil=%%q,%%v", got, ok) }
	if got, ok := %s; !ok || got != false { t.Fatalf("bool=%%v,%%v", got, ok) }
	if got, ok := %s; !ok || got != "named" { t.Fatalf("named string=%%q,%%v", got, ok) }
	if got, ok := %s; !ok || got != "direct" { t.Fatalf("direct named string=%%q,%%v", got, ok) }
	if got, ok := %s; ok || got != 0 { t.Fatalf("overflow uint=%%d,%%v", got, ok) }
	if got, ok := %s; ok || got != 0 { t.Fatalf("inf float=%%v,%%v", got, ok) }
	if got, ok := %s; ok || got != 0 { t.Fatalf("nil nested=%%v,%%v", got, ok) }
	if got, ok := %s; !ok || got != "" { t.Fatalf("result string=%%q,%%v", got, ok) }
	if got, ok := %s; ok || got != 0 { t.Fatalf("result overflow=%%d,%%v", got, ok) }
	if got, ok := %s; ok || got != 0 { t.Fatalf("nan=%%v,%%v", got, ok) }
	if got, ok := %s; ok || got != 0 { t.Fatalf("result inf=%%v,%%v", got, ok) }
	if got, ok := %s; !ok || got != false { t.Fatalf("constant bool=%%v,%%v", got, ok) }
	if got, ok := %s; !ok || got != "fixed" { t.Fatalf("constant string=%%q,%%v", got, ok) }
	if got, ok := %s; !ok || got != 7 { t.Fatalf("constant int=%%d,%%v", got, ok) }
	req.Inner = &Inner{Score: 1.5, Child: &InnerChild{Value: 2.5}}
	req.Count = 42
	req.Signed = -7
	result.Big = 42
	if got, ok := %s; !ok || got != 42 { t.Fatalf("uint=%%d,%%v", got, ok) }
	if got, ok := %s; !ok || got != 1.5 { t.Fatalf("float=%%v,%%v", got, ok) }
	if got, ok := %s; !ok || got != 2.5 { t.Fatalf("nested=%%v,%%v", got, ok) }
	if got, ok := %s; !ok || got != -7 { t.Fatalf("signed=%%d,%%v", got, ok) }
	if got, ok := %s; !ok || got != 42 { t.Fatalf("result?=%%d,%%v", got, ok) }
}
`, call("constant.zero", "nil"), call("arg.string", "req"), call("arg.string", "42"), call("arg.string", "nilReq"), call("arg.bool", "req"), call("arg.alias", "req"), call("arg.direct_alias", "code"), call("arg.uint64", "req"), call("arg.float", "req"), call("arg.nested", "req"), call("result.message", "result"), call("result.uint64", "result"), call("result.nan", "result"), call("result.score", "result"), call("constant.bool", "nil"), call("constant.string", "nil"), call("constant.uint", "nil"), call("arg.uint64", "req"), call("arg.float", "req"), call("arg.nested", "req"), call("arg.signed", "req"), call("result.uint64", "result"))
}
