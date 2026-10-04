package compiler_test

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type pinnedWorkspaceFixture struct {
	root     *os.Root
	buildDir string
	original model.Artifacts
	code     *model.CodeModel
	plan     model.ResolvedPlan
}

func TestPreparedWorkspaceWithBackend(t *testing.T) {
	t.Parallel()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is not set")
	}

	fixture := newPinnedWorkspaceFixture(t)

	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	prepared, runtime := preparePinnedWorkspace(t, fixture, backend)

	env := append(os.Environ(), "GOFLAGS=-tags=wrong", "GOOS=wrong")
	request := pinnedPreparedBuildRequest(t, fixture, prepared, runtime, backend, executable, env)
	assertInvalidPreparedBuilds(t, request, fixture.root.Name())

	built := buildPinnedWorkspace(t, fixture, backend, executable, env)
	if len(built.Files) != 1 {
		t.Fatal("expected a single executable")
	}

	artifact := built.Files[0]
	assertPinnedBuildArtifact(t, artifact)
	runPublishedWorkspaceArtifact(t, artifact)

	err = compiler.VerifyArtifacts(runtime)
	if err != nil {
		t.Fatalf("original runtime changed: %v", err)
	}

	err = compiler.VerifyArtifacts(fixture.original)
	if err != nil {
		t.Fatalf("backend changed original source tree: %v", err)
	}
}

func newPinnedWorkspaceFixture(t *testing.T) pinnedWorkspaceFixture {
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
	configurePinnedWorkspaceSource(t, root)
	original := pinnedWorkspaceInventory(t, root)

	var options discovery.Options

	options.Root = source
	options.Patterns = []string{"./ops"}
	options.BuildTags = []string{"otelplan_probe"}
	options.Env = []string{"GOWORK=off", "GOFLAGS=-race -buildvcs=false"}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return pinnedWorkspaceFixture{
		root: root, buildDir: filepath.Join(source, "cmd", "probe"), original: original,
		code: code, plan: pinnedWorkspacePlan(t, code),
	}
}

