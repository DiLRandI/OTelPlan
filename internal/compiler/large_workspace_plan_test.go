package compiler_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestLargeWorkspacePlan(t *testing.T) {
	t.Parallel()

	fixture := newLargeWorkspaceFixture(t)
	original := testenv.SnapshotFiles(t, fixture.root.Name())
	loadLargeWorkspaceFixture(t, &fixture)
	assertLargeWorkspaceResolution(t, fixture)
	assertLargeWorkspaceBundle(t, fixture)
	testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, fixture.root.Name()))
}

func assertLargeWorkspaceResolution(t *testing.T, fixture largeWorkspaceFixture) {
	t.Helper()

	if !fixture.code.EffectiveBuild.Workspace || len(fixture.policy.Rules) != largeWorkspaceRules {
		t.Fatal("large plan did not analyze workspace policy")
	}

	assertLargeWorkspaceModules(t, fixture.code)

	packages := 0

	for _, pkg := range fixture.code.Packages {
		if strings.HasPrefix(pkg.ImportPath, largeWorkspaceOperations+"/pkg") {
			packages++
		}
	}

	if packages != largeWorkspacePackages {
		t.Fatalf("operation packages=%d; want %d", packages, largeWorkspacePackages)
	}

	assertLargeWorkspaceTargets(t, fixture.plan)
	assertLargeWorkspaceSafety(t, fixture)
}

func assertLargeWorkspaceModules(t *testing.T, code *model.CodeModel) {
	t.Helper()

	mainModules := map[string]bool{}

	for _, module := range code.Modules {
		if module.Main {
			mainModules[module.Path] = true
		}
	}

	if len(mainModules) != 2 || !mainModules[largeWorkspaceOperations] || !mainModules["example.com/scale/app"] {
		t.Fatalf("unexpected workspace modules: %v", mainModules)
	}
}

func assertLargeWorkspaceTargets(t *testing.T, plan model.ResolvedPlan) {
	t.Helper()

	if len(plan.Targets) != largeWorkspaceTargets {
		t.Fatalf("resolved targets=%d; want %d", len(plan.Targets), largeWorkspaceTargets)
	}

	seen := make(map[model.SymbolID]bool, largeWorkspaceTargets)
	for _, target := range plan.Targets {
		if seen[target.SymbolID] || target.SpanName != string(target.SymbolID) {
			t.Fatalf("duplicate or incorrectly named target %s", target.SymbolID)
		}

		seen[target.SymbolID] = true
	}

	for index := range largeWorkspaceTargets {
		if !seen[largeWorkspaceSymbol(index)] {
			t.Fatalf("missing exact target %s", largeWorkspaceSymbol(index))
		}
	}
}

func assertLargeWorkspaceSafety(t *testing.T, fixture largeWorkspaceFixture) {
	t.Helper()

	var options validate.Options

	diagnostics := validate.Safety(fixture.code, fixture.plan, options)
	if diagnostics.HasErrors() {
		t.Fatalf("scale plan failed safety: %v", diagnostics)
	}

	broadWarning := false

	for _, diagnostic := range diagnostics {
		if diagnostic.Code == model.CodeBroadPlan && diagnostic.Severity == model.SeverityWarning {
			broadWarning = true
		}
	}

	if !broadWarning {
		t.Fatal("large exact plan did not report span-volume warnings")
	}

	diagnostics = otelc.Check(otelc.SupportedVersion, fixture.code, fixture.plan)
	if diagnostics.HasErrors() {
		t.Fatalf("scale plan failed backend capability checks: %v", diagnostics)
	}
}

