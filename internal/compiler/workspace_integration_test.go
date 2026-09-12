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
	code, err := discovery.LoadContext(t.Context(), discovery.Options{Root: source, Patterns: []string{"./ops"}, Env: []string{"GOWORK=off", "GOFLAGS="}})
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
	prepared, err := PrepareWorkspace(t.Context(), WorkspaceRequest{SourceDirs: []string{source}, OriginalWorkspaceDir: source, Workspace: []byte("go 1.27\nuse .\n"), Runtime: runtime, Parent: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(prepared.Dir, "probe")
	rules := filepath.Join(prepared.Runtime.Dir, "rules")
	build := exec.CommandContext(t.Context(), executable, "--rules", rules, "go", "build", "-race", "-buildvcs=false", "-o", binary, ".")
	build.Dir = prepared.Relocations[source]
	build.Env = append(os.Environ(), "GOTMPDIR="+t.TempDir(), "GOWORK="+prepared.WorkspaceFile, "GOFLAGS=", "OTELC_BUILD_FLAGS=", "OTELC_RULES="+rules, "OTELC_WORK_DIR="+prepared.Dir)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("prepared backend build failed: %v\n%s", err, output)
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
