package compiler_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	largeWorkspaceTargets    = 2000
	largeWorkspacePackages   = 100
	largeWorkspaceRules      = 200
	largeWorkspaceOperations = "example.com/scale/operations"
)

type largeWorkspaceFixture struct {
	root   *os.Root
	code   *model.CodeModel
	policy *model.Policy
	plan   model.ResolvedPlan
}

func newLargeWorkspaceFixture(t *testing.T) largeWorkspaceFixture {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := root.Close()
		if err != nil {
			t.Error(err)
		}
	})
	writeLargeWorkspaceModules(t, root)

	for packageIndex := range largeWorkspacePackages {
		name := fmt.Sprintf("operations/pkg%03d/operations.go", packageIndex)
		writeLargeWorkspaceFile(t, root, name, []byte(largeWorkspaceOperationsSource(packageIndex)))
	}

	writeLargeWorkspaceFile(t, root, "app/cmd/probe/main.go", []byte(largeWorkspaceProbeSource()))
	writeLargeWorkspaceFile(t, root, "otelplan.yaml", []byte(largeWorkspacePolicySource()))

	var plan model.ResolvedPlan

	return largeWorkspaceFixture{root: root, code: nil, policy: nil, plan: plan}
}

func writeLargeWorkspaceModules(t *testing.T, root *os.Root) {
	t.Helper()

	template, err := os.OpenRoot("../cli/testdata/architectures/common")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = template.Close() }()

	manifest, err := template.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}

	manifest = bytes.Replace(manifest, []byte("module example.com/architecture"),
		[]byte("module example.com/scale/app"), 1)
	writeLargeWorkspaceFile(t, root, "app/go.mod", manifest)

	checksums, err := template.ReadFile("go.sum")
	if err != nil {
		t.Fatal(err)
	}

	writeLargeWorkspaceFile(t, root, "app/go.sum", checksums)
	writeLargeWorkspaceFile(t, root, "operations/go.mod", []byte("module "+largeWorkspaceOperations+"\n\ngo 1.27\n"))
	writeLargeWorkspaceFile(t, root, "go.work", []byte("go 1.27.0\n\nuse (\n ./app\n ./operations\n)\n"))
}

