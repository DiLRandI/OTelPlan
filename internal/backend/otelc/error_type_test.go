package otelc_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestErrorTypeAccessorsDoNotInvokeApplicationMethods(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	for name, contents := range map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.27\n",
		"app.go": errorTypeFixtureSource,
	} {
		writeErr := root.WriteFile(name, []byte(contents), 0o600)
		if writeErr != nil {
			t.Fatal(writeErr)
		}
	}

	var options discovery.Options

	options.Root, options.Offline = root.Name(), true

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	symbol, ok := code.Symbol("example.com/app.Run")
	if !ok {
		t.Fatal("error fixture declaration missing")
	}

	var target model.ResolvedTarget

	target.SymbolID, target.Signature = symbol.ID, symbol.Signature
	target.ContextStrategy.Strategy = model.ContextStrategyRoot

	for _, selector := range []string{"failure", "pointer", "value", "slice", "mapping", "callback", "channel"} {
		var attribute model.AttributePlan

		attribute.Key, attribute.From.ErrorType = selector, selector
		target.Attributes = append(target.Attributes, attribute)
	}

	source, bindings, err := otelc.RenderAccessors(code, target)
	if err != nil {
		t.Fatal(err)
	}

	compileErrorTypeAccessors(t, root, source, bindings)
}

func compileErrorTypeAccessors(t *testing.T, root *os.Root, source []byte, bindings []otelc.AccessorBinding) {
	t.Helper()

	var assertions bytes.Buffer
	assertions.WriteString("package app\nimport \"testing\"\nfunc TestClasses(t *testing.T) {\n")

	for _, binding := range bindings {
		if binding.Source != "result" || binding.Kind != "string" {
			t.Fatalf("error classifier binding lost result provenance: %+v", binding)
		}

		switch binding.Key {
		case "failure":
			fmt.Fprintf(&assertions, `
if got, ok := %s(nil); ok || got != "" { t.Fatal("nil error captured", got, ok) }
if got, ok := %s(42); ok || got != "" { t.Fatal("nonerror captured", got, ok) }
for _, err := range []error{&pointerFailure{}, (*pointerFailure)(nil)} {
 if got, ok := %s(err); !ok || got != "example.com/app.pointerFailure" { t.Fatal(got, ok) }
}
if got, ok := %s(genericFailure[struct{ Private string `+"`secret:\"private-value\"`"+` }]{});
 !ok || got != "example.com/app.genericFailure" { t.Fatal(got, ok) }
if got, ok := %s(struct{ valueFailure }{}); !ok || got != "unnamed" { t.Fatal(got, ok) }
if got, ok := %s(wrappedFailure{error: &pointerFailure{}});
 !ok || got != "example.com/app.wrappedFailure" { t.Fatal(got, ok) }
`, binding.Function, binding.Function, binding.Function, binding.Function, binding.Function, binding.Function)
		case "value":
			fmt.Fprintf(&assertions, "if got, ok := %s(valueFailure{}); !ok || got != %q { t.Fatal(got, ok) }\n",
				binding.Function, "example.com/app.valueFailure")
		default:
			typeName := map[string]string{"pointer": "*pointerFailure", "slice": "sliceFailure", "mapping": "mapFailure",
				"callback": "funcFailure", "channel": "chanFailure"}[binding.Key]
			className := strings.TrimPrefix(typeName, "*")

			fmt.Fprintf(&assertions, "if got, ok := %s(nil); ok || got != \"\" { t.Fatal(got, ok) }\n",
				binding.Function)
			fmt.Fprintf(&assertions, "if got, ok := %s((%s)(nil)); !ok || got != %q { t.Fatal(got, ok) }\n",
				binding.Function, typeName, "example.com/app."+className)
		}
	}

	assertions.WriteString("}\n")

	for name, contents := range map[string][]byte{
		"accessors.go":      []byte(strings.TrimPrefix(string(source), "//go:build ignore\n\n")),
		"accessors_test.go": assertions.Bytes(),
	} {
		err := root.WriteFile(name, contents, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	command := exec.CommandContext(t.Context(), "go", "test", "./...")
	command.Dir = root.Name()

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off",
		"GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated error classifiers failed: %v\n%s", err, output)
	}
}

