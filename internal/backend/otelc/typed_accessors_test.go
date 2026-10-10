package otelc

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/parser"
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

const accessorSourceSHA256 = "013b2d884e1c75e3b8ec1bded359d5841f3a5564238c9b572b714e617fd55323"

func TestGenericAccessorTypeParameterDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expression string
		uses       bool
	}{
		{expression: "T", uses: true},
		{expression: "*Request[T]", uses: true},
		{expression: "map[string][]T", uses: true},
		{expression: "pkg.T", uses: false},
		{expression: "struct{ T string; Value int }", uses: false},
		{expression: "struct{ Value T }", uses: true},
		{expression: "func(T int) string", uses: false},
		{expression: "func(value int) T", uses: true},
		{expression: "Request[int]", uses: false},
	}

	for _, test := range tests {
		t.Run(test.expression, func(t *testing.T) {
			t.Parallel()

			expression, err := parser.ParseExpr(test.expression)
			if err != nil {
				t.Fatal(err)
			}

			got := expressionUsesTypeParameters(expression, map[string]bool{"T": true})
			if got != test.uses {
				t.Fatalf("type parameter detection=%t, want %t", got, test.uses)
			}
		})
	}
}

func TestTypedAccessorsCompileAndRun(t *testing.T) {
	t.Parallel()

	rootPath, code, target := accessorFixture(t)
	target.Attributes = accessorAttributes()
	original := append([]model.AttributePlan(nil), target.Attributes...)

	source, bindings, err := RenderAccessors(code, target)
	if err != nil {
		t.Fatal(err)
	}

	assertAccessorInputUnchanged(t, target.Attributes, original)
	assertAccessorBindings(t, bindings, 16)
	assertAccessorSourceIdentity(t, source)
	assertAccessorReorderingStable(t, code, target, source, bindings)

	fixtureRoot, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := fixtureRoot.Close()
		if err != nil {
			t.Errorf("close fixture root: %v", err)
		}
	})

	generated := strings.TrimPrefix(string(source), "//go:build ignore\n\n")
	if generated == string(source) {
		t.Fatal("generated accessor source is missing build-ignore directive")
	}

	writeAccessorFixtureFile(t, fixtureRoot, "ops/generated_accessors.go", []byte(generated))
	writeAccessorFixtureFile(t, fixtureRoot, "ops/generated_accessors_test.go", []byte(accessorRuntimeTest(bindings)))

	cmd := exec.CommandContext(t.Context(), "go", "test", "./ops")
	cmd.Dir = fixtureRoot.Name()

	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated accessors failed to compile or run: %v\n%s", err, output)
	}
}

func accessorAttributes() []model.AttributePlan {
	return []model.AttributePlan{
		argumentAccessorAttribute("arg.alias", "req.Code"),
		argumentAccessorAttribute("arg.direct_alias", "code"),
		argumentAccessorAttribute("arg.bool", "req.Enabled"),
		argumentAccessorAttribute("arg.float", "req.Inner.Score"),
		argumentAccessorAttribute("arg.nested", "req.Inner.Child.Value"),
		argumentAccessorAttribute("arg.string", "req.ID"),
		argumentAccessorAttribute("arg.signed", "req.Signed"),
		argumentAccessorAttribute("arg.uint64", "req.Count"),
		accessorAttributePlan("constant.zero", constantAccessorSource(math.Copysign(0, -1))),
		{
			Key: "constant.bool",
			From: model.AttributeSource{
				Argument: "",
				Result:   "",
				Constant: false,
			},
			Classification: model.ClassificationPublic,
			Allow:          false,
		},
		{
			Key: "constant.string",
			From: model.AttributeSource{
				Argument: "",
				Result:   "",
				Constant: "fixed",
			},
			Classification: model.ClassificationPublic,
			Allow:          false,
		},
		constantAccessorAttribute("constant.uint", int64(7)),
		resultAccessorAttribute("result.message", "result.Message"),
		resultAccessorAttribute("result.nan", "result.NaN"),
		resultAccessorAttribute("result.score", "result.Score"),
		resultAccessorAttribute("result.uint64", "result.Big"),
	}
}

