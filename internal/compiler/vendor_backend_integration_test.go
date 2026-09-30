package compiler

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestVendorBuildWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is not set")
	}

	fixture, err := filepath.Abs("../backend/otelc/testdata/accessors")
	if err != nil {
		t.Fatal(err)
	}

	source, err := CopySourceTree(t.Context(), fixture, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	buildDir := filepath.Join(source, "cmd", "probe")
	if err := os.MkdirAll(buildDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(source, "main.go"), filepath.Join(buildDir, "main.go")); err != nil {
		t.Fatal(err)
	}

	prepareVendoredFixture(t, source)
	original := sourceInventory(t, source)

	code, err := discovery.LoadContext(t.Context(), discovery.Options{Root: source, Patterns: []string{"./ops"}, Env: []string{"GOWORK=off", "GOFLAGS=-mod=vendor", "GOPROXY=off"}, Offline: true})
	if err != nil {
		t.Fatal(err)
	}

	if code.EffectiveBuild.ModuleMode != "vendor" {
		t.Fatal("analysis did not retain vendor mode")
	}

	symbol, ok := code.Symbol("example.com/probe/ops.(*Worker).Handle")
	if !ok {
		t.Fatal("fixture target missing")
	}

	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{{SymbolID: symbol.ID, Signature: symbol.Signature, SpanName: "vendor-operation", ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0}, ErrorStrategy: model.ErrorStrategy{Record: true, Indexes: []int{1}}}}}

	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	prepareVendoredRuntime(t, backend, code, plan)

	built, err := BuildResolved(t.Context(), ResolvedBuildRequest{Code: code, Plan: plan, Backend: backend, Executable: executable, RuntimeVersion: "test", WorkingDir: buildDir, Parent: t.TempDir(), Env: os.Environ(), GoArgs: []string{buildDir}, Packages: []string{buildDir}, DefaultOutput: true, Offline: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(built.Dir) }()

	if len(built.Files) != 1 || built.Files[0].DefaultName != "probe" {
		t.Fatalf("unexpected vendor build artifacts: %+v", built.Files)
	}

	published := filepath.Join(t.TempDir(), "probe")
	if err := PublishBuildArtifact(built.Files[0], published); err != nil {
		t.Fatal(err)
	}

	output, err := exec.CommandContext(t.Context(), published).Output()
	if err != nil {
		t.Fatal(err)
	}

	var trace struct {
		Spans []struct{ Name string }
	}
	if err := json.Unmarshal(output, &trace); err != nil {
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

	if err := VerifyArtifacts(original); err != nil {
		t.Fatalf("vendor build changed the source checkout: %v", err)
	}
}

func prepareVendoredFixture(t *testing.T, source string) {
	t.Helper()

	for _, args := range [][]string{{"mod", "download", "all"}, {"mod", "vendor"}} {
		command := exec.CommandContext(t.Context(), "go", args...)
		command.Dir = source

		command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")

		if args[1] == "vendor" {
			command.Env = append(command.Env, "GOPROXY=off")
		}
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("prepare fixture dependencies: %v\n%s", err, output)
		}
	}
}

func prepareVendoredRuntime(t *testing.T, backend model.LockBackend, code *model.CodeModel, plan model.ResolvedPlan) {
	t.Helper()

	files, err := otelc.RenderBundle(backend, "test", code, plan, "otelplan.local/generated")
	if err != nil {
		t.Fatal(err)
	}

	runtime, err := StageArtifacts(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}

	command := exec.CommandContext(t.Context(), "go", "mod", "download", "all")
	command.Dir = runtime.Dir

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("prepare generated runtime dependencies: %v\n%s", err, output)
	}
}

func sourceInventory(t *testing.T, root string) model.Artifacts {
	t.Helper()
	files := model.Artifacts{Dir: root}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		files.Files = append(files.Files, model.ArtifactFile{Path: filepath.ToSlash(relative), Digest: artifactDigest(data)})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return files
}