const errorTypeFixtureSource = `package app

type pointerFailure struct{}
func (*pointerFailure) Error() string { panic("Error must not run") }
func (*pointerFailure) String() string { panic("String must not run") }
func (*pointerFailure) Unwrap() error { panic("Unwrap must not run") }
type wrappedFailure struct{ error }
func (wrappedFailure) Error() string { panic("Error must not run") }
func (wrappedFailure) Unwrap() error { panic("Unwrap must not run") }
type valueFailure struct{}
func (valueFailure) Error() string { panic("Error must not run") }
type genericFailure[T any] struct{}
func (genericFailure[T]) Error() string { panic("Error must not run") }
type sliceFailure []int
func (sliceFailure) Error() string { panic("Error must not run") }
type mapFailure map[string]int
func (mapFailure) Error() string { panic("Error must not run") }
type funcFailure func()
func (funcFailure) Error() string { panic("Error must not run") }
type chanFailure chan int
func (chanFailure) Error() string { panic("Error must not run") }
func Run() (failure error, pointer error, value error, slice error,
 mapping error, callback error, channel error) {
 return (*pointerFailure)(nil), &pointerFailure{}, valueFailure{}, sliceFailure(nil),
 mapFailure(nil), funcFailure(nil), chanFailure(nil)
}
`

func TestGenericErrorTypeCaptureWithoutErrorRecording(t *testing.T) {
	t.Parallel()

	code, plan := genericHookFixture()

	var typ model.TypeInfo

	typ.Type, typ.Expression, typ.Kind = "error", "error", "interface"
	code.Types = append(code.Types, typ)

	var attribute model.AttributePlan

	attribute.Key, attribute.From.ErrorType = "failure.class", "err"
	plan.Targets[0].Attributes = []model.AttributePlan{attribute}
	plan.Targets[0].ErrorStrategy.Record, plan.Targets[0].ErrorStrategy.Indexes = false, nil

	source, err := otelc.RenderHooks(otelc.SupportedVersion, "runtime", code, plan, hookImportPath)
	if err != nil {
		t.Fatal(err)
	}

	_, bindings, err := otelc.RenderAccessors(code, plan.Targets[0])
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(source), "returned_1 any") ||
		!strings.Contains(string(source), "read_"+bindings[0].Function+"(returned_1)") ||
		!strings.Contains(string(source), "if span.IsRecording()") {
		t.Fatalf("generic result classification lost direct guarded binding: %s", source)
	}

	for _, forbidden := range []string{"GetReturnVal(", "span.RecordError(", "SetReturnVal("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("classification changed error behavior: %s", source)
		}
	}

	parseGeneratedHooks(t, source)
}

func TestErrorTypeWithPinnedBackendWithoutErrorEvents(t *testing.T) {
	t.Parallel()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC to test error classification through the real backend")
	}

	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	root := privateAccessorWorkspace(t)
	original := snapshotApplicationFiles(t, root)
	code := discoverPrivateAccessors(t, root)
	plan := privateAccessorPlan(t, code)

	var attribute model.AttributePlan

	attribute.Key, attribute.From.ErrorType = "failure.class", "err"
	plan.Targets[0].Attributes = []model.AttributePlan{attribute}
	plan.Targets[0].ErrorStrategy.Record, plan.Targets[0].ErrorStrategy.Indexes = false, nil

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
	checkErrorTypeOnlySpans(t, handles)
}

func checkErrorTypeOnlySpans(t *testing.T, handles []privateAccessorSpan) {
	t.Helper()

	classified := 0

	for _, span := range handles {
		if span.Error || span.Events != 0 {
			t.Fatal("classification enabled error events or status")
		}

		if len(span.Attributes) == 0 {
			continue
		}

		if len(span.Attributes) != 1 || span.Attributes["failure.class"] != "errors.errorString" {
			t.Fatalf("unexpected classification attributes: %#v", span.Attributes)
		}

		classified++
	}

	if classified != 1 {
		t.Fatalf("classified %d results; want only the non-nil error", classified)
	}
}

func TestErrorTypeAccessorsRespectOwnPackageImports(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"reflect", "strings"} {
		var symbol model.Symbol

		symbol.ID, symbol.PackageImportPath, symbol.PackageName = model.SymbolID(name+".Read"), name, name
		symbol.Name, symbol.Signature, symbol.HasBody = "Read", "func() error", true
		symbol.ErrorIndexes = []int{0}

		var result model.Result

		result.Type = "error"
		symbol.Results = []model.Result{result}

		var typ model.TypeInfo

		typ.Type, typ.Expression, typ.Kind = "error", "error", "interface"

		var code model.CodeModel

		code.Symbols, code.Types = []model.Symbol{symbol}, []model.TypeInfo{typ}

		var target model.ResolvedTarget

		target.SymbolID, target.Signature = symbol.ID, symbol.Signature
		target.ContextStrategy.Strategy = model.ContextStrategyRoot

		var attribute model.AttributePlan

		attribute.Key, attribute.From.ErrorType = "failure.class", "0"
		target.Attributes = []model.AttributePlan{attribute}

		source, _, err := otelc.RenderAccessors(&code, target)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(string(source), "otelplan"+name+".") {
			t.Fatalf("classifier uses an unavailable self-import alias in %s", name)
		}
	}
}