func argumentAccessorAttribute(key, argument string) model.AttributePlan {
	return model.AttributePlan{
		Key: key,
		From: model.AttributeSource{
			Argument: argument,
			Result:   "",
			Constant: nil,
		},
		Classification: model.ClassificationPublic,
		Allow:          false,
	}
}

func constantAccessorAttribute(key string, value any) model.AttributePlan {
	return model.AttributePlan{
		Key: key,
		From: model.AttributeSource{
			Argument: "",
			Result:   "",
			Constant: value,
		},
		Classification: model.ClassificationPublic,
		Allow:          false,
	}
}

func resultAccessorAttribute(key, result string) model.AttributePlan {
	return model.AttributePlan{
		Key: key,
		From: model.AttributeSource{
			Argument: "",
			Result:   result,
			Constant: nil,
		},
		Classification: model.ClassificationPublic,
		Allow:          false,
	}
}

func assertAccessorInputUnchanged(t *testing.T, got, want []model.AttributePlan) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatal("RenderAccessors mutated target attributes")
	}
}

func assertAccessorBindings(t *testing.T, bindings []AccessorBinding, wantCount int) {
	t.Helper()

	keys := make([]string, 0, len(bindings))

	for _, binding := range bindings {
		keys = append(keys, binding.Key)
		wantIndex := expectedAccessorIndex(t, binding)

		if binding.Index != wantIndex {
			t.Errorf("attribute %q has index %d, want %d", binding.Key, binding.Index, wantIndex)
		}
	}

	wantKeys := []string{
		"arg.alias", "arg.bool", "arg.direct_alias", "arg.float", "arg.nested", "arg.signed", "arg.string", "arg.uint64",
		"constant.bool", "constant.string", "constant.uint", "constant.zero",
		"result.message", "result.nan", "result.score", "result.uint64",
	}

	if !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("binding keys=%v, want %v", keys, wantKeys)
	}

	if !sort.StringsAreSorted(keys) {
		t.Errorf("bindings are not sorted: %v", keys)
	}

	if len(bindings) != wantCount {
		t.Fatalf("got %d bindings, want %d", len(bindings), wantCount)
	}
}

func expectedAccessorIndex(t *testing.T, binding AccessorBinding) int {
	t.Helper()

	switch binding.Source {
	case "constant":
		return -1
	case "argument":
		if binding.Key == "arg.direct_alias" {
			return 2
		}

		return 1
	case "result":
		return 0
	default:
		t.Errorf("attribute %q has unexpected source %q", binding.Key, binding.Source)

		return 0
	}
}

func assertAccessorSourceIdentity(t *testing.T, source []byte) {
	t.Helper()

	got := fmt.Sprintf("%x", sha256.Sum256(source))

	if got != accessorSourceSHA256 {
		t.Fatalf("generated accessor source SHA256=%s, want %s", got, accessorSourceSHA256)
	}
}

func assertAccessorReorderingStable(
	t *testing.T,
	code *model.CodeModel,
	target model.ResolvedTarget,
	source []byte,
	bindings []AccessorBinding,
) {
	t.Helper()

	reordered := target
	reordered.Attributes = append([]model.AttributePlan(nil), target.Attributes...)
	sort.Slice(reordered.Attributes, func(i, j int) bool {
		return reordered.Attributes[i].Key > reordered.Attributes[j].Key
	})

	gotSource, gotBindings, err := RenderAccessors(code, reordered)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(source, gotSource) || !reflect.DeepEqual(bindings, gotBindings) {
		t.Fatal("reordering attributes changed generated output")
	}
}

func TestTypedAccessorsRejectInvalidPlans(t *testing.T) {
	t.Parallel()

	_, code, target := accessorFixture(t)
	tests := invalidAccessorCases()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			localTarget := target
			localTarget.Attributes = test.attributes

			source, bindings, err := RenderAccessors(code, localTarget)
			if err == nil || source != nil || bindings != nil {
				t.Fatalf("expected no partial output, got source=%d bindings=%d err=%v", len(source), len(bindings), err)
			}
		})
	}
}

