package compiler

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPreparedWorkspaceWithBackend(t *testing.T) {
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
	for name, content := range map[string]string{
		"selected.go":   "//go:build otelplan_probe\n\npackage ops\nconst buildSelection = true\n",
		"unselected.go": "//go:build !otelplan_probe\n\npackage ops\nconst buildSelection = false\n",
	} {
		if err := os.WriteFile(filepath.Join(source, "ops", name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	opsPath := filepath.Join(source, "ops", "ops.go")
	opsSource, err := os.ReadFile(opsPath)
	if err != nil {
		t.Fatal(err)
	}
	opsSource = []byte(strings.Replace(string(opsSource), "(size int, err error) {", "(size int, err error) {\nif !buildSelection { panic(\"wrong build selection\") }", 1))
	if err := os.WriteFile(opsPath, opsSource, 0600); err != nil {
		t.Fatal(err)
	}
	originalFiles := model.Artifacts{Dir: source}
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
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
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		originalFiles.Files = append(originalFiles.Files, model.ArtifactFile{Path: filepath.ToSlash(relative), Digest: artifactDigest(data)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	code, err := discovery.LoadContext(t.Context(), discovery.Options{Root: source, Patterns: []string{"./ops"}, BuildTags: []string{"otelplan_probe"}, Env: []string{"GOWORK=off", "GOFLAGS=-race -buildvcs=false"}})
	if err != nil {
		t.Fatal(err)
	}
	symbol, ok := code.Symbol("example.com/probe/ops.(*Worker).Handle")
	if !ok {
		t.Fatal("fixture target missing")
	}
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{{SymbolID: symbol.ID, Signature: symbol.Signature, SpanName: "prepared-operation", ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: 0}, ErrorStrategy: model.ErrorStrategy{Record: true, Indexes: []int{1}}, Attributes: []model.AttributePlan{{Key: "request.id", From: model.AttributeSource{Argument: "request.ID"}}}}}}
	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}
	files, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := StageArtifacts(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareWorkspace(t.Context(), WorkspaceRequest{OriginalWorkspaceDir: source, Workspace: []byte("go 1.27\nuse .\n"), Runtime: runtime, Parent: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(prepared.Dir, "probe")
	env := append(os.Environ(), "GOFLAGS=-tags=wrong", "GOOS=wrong")
	runtimeSelection, err := ReadModuleSelection(t.Context(), runtime.Dir, append(os.Environ(), "GOFLAGS=", "GOWORK=off"))
	if err != nil {
		t.Fatal(err)
	}
	request := PreparedBuildRequest{
		BuildEnvironment: code.EffectiveBuild,
		Workspace:        prepared, ModuleDir: prepared.Relocations[source], Executable: executable, Backend: backend,
		ApplicationModules: code.Modules, RuntimeModules: runtimeSelection, RuntimeOriginalDir: runtime.Dir,
		Env: env, GoArgs: []string{"-o", binary, "."},
	}
	for _, change := range []func(*PreparedBuildRequest){
		func(r *PreparedBuildRequest) { r.Backend.Digest = "sha256:" + strings.Repeat("0", 64) },
		func(r *PreparedBuildRequest) { r.ModuleDir = source },
		func(r *PreparedBuildRequest) { r.GoArgs = []string{"-race=false", "-o", binary, "."} },
		func(r *PreparedBuildRequest) { r.GoArgs = []string{"-tags=wrong", "-o", binary, "."} },
		func(r *PreparedBuildRequest) { r.ApplicationModules = nil },
		func(r *PreparedBuildRequest) {
			r.ApplicationModules = append([]model.ModuleInfo(nil), r.ApplicationModules...)
			r.ApplicationModules[0].Version = "v999.0.0"
		},
	} {
		invalid := request
		change(&invalid)
		if err := BuildPrepared(t.Context(), invalid); err == nil {
			t.Fatal("accepted invalid build inputs")
		}
		if _, err := os.Stat(binary); !os.IsNotExist(err) {
			t.Fatal("invalid inputs produced a binary")
		}
	}
	if err := BuildPrepared(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	output, err := exec.CommandContext(t.Context(), binary).Output()
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Spans []struct {
			Name       string
			Attributes map[string]any
		}
	}
	if err := json.Unmarshal(output, &result); err != nil {
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
	if err := VerifyArtifacts(runtime); err != nil {
		t.Fatalf("original runtime changed: %v", err)
	}
	if err := VerifyArtifacts(originalFiles); err != nil {
		t.Fatalf("backend changed original source tree: %v", err)
	}
}
