package compiler_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestVendorBuildWithPinnedBackend(t *testing.T) {
	t.Parallel()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is not set")
	}

	root := newVendoredBackendFixture(t)
	prepareVendoredFixture(t, root.Name())
	original := pinnedWorkspaceInventory(t, root)

	code := analyzeVendoredBackendFixture(t, root.Name())
	plan := vendoredBackendPlan(t, code)

	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	prepareVendoredRuntime(t, backend, code, plan)

	built := buildVendoredBackendFixture(t, root.Name(), backend, code, plan, executable)
	if len(built.Files) != 1 || built.Files[0].DefaultName != "probe" {
		t.Fatalf("unexpected vendor build artifacts: %+v", built.Files)
	}

	runVendoredBackendArtifact(t, built.Files[0])

	err = compiler.VerifyArtifacts(original)
	if err != nil {
		t.Fatalf("vendor build changed the source checkout: %v", err)
	}
}

func newVendoredBackendFixture(t *testing.T) *os.Root {
	t.Helper()

	fixture, err := filepath.Abs("../backend/otelc/testdata/accessors")
	if err != nil {
		t.Fatal(err)
	}

	source, err := compiler.CopySourceTree(t.Context(), fixture, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(source)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	err = root.MkdirAll(filepath.Join("cmd", "probe"), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = root.Rename("main.go", filepath.Join("cmd", "probe", "main.go"))
	if err != nil {
		t.Fatal(err)
	}

	return root
}

func prepareVendoredFixture(t *testing.T, source string) {
	t.Helper()

	downloadVendoredDependencies(t, source)

	command := exec.CommandContext(t.Context(), "go", "mod", "vendor")
	command.Dir = source

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare fixture vendor tree: %v\n%s", err, output)
	}
}

func downloadVendoredDependencies(t *testing.T, directory string) {
	t.Helper()

	command := exec.CommandContext(t.Context(), "go", "mod", "download", "all")
	command.Dir = directory

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare vendor dependency cache: %v\n%s", err, output)
	}
}

func analyzeVendoredBackendFixture(t *testing.T, source string) *model.CodeModel {
	t.Helper()

	var options discovery.Options

	options.Root, options.Patterns = source, []string{"./ops"}
	options.Env = []string{"GOWORK=off", "GOFLAGS=-mod=vendor", "GOPROXY=off"}
	options.Offline = true

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	if code.EffectiveBuild.ModuleMode != "vendor" {
		t.Fatal("analysis did not retain vendor mode")
	}

	return code
}

func vendoredBackendPlan(t *testing.T, code *model.CodeModel) model.ResolvedPlan {
	t.Helper()

	symbol, ok := code.Symbol("example.com/probe/ops.(*Worker).Handle")
	if !ok {
		t.Fatal("fixture target missing")
	}

	var target model.ResolvedTarget

	target.SymbolID, target.Signature, target.SpanName = symbol.ID, symbol.Signature, "vendor-operation"
	target.ContextStrategy = model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0}
	target.ErrorStrategy = model.ErrorStrategy{Record: true, Indexes: []int{1}}

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{target}

	return plan
}

func prepareVendoredRuntime(t *testing.T, backend model.LockBackend, code *model.CodeModel, plan model.ResolvedPlan) {
	t.Helper()

	files, err := otelc.RenderBundle(backend, "test", code, plan, "otelplan.local/generated")
	if err != nil {
		t.Fatal(err)
	}

	runtime, err := compiler.StageArtifacts(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}

	downloadVendoredDependencies(t, runtime.Dir)
}

func buildVendoredBackendFixture(t *testing.T, source string, backend model.LockBackend,
	code *model.CodeModel, plan model.ResolvedPlan, executable string) compiler.BuildResult {
	t.Helper()

	var request compiler.ResolvedBuildRequest

	buildDir := filepath.Join(source, "cmd", "probe")
	request.Code, request.Plan, request.Backend = code, plan, backend
	request.Executable, request.RuntimeVersion = executable, "test"
	request.WorkingDir, request.Parent = buildDir, t.TempDir()
	request.Env, request.GoArgs, request.Packages = os.Environ(), []string{buildDir}, []string{buildDir}
	request.DefaultOutput, request.Offline = true, true

	built, err := compiler.BuildResolved(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(built.Dir) })

	return built
}

func runVendoredBackendArtifact(t *testing.T, artifact compiler.BuildArtifact) {
	t.Helper()

	directory := t.TempDir()
	published := filepath.Join(directory, "probe")

	err := compiler.PublishBuildArtifact(artifact, published)
	if err != nil {
		t.Fatal(err)
	}

	command := exec.CommandContext(t.Context(), "./probe")
	command.Dir = directory

	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}

	var trace struct {
		Spans []struct {
			Name string `json:"Name"`
		} `json:"Spans"`
	}

	err = json.Unmarshal(output, &trace)
	if err != nil {
		t.Fatal(err)
	}

	operations := 0

	for _, span := range trace.Spans {
		if span.Name == "vendor-operation" {
			operations++
		}
	}

	if operations != 2 {
		t.Fatalf("vendored binary reported %d instrumented operations", operations)
	}
}