type invalidAccessorCase struct {
	name       string
	attributes []model.AttributePlan
}

func constantAccessorSource(value any) model.AttributeSource {
	var source model.AttributeSource

	source.Constant = value

	return source
}

func invalidAccessorCases() []invalidAccessorCase {
	multipleSources := model.AttributeSource{Argument: "req.ID", Result: "result.Message", Constant: nil}
	secret := argumentAccessorAttribute("secret", "req.Secret")
	secret.Classification = model.ClassificationSecret

	return []invalidAccessorCase{
		{name: "non-finite constant", attributes: []model.AttributePlan{
			accessorAttributePlan("nonfinite", constantAccessorSource(math.NaN())),
		}},
		{name: "blank key", attributes: []model.AttributePlan{
			accessorAttributePlan("", argumentAttributeSource("req.ID")),
		}},
		{name: "reserved otel key", attributes: []model.AttributePlan{
			accessorAttributePlan("otel.trace", argumentAttributeSource("req.ID")),
		}},
		{name: "duplicate key", attributes: []model.AttributePlan{
			accessorAttributePlan("same", argumentAttributeSource("req.ID")),
			accessorAttributePlan("same", argumentAttributeSource("req.Enabled")),
		}},
		{name: "unknown field", attributes: []model.AttributePlan{
			accessorAttributePlan("bad", argumentAttributeSource("req.Missing")),
		}},
		{name: "object source", attributes: []model.AttributePlan{
			accessorAttributePlan("bad", argumentAttributeSource("req")),
		}},
		{name: "unexported field", attributes: []model.AttributePlan{
			accessorAttributePlan("bad", argumentAttributeSource("req.secret")),
		}},
		{name: "multiple sources", attributes: []model.AttributePlan{
			accessorAttributePlan("bad", multipleSources),
		}},
		{name: "secret without approval", attributes: []model.AttributePlan{secret}},
	}
}

func TestTypedAccessorFailuresShareCauses(t *testing.T) {
	t.Parallel()

	t.Run("invalid package identity", func(t *testing.T) {
		t.Parallel()

		_, code, target := accessorFixture(t)
		localCode := *code
		localCode.Symbols = append([]model.Symbol(nil), code.Symbols...)

		symbol, ok := localCode.Symbol(target.SymbolID)
		if !ok {
			t.Fatal("fixture symbol missing")
		}

		symbol.PackageName = "_"

		assertRepeatedAccessorFailureSharesCause(t, &localCode, target)
	})

	t.Run("unsafe attribute plan", func(t *testing.T) {
		t.Parallel()

		_, code, target := accessorFixture(t)
		target.Attributes = []model.AttributePlan{{
			Key:            "secret",
			From:           model.AttributeSource{Argument: "req.Secret", Result: "", Constant: nil},
			Classification: model.ClassificationSecret,
			Allow:          false,
		}}
		assertRepeatedAccessorFailureSharesCause(t, code, target)
	})
}

func assertRepeatedAccessorFailureSharesCause(t *testing.T, code *model.CodeModel, target model.ResolvedTarget) {
	t.Helper()

	firstSource, firstBindings, firstErr := RenderAccessors(code, target)
	if firstErr == nil {
		t.Fatal("first render unexpectedly succeeded")
	}

	if firstSource != nil || firstBindings != nil {
		t.Fatalf("first render returned partial output: source=%d bindings=%d", len(firstSource), len(firstBindings))
	}

	cause := firstErrorCause(firstErr)

	secondSource, secondBindings, secondErr := RenderAccessors(code, target)
	if secondErr == nil {
		t.Fatal("second render unexpectedly succeeded")
	}

	if secondSource != nil || secondBindings != nil {
		t.Fatalf("second render returned partial output: source=%d bindings=%d", len(secondSource), len(secondBindings))
	}

	if secondErr.Error() != firstErr.Error() {
		t.Fatalf("repeated errors differ: first=%q second=%q", firstErr, secondErr)
	}

	if !errors.Is(secondErr, cause) {
		t.Fatalf("second render error %v does not share first cause %v", secondErr, cause)
	}
}