func writeLargeWorkspaceFile(t *testing.T, root *os.Root, name string, contents []byte) {
	t.Helper()

	err := root.MkdirAll(filepath.Dir(name), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = root.WriteFile(name, contents, 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func largeWorkspaceOperationsSource(packageIndex int) string {
	var source strings.Builder

	fmt.Fprintf(&source, "package pkg%03d\n\nimport \"context\"\n", packageIndex)

	for localIndex := range largeWorkspaceTargets / largeWorkspacePackages {
		index := packageIndex*(largeWorkspaceTargets/largeWorkspacePackages) + localIndex
		fmt.Fprintf(&source, "func Operation%04d(ctx context.Context) error { return ctx.Err() }\n", index)
	}

	first := packageIndex * (largeWorkspaceTargets / largeWorkspacePackages)
	fmt.Fprintf(&source, "func Operation%04dExtra(ctx context.Context) error { return ctx.Err() }\n", first)

	return source.String()
}

func largeWorkspaceSymbol(index int) model.SymbolID {
	packageIndex := index / (largeWorkspaceTargets / largeWorkspacePackages)
	path := fmt.Sprintf("%s/pkg%03d", largeWorkspaceOperations, packageIndex)

	return model.FunctionID(path, fmt.Sprintf("Operation%04d", index))
}

func largeWorkspacePolicySource() string {
	var source strings.Builder

	source.WriteString(`apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
project: {packages: [./app/..., ./operations/...]}
defaults:
  spanName: "{{symbol}}"
  context: {mode: require}
  errors: {record: true}
rules:
`)

	for ruleIndex := range largeWorkspaceRules {
		fmt.Fprintf(&source, "- id: exact-%03d\n  match:\n    symbols:\n", ruleIndex)

		for localIndex := range largeWorkspaceTargets / largeWorkspaceRules {
			index := ruleIndex*(largeWorkspaceTargets/largeWorkspaceRules) + localIndex
			fmt.Fprintf(&source, "    - %s\n", largeWorkspaceSymbol(index))
		}

		if ruleIndex == 0 || ruleIndex == largeWorkspaceRules-1 {
			source.WriteString("  attributes:\n  - key: operation.kind\n    from: {constant: capacity}\n")
		}
	}

	return source.String()
}

func largeWorkspaceProbeSource() string {
	var source strings.Builder

	source.WriteString("package main\n\nimport (\n\"context\"\n\"encoding/json\"\n\"os\"\n")
	source.WriteString("\"go.opentelemetry.io/otel\"\n")
	source.WriteString("sdktrace \"go.opentelemetry.io/otel/sdk/trace\"\n")
	source.WriteString("\"go.opentelemetry.io/otel/sdk/trace/tracetest\"\n")

	for packageIndex := range largeWorkspacePackages {
		fmt.Fprintf(&source, "\"%s/pkg%03d\"\n", largeWorkspaceOperations, packageIndex)
	}

	source.WriteString(")\n\nfunc main() {\n")
	source.WriteString(`exporter := tracetest.NewInMemoryExporter()
provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter), sdktrace.WithSampler(sdktrace.AlwaysSample()))
otel.SetTracerProvider(provider)
ctx, parent := provider.Tracer("scale-parent").Start(context.Background(), "root")
operations := []func(context.Context) error{
`)

	for index := range largeWorkspaceTargets {
		packageIndex := index / (largeWorkspaceTargets / largeWorkspacePackages)
		fmt.Fprintf(&source, "pkg%03d.Operation%04d,\n", packageIndex, index)
	}

	for packageIndex := range largeWorkspacePackages {
		first := packageIndex * (largeWorkspaceTargets / largeWorkspacePackages)
		fmt.Fprintf(&source, "pkg%03d.Operation%04dExtra,\n", packageIndex, first)
	}

	source.WriteString(`}
for _, operation := range operations {
 if err := operation(ctx); err != nil { panic(err) }
}
parent.End()
type span struct {
 Name string
 ID string
 Parent string
 Trace string
 Scope string
 Kind string
 Attributes int
 Events int
 Status string
}
report := struct {
 RootID string
 RootTrace string
 Spans []span
}{RootID: parent.SpanContext().SpanID().String(), RootTrace: parent.SpanContext().TraceID().String()}
for _, item := range exporter.GetSpans() {
 report.Spans = append(report.Spans, span{
 Name: item.Name, ID: item.SpanContext.SpanID().String(), Parent: item.Parent.SpanID().String(),
 Trace: item.SpanContext.TraceID().String(), Scope: item.InstrumentationScope.Name,
 Kind: item.SpanKind.String(), Attributes: len(item.Attributes), Events: len(item.Events),
 Status: item.Status.Code.String(),
 })
}
if err := provider.Shutdown(context.Background()); err != nil { panic(err) }
if err := json.NewEncoder(os.Stdout).Encode(report); err != nil { panic(err) }
}
`)

	return source.String()
}

func loadLargeWorkspaceFixture(t *testing.T, fixture *largeWorkspaceFixture) {
	t.Helper()

	var options discovery.Options

	options.Root = fixture.root.Name()
	options.Patterns = []string{"./app/...", "./operations/..."}
	options.Env = []string{"GOWORK=" + filepath.Join(options.Root, "go.work"), "GOFLAGS=", "CGO_ENABLED=0"}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := policy.Load(filepath.Join(fixture.root.Name(), "otelplan.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	diagnostics := policy.Validate(loaded)
	if diagnostics.HasErrors() {
		t.Fatalf("invalid scale policy: %v", diagnostics)
	}

	result := resolve.Resolve(loaded, code)
	if result.Diagnostics.HasErrors() {
		t.Fatalf("unresolved scale policy: %v", result.Diagnostics)
	}

	fixture.code, fixture.policy, fixture.plan = code, loaded, result.Plan
}