func assertLargeWorkspaceBundle(t *testing.T, fixture largeWorkspaceFixture) {
	t.Helper()

	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	first, err := otelc.RenderBundle(backend, "scale-test", fixture.code, fixture.plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	second, err := otelc.RenderBundle(backend, "scale-test", fixture.code, fixture.plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	if len(first) != len(second) || len(first) == 0 {
		t.Fatal("large bundle file inventory changed")
	}

	for index, file := range first {
		if file.Path != second[index].Path || !bytes.Equal(file.Data, second[index].Data) {
			t.Fatalf("large bundle output changed for %s", file.Path)
		}
	}

	assertLargeWorkspaceBindings(t, fixture)
}

func assertLargeWorkspaceBindings(t *testing.T, fixture largeWorkspaceFixture) {
	t.Helper()

	_, bindings, err := otelc.RenderRules(otelc.SupportedVersion, fixture.code,
		fixture.plan, "example.com/generated/hooks")
	if err != nil {
		t.Fatal(err)
	}

	if len(bindings) != largeWorkspaceTargets {
		t.Fatalf("backend bindings=%d; want %d", len(bindings), largeWorkspaceTargets)
	}

	seen := make(map[model.SymbolID]bool, len(bindings))
	for _, binding := range bindings {
		if seen[binding.Symbol] {
			t.Fatalf("duplicate backend binding %s", binding.Symbol)
		}

		seen[binding.Symbol] = true
	}

	for index := range largeWorkspaceTargets {
		if !seen[largeWorkspaceSymbol(index)] {
			t.Fatalf("missing backend binding %s", largeWorkspaceSymbol(index))
		}
	}
}

func TestLargeWorkspacePlanWithBackend(t *testing.T) {
	t.Parallel()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is not set")
	}

	fixture := newLargeWorkspaceFixture(t)
	original := testenv.SnapshotFiles(t, fixture.root.Name())
	loadLargeWorkspaceFixture(t, &fixture)
	assertLargeWorkspaceResolution(t, fixture)

	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	var request compiler.ResolvedBuildRequest

	request.Code, request.Plan, request.Backend = fixture.code, fixture.plan, backend
	request.Executable, request.RuntimeVersion = executable, "scale-test"
	request.WorkingDir, request.Parent = filepath.Join(fixture.root.Name(), "app"), t.TempDir()
	request.Env = os.Environ()
	request.GoArgs, request.Packages = []string{"./cmd/probe"}, []string{"./cmd/probe"}
	request.DefaultOutput = true

	built, err := compiler.BuildResolved(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(built.Dir) })

	if len(built.Files) != 1 {
		t.Fatalf("large plan produced %d executables", len(built.Files))
	}

	assertLargeWorkspaceRuntime(t, built.Files[0], fixture.plan)
	testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, fixture.root.Name()))
}

type largeWorkspaceSpan struct {
	Name       string `json:"Name"`
	ID         string `json:"ID"`
	Parent     string `json:"Parent"`
	Trace      string `json:"Trace"`
	Scope      string `json:"Scope"`
	Kind       string `json:"Kind"`
	Attributes int    `json:"Attributes"`
	Events     int    `json:"Events"`
	Status     string `json:"Status"`
}

type largeWorkspaceOutput struct {
	RootID    string               `json:"RootID"`
	RootTrace string               `json:"RootTrace"`
	Spans     []largeWorkspaceSpan `json:"Spans"`
}

func assertLargeWorkspaceRuntime(t *testing.T, artifact compiler.BuildArtifact, plan model.ResolvedPlan) {
	t.Helper()

	directory := t.TempDir()

	err := compiler.PublishBuildArtifact(artifact, filepath.Join(directory, "probe"))
	if err != nil {
		t.Fatal(err)
	}

	command := exec.CommandContext(t.Context(), "./probe")
	command.Dir = directory

	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}

	var report largeWorkspaceOutput

	err = json.Unmarshal(output, &report)
	if err != nil {
		t.Fatal(err)
	}

	assertLargeWorkspaceReport(t, report, plan)
}

func assertLargeWorkspaceReport(t *testing.T, report largeWorkspaceOutput, plan model.ResolvedPlan) {
	t.Helper()

	if len(report.Spans) != largeWorkspaceTargets+1 {
		t.Fatalf("emitted spans=%d; want %d", len(report.Spans), largeWorkspaceTargets+1)
	}

	want := make(map[string]int, len(plan.Targets))
	for _, target := range plan.Targets {
		want[target.SpanName] = len(target.Attributes)
	}

	seen := make(map[string]bool, len(want))

	for _, span := range report.Spans {
		if span.Name == "root" && span.ID == report.RootID {
			continue
		}

		attributes, selected := want[span.Name]
		if !selected || seen[span.Name] {
			t.Fatalf("unexpected or duplicate business span %s", span.Name)
		}

		seen[span.Name] = true
		assertLargeWorkspaceSpan(t, report, span, attributes)
	}

	if len(seen) != len(want) {
		t.Fatalf("instrumented operations=%d; want %d", len(seen), len(want))
	}
}

func assertLargeWorkspaceSpan(t *testing.T, report largeWorkspaceOutput, span largeWorkspaceSpan, attributes int) {
	t.Helper()

	if span.Parent != report.RootID || span.Trace != report.RootTrace || span.ID == report.RootID {
		t.Fatalf("incorrect parent/trace relationship for %s", span.Name)
	}

	if span.Scope != "otelplan.io/business" || span.Kind != "internal" {
		t.Fatalf("incorrect instrumentation scope/kind for %s", span.Name)
	}

	if span.Attributes != attributes || span.Events != 0 || span.Status != "Unset" {
		t.Fatalf("unexpected capture/error data for %s", span.Name)
	}
}