func configurePinnedWorkspaceSource(t *testing.T, root *os.Root) {
	t.Helper()

	err := root.MkdirAll(filepath.Join("cmd", "probe"), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = root.Rename("main.go", filepath.Join("cmd", "probe", "main.go"))
	if err != nil {
		t.Fatal(err)
	}

	writeRelocationFixture(t, root, map[string]string{
		"ops/selected.go":   "//go:build otelplan_probe\n\npackage ops\nconst buildSelection = true\n",
		"ops/unselected.go": "//go:build !otelplan_probe\n\npackage ops\nconst buildSelection = false\n",
	})

	opsPath := filepath.Join("ops", "ops.go")

	opsSource, err := root.ReadFile(opsPath)
	if err != nil {
		t.Fatal(err)
	}

	replacement := "(size int, err error) {\nif !buildSelection { panic(\"wrong build selection\") }"
	opsSource = []byte(strings.Replace(string(opsSource), "(size int, err error) {", replacement, 1))

	err = root.WriteFile(opsPath, opsSource, 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func pinnedWorkspaceInventory(t *testing.T, root *os.Root) model.Artifacts {
	t.Helper()

	var original model.Artifacts

	original.Dir = root.Name()

	err := fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("read workspace fixture entry: %w", walkErr)
		}

		if entry.IsDir() {
			return nil
		}

		data, err := root.ReadFile(filepath.FromSlash(path))
		if err != nil {
			return fmt.Errorf("read workspace fixture content: %w", err)
		}

		original.Files = append(original.Files, model.ArtifactFile{
			Path: path, Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(data)),
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return original
}

func pinnedWorkspacePlan(t *testing.T, code *model.CodeModel) model.ResolvedPlan {
	t.Helper()

	symbol, ok := code.Symbol("example.com/probe/ops.(*Worker).Handle")
	if !ok {
		t.Fatal("fixture target missing")
	}

	var target model.ResolvedTarget

	target.SymbolID, target.Signature, target.SpanName = symbol.ID, symbol.Signature, "prepared-operation"
	target.ContextStrategy = model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0}
	target.ErrorStrategy = model.ErrorStrategy{Record: true, Indexes: []int{1}}

	var attribute model.AttributePlan

	attribute.Key = "request.id"
	attribute.From.Argument = "request.ID"
	target.Attributes = []model.AttributePlan{attribute}

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{target}

	return plan
}

func preparePinnedWorkspace(t *testing.T, fixture pinnedWorkspaceFixture,
	backend model.LockBackend) (compiler.PreparedWorkspace, model.Artifacts) {
	t.Helper()

	files, err := otelc.RenderBundle(backend, "test", fixture.code, fixture.plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	runtime, err := compiler.StageArtifacts(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}

	request, err := compiler.WorkspaceForAnalysis(fixture.code)
	if err != nil {
		t.Fatal(err)
	}

	request.Runtime, request.Parent = runtime, t.TempDir()

	prepared, err := compiler.PrepareWorkspace(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	return prepared, runtime
}

func pinnedPreparedBuildRequest(t *testing.T, fixture pinnedWorkspaceFixture,
	prepared compiler.PreparedWorkspace, runtime model.Artifacts, backend model.LockBackend,
	executable string, env []string) compiler.PreparedBuildRequest {
	t.Helper()

	selection, err := compiler.ReadModuleSelection(t.Context(), runtime.Dir,
		append(os.Environ(), "GOFLAGS=", "GOWORK=off"))
	if err != nil {
		t.Fatal(err)
	}

	preparedDir, err := prepared.BuildDirectory(fixture.buildDir)
	if err != nil {
		t.Fatal(err)
	}

	return compiler.PreparedBuildRequest{
		BuildEnvironment: fixture.code.EffectiveBuild, Workspace: prepared, ModuleDir: preparedDir,
		Executable: executable, Backend: backend, ApplicationModules: fixture.code.Modules,
		RuntimeModules: selection, RuntimeOriginalDir: runtime.Dir, Env: env,
		GoArgs: []string{"-o", filepath.Join(prepared.Dir, "probe"), "."},
	}
}

func assertInvalidPreparedBuilds(t *testing.T, request compiler.PreparedBuildRequest, source string) {
	t.Helper()

	binary := filepath.Join(request.Workspace.Dir, "probe")
	for _, testCase := range []struct {
		name   string
		change func(*compiler.PreparedBuildRequest)
	}{
		{name: "backend digest", change: func(r *compiler.PreparedBuildRequest) {
			r.Backend.Digest = "sha256:" + strings.Repeat("0", sha256.Size*2)
		}},
		{name: "source directory", change: func(r *compiler.PreparedBuildRequest) { r.ModuleDir = source }},
		{name: "race flag", change: func(r *compiler.PreparedBuildRequest) {
			r.GoArgs = []string{"-race=false", "-o", binary, "."}
		}},
		{name: "build tags", change: func(r *compiler.PreparedBuildRequest) {
			r.GoArgs = []string{"-tags=wrong", "-o", binary, "."}
		}},
		{name: "missing application modules", change: func(r *compiler.PreparedBuildRequest) { r.ApplicationModules = nil }},
		{name: "application module version", change: func(r *compiler.PreparedBuildRequest) {
			r.ApplicationModules = slices.Clone(r.ApplicationModules)
			r.ApplicationModules[0].Version = "v999.0.0"
		}},
	} {
		invalid := request
		testCase.change(&invalid)

		err := compiler.BuildPrepared(t.Context(), invalid)
		if err == nil {
			t.Fatalf("accepted invalid %s", testCase.name)
		}

		_, err = os.Stat(binary)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("invalid %s produced a binary: %v", testCase.name, err)
		}
	}
}

func buildPinnedWorkspace(t *testing.T, fixture pinnedWorkspaceFixture,
	backend model.LockBackend, executable string, env []string) compiler.BuildResult {
	t.Helper()

	var request compiler.ResolvedBuildRequest

	request.Code, request.Plan, request.Backend = fixture.code, fixture.plan, backend
	request.Executable, request.RuntimeVersion = executable, "test"
	request.WorkingDir, request.Parent = fixture.buildDir, t.TempDir()
	request.Env = env
	request.GoArgs, request.Packages = []string{fixture.buildDir}, []string{fixture.buildDir}
	request.DefaultOutput, request.Offline = true, true

	built, err := compiler.BuildResolved(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(built.Dir) })

	return built
}

func assertPinnedBuildArtifact(t *testing.T, artifact compiler.BuildArtifact) {
	t.Helper()

	if artifact.DefaultName != "probe" {
		t.Fatalf("unexpected default output: %s", artifact.DefaultName)
	}

	root, err := os.OpenRoot(artifact.Dir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	relative, err := filepath.Rel(artifact.Dir, artifact.File)
	if err != nil {
		t.Fatal(err)
	}

	data, err := root.ReadFile(relative)
	if err != nil || fmt.Sprintf("sha256:%x", sha256.Sum256(data)) != artifact.Digest {
		t.Fatal("build output identity mismatch", err)
	}
}

func runPublishedWorkspaceArtifact(t *testing.T, artifact compiler.BuildArtifact) {
	t.Helper()

	directory := filepath.Join(t.TempDir(), "bin")
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

	assertPinnedWorkspaceTraces(t, output)
}

func assertPinnedWorkspaceTraces(t *testing.T, output []byte) {
	t.Helper()

	var result struct {
		Spans []struct {
			Name       string         `json:"Name"`
			Attributes map[string]any `json:"Attributes"`
		} `json:"Spans"`
	}

	err := json.Unmarshal(output, &result)
	if err != nil {
		t.Fatal(err)
	}

	operations, captured := 0, 0

	for _, span := range result.Spans {
		if span.Name == "prepared-operation" {
			operations++

			if span.Attributes["request.id"] == "approved-id" {
				captured++
			}
		}
	}

	if operations != 2 || captured != 1 || len(result.Spans) != 5 {
		t.Fatalf("incorrect prepared instrumentation: %s", output)
	}
}