func firstErrorCause(err error) error {
	for errors.Unwrap(err) != nil {
		err = errors.Unwrap(err)
	}

	return err
}

func TestTypedAccessorsEmptyPlan(t *testing.T) {
	t.Parallel()

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
	rootPath := t.TempDir()

	fixtureRoot, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := fixtureRoot.Close()
		if err != nil {
			t.Errorf("close fixture root: %v", err)
		}
	})

	writeAccessorFixtureFile(t, fixtureRoot, "go.mod", []byte("module example.com/accessorprobe\n\ngo 1.27\n"))
	writeAccessorFixtureFile(t, fixtureRoot, "dep/dep.go", []byte("package dep\n\ntype Code string\n"))
	writeAccessorFixtureFile(t, fixtureRoot, "ops/ops.go", []byte(accessorPackageSource))

	var options discovery.Options

	options.Root = rootPath
	options.Patterns = []string{"./ops"}
	options.Env = []string{"GOWORK=off", "GOFLAGS="}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	symbol, ok := code.Symbol(model.SymbolID("example.com/accessorprobe/ops.Handle"))
	if !ok {
		t.Fatal("fixture symbol missing")
	}

	var target model.ResolvedTarget

	target.SymbolID = symbol.ID
	target.Signature = symbol.Signature
	target.ContextStrategy.Strategy = model.ContextStrategyArgument

	return rootPath, code, target
}

const accessorPackageSource = `package ops

import (
	"context"
	"math"
	"example.com/accessorprobe/dep"
)

type InnerChild struct {
	Value float64
}

type Inner struct {
	Score float64
	Child *InnerChild
}

type request struct {
	ID      string
	Enabled bool
	Count   uint64
	Signed  int64
	Code    dep.Code
	Secret  string
	Inner   *Inner
	secret  string
}

type output struct {
	Message string
	Big     uint64
	Score   float64
	NaN     float64
}

func NewRequest() *request {
	return &request{
		ID:      "",
		Enabled: false,
		Count:   9223372036854775808,
		Code:    dep.Code("named"),
		Secret:  "hidden",
		Inner:   &Inner{Score: math.Inf(1), Child: nil},
	}
}

func NewResult() output {
	return output{Message: "", Big: 9223372036854775808, Score: math.Inf(1), NaN: math.NaN()}
}

func Handle(ctx context.Context, req *request, code dep.Code) (result output, err error) {
	return output{}, nil
}
`

func writeAccessorFixtureFile(t *testing.T, root *os.Root, path string, contents []byte) {
	t.Helper()

	err := root.MkdirAll(filepath.Dir(path), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = root.WriteFile(path, contents, 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func accessorRuntimeTest(bindings []AccessorBinding) string {
	functions := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		functions[binding.Key] = binding.Function
	}

	call := func(key, argument string) string {
		function := functions[key]
		if function == "" {
			return fmt.Sprintf("missingAccessor(%q)", key)
		}

		return function + "(" + argument + ")"
	}

	return fmt.Sprintf(`package ops

import (
	"math"
	"testing"
)

func TestGeneratedTypedAccessors(t *testing.T) {
	if got, ok := %s; !ok || got != 0 || !math.Signbit(got) { t.Fatalf("negative zero lost: %%v,%%v", got, ok) }
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
	if got, ok := %s; !ok || got != 42 { t.Fatalf("result=%%d,%%v", got, ok) }
}
`,
		call("constant.zero", "nil"), call("arg.string", "req"), call("arg.string", "42"),
		call("arg.string", "nilReq"), call("arg.bool", "req"), call("arg.alias", "req"),
		call("arg.direct_alias", "code"), call("arg.uint64", "req"), call("arg.float", "req"),
		call("arg.nested", "req"), call("result.message", "result"), call("result.uint64", "result"),
		call("result.nan", "result"), call("result.score", "result"), call("constant.bool", "nil"),
		call("constant.string", "nil"), call("constant.uint", "nil"), call("arg.uint64", "req"),
		call("arg.float", "req"), call("arg.nested", "req"), call("arg.signed", "req"),
		call("result.uint64", "result"))
}
